package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/jtharris/yadokari/internal/gfx"
	"github.com/jtharris/yadokari/internal/glyph"
	"github.com/jtharris/yadokari/internal/session"
	"github.com/jtharris/yadokari/internal/store"
	"github.com/jtharris/yadokari/internal/wk"
	"github.com/jtharris/yadokari/internal/wkmock"
	"github.com/jtharris/yadokari/internal/wksync"
)

type harness struct {
	t    *testing.T
	app  *App
	mock *wkmock.Server
	st   *store.Store
	raw  strings.Builder // raw output (kitty uploads) the app sent
	fake *fakeKitty      // the renderer: every glyph's upload is tracked
	snd  *fakeAudio      // the audio player: what was played
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	clock := now
	m := wkmock.New(wkmock.SampleData(now))
	m.Now = func() time.Time { return clock }
	srv := m.Start()
	t.Cleanup(srv.Close)
	client, err := wk.New("tok", wk.WithBaseURL(srv.URL+"/v2"), wk.WithRateLimit(60000, 1000))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	syncer := &wksync.Syncer{Client: client, Store: st, Now: func() time.Time { return clock }}
	if _, err := syncer.Run(ctx, append(append([]string{}, wksync.Foreground...), wksync.Background...)); err != nil {
		t.Fatal(err)
	}
	fake := &fakeKitty{seen: map[string]bool{}}
	snd := &fakeAudio{}
	app := NewApp(
		Deps{Store: st, Syncer: syncer, Now: func() time.Time { return clock }, Location: time.UTC, Endpoint: "mock"},
		ReviewDeps{Renderer: fake, Uploader: &wksync.Uploader{Client: client, Store: st}, Audio: snd, Sounds: snd},
	)
	h := &harness{t: t, app: app, mock: m, st: st, fake: fake, snd: snd}
	h.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	h.send(graphicsReadyMsg{}) // as App.Init's timer would
	h.send(h.app.dash.load()())
	return h
}

// send delivers a message and runs the resulting commands to completion,
// skipping timers and raw output.
func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	_, cmd := h.app.Update(msg)
	h.run(cmd, 0)
	h.checkGlyphsSent()
}

// checkGlyphsSent fails the test if the screen now shows a kitty image the
// terminal was never sent: a blank glyph. Every harness test checks this
// after every message.
func (h *harness) checkGlyphsSent() {
	h.t.Helper()
	h.fake.drawn = nil
	_ = h.app.View()
	sent := h.raw.String()
	for _, key := range h.fake.drawn {
		if !strings.Contains(sent, "UPLOAD "+key+";") {
			h.t.Fatalf("screen %d shows image %s, but its upload never reached the terminal\nsent: %q\n%s",
				h.app.screen, key, sent, h.screen())
		}
	}
}

func (h *harness) run(cmd tea.Cmd, depth int) {
	if cmd == nil || depth > 20 {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case nil:
	case tea.RawMsg:
		fmt.Fprint(&h.raw, msg.Msg)
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c, depth+1)
		}
	case tickMsg:
		// the dashboard's clock: deliver once but don't follow up with another tick
		_, _ = h.app.Update(msg)
	default:
		_, next := h.app.Update(msg)
		h.run(next, depth+1)
	}
}

func (h *harness) key(s string) {
	h.t.Helper()
	switch s {
	case "enter":
		h.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	case "esc":
		h.send(tea.KeyPressMsg{Code: tea.KeyEscape})
	case "backspace":
		h.send(tea.KeyPressMsg{Code: tea.KeyBackspace})
	default:
		for _, r := range s {
			h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
}

func (h *harness) screen() string { return ansi.Strip(h.app.View().Content) }

// answerFor returns the right answer for the current question.
func (h *harness) answerFor() string {
	q, it, ok := h.app.review.current()
	if !ok {
		h.t.Fatal("no current question")
	}
	if q.Kind == session.Reading {
		for _, r := range it.Subject.Data.Readings {
			if r.Primary {
				return r.Reading // kana passes straight through the IME
			}
		}
	}
	return primaryMeaning(it.Subject.Data)
}

func TestReviewAllCorrect(t *testing.T) {
	h := newHarness(t)
	if !strings.Contains(h.screen(), "Reviews      4") {
		t.Fatalf("dashboard:\n%s", h.screen())
	}
	h.key("r")
	if h.app.screen != reviewScreen {
		t.Fatalf("r should open reviews:\n%s", h.screen())
	}
	for n := 0; h.app.screen == reviewScreen; n++ {
		if n > 20 {
			t.Fatal("session did not end")
		}
		h.key(h.answerFor())
		h.key("enter")
		if !strings.Contains(h.screen(), "✓") {
			t.Fatalf("expected correct:\n%s", h.screen())
		}
		h.key("enter")
	}
	if h.app.screen != summaryScreen {
		t.Fatalf("expected summary, screen %d", h.app.screen)
	}
	out := h.screen()
	// SRS movement comes from the assignments the mock returned.
	for _, want := range []string{"4 reviewed · 100% correct", "Answered correctly (4)", "All reviews uploaded.",
		"Guru 1 ↑", "Apprentice 2 ↑"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
	if w := h.mock.Writes(); len(w) != 4 {
		t.Errorf("mock received %d reviews, want 4", len(w))
	}
	h.key("enter")
	if h.app.screen != dashScreen || !strings.Contains(h.screen(), "Reviews      0") {
		t.Errorf("back on the dashboard with nothing due:\n%s", h.screen())
	}
	if ok, _ := h.st.LoadSession(context.Background(), sessionName, &session.Snapshot{}); ok {
		t.Error("finished session should be cleared")
	}
}

func TestReviewWrongWarnAndResume(t *testing.T) {
	h := newHarness(t)
	h.key("r")

	// Kana typed for a meaning question is a warning, not a miss.
	for {
		q, _, _ := h.app.review.current()
		if q.Kind == session.Meaning {
			break
		}
		h.key(h.answerFor())
		h.key("enter")
		h.key("enter")
	}
	h.key("あ")
	h.key("enter")
	if !strings.Contains(h.screen(), "We want the meaning") || h.app.review.state != answering {
		t.Fatalf("expected a warning:\n%s", h.screen())
	}
	h.key("backspace")

	// A wrong answer shows the answer panel and counts a miss.
	q, _, _ := h.app.review.current()
	h.key("zzzz")
	h.key("enter")
	if !strings.Contains(h.screen(), "✗") || !strings.Contains(h.screen(), "Meaning  ") {
		t.Fatalf("expected wrong + answer panel:\n%s", h.screen())
	}
	if it := h.app.review.sess.Item(q.Index); it.MeaningMisses != 1 {
		t.Errorf("miss not recorded: %+v", it)
	}
	h.key("enter")

	// Leave mid-session and come back: same question, same misses.
	before, _, _ := h.app.review.current()
	h.key("esc")
	if h.app.screen != dashScreen {
		t.Fatal("esc should return to the dashboard")
	}
	h.key("r")
	after, _, _ := h.app.review.current()
	if !h.app.review.resume || after.AssignmentID != before.AssignmentID || after.Kind != before.Kind {
		t.Errorf("resume: before %+v after %+v (resumed=%v)", before, after, h.app.review.resume)
	}
	if it := h.app.review.sess.Item(q.Index); it.MeaningMisses != 1 {
		t.Errorf("misses lost on resume: %+v", it)
	}
}

func TestReviewItemsMatchQueue(t *testing.T) {
	// Every due item gets a renderable question; the image-only radical in
	// the sample data isn't due, so all four have characters.
	h := newHarness(t)
	items, err := h.st.ReviewItems(context.Background(), now, 3)
	if err != nil {
		t.Fatal(err)
	}
	var objs []string
	for _, it := range items {
		objs = append(objs, it.Subject.Object)
	}
	raw, _ := json.Marshal(objs)
	if string(raw) != `["radical","radical","kanji","kana_vocabulary"]` {
		t.Errorf("due items: %s", raw)
	}
}

func TestReviewsDisabledShowsNotice(t *testing.T) {
	h := newHarness(t)
	h.app.rdeps.Disabled = "This is a development build: it never sends answers to WaniKani."
	h.key("r")
	if h.app.screen != dashScreen || !strings.Contains(h.screen(), "development build") {
		t.Errorf("expected a notice on the dashboard:\n%s", h.screen())
	}
	if len(h.mock.Writes()) != 0 {
		t.Error("nothing should be written")
	}
}

// TestAnswerPanelHidesUnaskedPart: missing a kanji's reading before its
// meaning has been asked must not reveal the meaning.
func TestAnswerPanelHidesUnaskedPart(t *testing.T) {
	h := newHarness(t)
	h.key("r")
	for n := 0; ; n++ {
		if n > 20 {
			t.Fatal("never got the kanji's reading before its meaning")
		}
		q, it, _ := h.app.review.current()
		state := h.app.review.sess.Item(q.Index)
		if it.Subject.Object == wk.ObjectKanji && q.Kind == session.Reading && !state.MeaningDone {
			break
		}
		if it.Subject.Object == wk.ObjectKanji {
			// Restart until the reading comes first (the order is random).
			h.key("esc")
			h.st.ClearSession(context.Background(), sessionName)
			h.key("r")
			continue
		}
		h.key(h.answerFor())
		h.key("enter")
		h.key("enter")
	}
	h.key("に") // wrong reading for 一
	h.key("enter")
	out := h.screen()
	if !strings.Contains(out, "Reading  いち") {
		t.Errorf("the reading should be shown:\n%s", out)
	}
	if strings.Contains(out, "Meaning  ") || strings.Contains(out, "One") {
		t.Errorf("the unasked meaning must stay hidden:\n%s", out)
	}
	h.key("enter")

	// Later, answering the meaning: the reading was already missed (and so
	// revealed), so Tab shows both.
	for n := 0; n < 20; n++ {
		q, it, ok := h.app.review.current()
		if !ok {
			break
		}
		h.key(h.answerFor())
		h.key("enter")
		if it.Subject.Object == wk.ObjectKanji && q.Kind == session.Meaning {
			h.send(tea.KeyPressMsg{Code: tea.KeyTab})
			out := h.screen()
			if !strings.Contains(out, "Meaning  One") || !strings.Contains(out, "Reading  いち") {
				t.Errorf("after both parts, the panel shows both:\n%s", out)
			}
			return
		}
		h.key("enter")
	}
	t.Fatal("never reached the kanji's meaning question")
}

func TestSummaryMovementAfterLateUpload(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	// A wrong answer queued but not yet uploaded when the summary opens.
	if err := h.st.QueueReview(ctx, 101, wk.ReviewInput{AssignmentID: 1002, IncorrectMeaningAnswers: 1}); err != nil {
		t.Fatal(err)
	}
	sum := Summary{Incorrect: []SummaryItem{{Characters: "一", Meaning: "One", Type: wk.ObjectKanji, AssignmentID: 1002, From: 2}}}
	if err := sum.Refresh(ctx, h.st); err != nil {
		t.Fatal(err)
	}
	if out := ansi.Strip(sum.View(100, 30)); !strings.Contains(out, "1 reviews still uploading") || strings.Contains(out, "↓") {
		t.Errorf("before upload:\n%s", out)
	}
	h.app.review = &Review{deps: h.app.deps, rdeps: h.app.rdeps}
	h.send(ReviewDoneMsg{Summary: sum})
	h.send(h.app.review.upload()())
	if out := h.screen(); !strings.Contains(out, "一 One") || !strings.Contains(out, "Apprentice 1 ↓") || !strings.Contains(out, "All reviews uploaded.") {
		t.Errorf("after upload:\n%s", out)
	}
}

func TestDashboardRejectedReviews(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.st.QueueReview(ctx, 3, wk.ReviewInput{AssignmentID: 1001}); err != nil {
		t.Fatal(err)
	}
	if err := h.st.QueueReview(ctx, 101, wk.ReviewInput{AssignmentID: 1002}); err != nil {
		t.Fatal(err)
	}
	items, _ := h.st.Outbox(ctx, false)
	if err := h.st.OutboxFailed(ctx, items[0].ID, "HTTP 422: assignment is not available for review"); err != nil {
		t.Fatal(err)
	}
	h.send(h.app.dash.Reload()())
	out := h.screen()
	for _, want := range []string{
		"Rejected by WaniKani (1) · not recorded",
		"二 Two  HTTP 422: assignment is not available for review",
		"· 1 to upload", "x dismiss rejected",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	h.key("x")
	if out := h.screen(); strings.Contains(out, "rejected") || !strings.Contains(out, "1 to upload") {
		t.Errorf("after dismiss:\n%s", out)
	}
}

// drain runs a dashboard command chain to the end. Commands must not sleep
// (tea.Tick blocks when called directly).
func drain(d *Dashboard, cmd tea.Cmd) {
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			_, next := d.Update(msg)
			queue = append(queue, next)
		}
	}
}

func TestOfflineReviewsUploadOnNextSync(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.st.QueueReview(ctx, 3, wk.ReviewInput{AssignmentID: 1001}); err != nil {
		t.Fatal(err)
	}
	online := h.app.deps

	// No network: nothing listens on port 1.
	dead, err := wk.New("tok", wk.WithBaseURL("http://127.0.0.1:1/v2/"), wk.WithRateLimit(60000, 1000))
	if err != nil {
		t.Fatal(err)
	}
	offline := online
	offline.Syncer = &wksync.Syncer{Client: dead, Store: h.st, Now: online.Now}
	offline.Uploader = &wksync.Uploader{Client: dead, Store: h.st}
	d := NewDashboard(offline)
	d.Update(windowSize(120, 30))
	drain(d, tea.Batch(d.load(), d.startSync())) // Init minus its 30 s timer
	out := ansi.Strip(d.render())
	if !strings.Contains(out, "sync failed: can't reach WaniKani") || !strings.Contains(out, "1 to upload") || len(h.mock.Writes()) != 0 {
		t.Errorf("offline: the review should stay queued:\n%s", out)
	}

	// Back online: the dashboard's next sync sends it before syncing.
	online.Uploader = &wksync.Uploader{Client: online.Syncer.Client, Store: h.st}
	d = NewDashboard(online)
	d.Update(windowSize(120, 30))
	drain(d, tea.Batch(d.load(), d.startSync())) // Init minus its 30 s timer
	if w := h.mock.Writes(); len(w) != 1 {
		t.Fatalf("mock received %d reviews, want 1", len(w))
	}
	if out := ansi.Strip(d.render()); strings.Contains(out, "to upload") || !strings.Contains(out, "Reviews      3") {
		t.Errorf("online: queue should be empty and 3 due:\n%s", out)
	}
}

// TestPreviewKittyUploadsReachTheCaller: the final frame's image upload must
// still be queued when Preview returns, for the caller to send.
func TestPreviewKittyUploadsReachTheCaller(t *testing.T) {
	font, err := glyph.FindFont("")
	if err != nil {
		t.Skip("no CJK font:", err)
	}
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	d := wkmock.DemoData(now)
	st.Upsert(ctx, store.Subjects, toRaw(d.Subjects))
	st.Upsert(ctx, store.Assignments, toRaw(d.Assignments))
	userRaw, _ := json.Marshal(d.User)
	st.PutSingleton(ctx, "user", store.Singleton{Raw: userRaw, FetchedAt: now})

	for _, screen := range []string{"review", "review-wrong", "review-reading"} {
		r := gfx.New(gfx.Kitty, 10, 20)
		out, err := Preview(ctx, Deps{Store: st, Now: func() time.Time { return now }, Location: time.UTC},
			ReviewDeps{Renderer: r, Font: font}, screen, 80, 24)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.ContainsRune(out, kitty.Placeholder) {
			t.Errorf("%s: no placeholders in the frame", screen)
		}
		if up := r.Flush(); !strings.Contains(up, "\x1b_G") {
			t.Errorf("%s: the frame's image upload was lost", screen)
		}
	}
}

func TestAnswerBoxModeAndPlaceholder(t *testing.T) {
	h := newHarness(t)
	h.key("r")
	q, _, _ := h.app.review.current()
	box := func() string {
		for _, l := range strings.Split(h.screen(), "\n") {
			if strings.Contains(l, "╭─ ") && (strings.Contains(l, "Kana") || strings.Contains(l, "English")) {
				return l
			}
		}
		t.Fatalf("no answer box:\n%s", h.screen())
		return ""
	}
	wantMode, wantPlaceholder := " A  English", "Your answer"
	if q.Kind == session.Reading {
		wantMode, wantPlaceholder = " あ  Kana", "答え"
	}
	if !strings.Contains(box(), wantMode) || !strings.Contains(h.screen(), "│ "+wantPlaceholder) {
		t.Errorf("empty box should show %q in the border and %q inside:\n%s", wantMode, wantPlaceholder, h.screen())
	}
	first := string([]rune(h.answerFor())[:1])
	h.key(first)
	if strings.Contains(h.screen(), wantPlaceholder) {
		t.Errorf("placeholder should go once typing starts:\n%s", h.screen())
	}
	if c := h.app.review.Cursor(); c == nil || c.X != (100-64)/2+2+ansi.StringWidth(first) {
		t.Errorf("cursor %+v should follow the typed text", c)
	}
}

// fakeKitty stands in for the kitty tier: every new size of a source queues
// an "upload" that only Flush hands out, once.
type fakeKitty struct {
	seen    map[string]bool
	pending string
	drawn   []string // keys drawn since the harness last cleared it
}

func (f *fakeKitty) Tier() gfx.Tier { return gfx.Kitty }
func (f *fakeKitty) Draw(src gfx.Source, cols, rows int, _, _ color.Color) (string, error) {
	key := fmt.Sprintf("%s@%dx%d", src.Key, cols, rows)
	f.drawn = append(f.drawn, key)
	if !f.seen[key] {
		f.seen[key] = true
		f.pending += "UPLOAD " + key + ";"
	}
	return strings.TrimSuffix(strings.Repeat(strings.Repeat("x", cols)+"\n", rows), "\n"), nil
}
func (f *fakeKitty) Flush() string { s := f.pending; f.pending = ""; return s }
func (f *fakeKitty) Reset() string { f.seen, f.pending = map[string]bool{}, ""; return "" }

// TestResizeSendsGlyphUpload: a resize redraws the glyph at a new size; its
// upload must reach the terminal, or the glyph stays blank at that size for
// the rest of the session.
func TestResizeSendsGlyphUpload(t *testing.T) {
	h := newHarness(t)
	h.key("r")
	if h.app.screen != reviewScreen {
		t.Fatalf("r should open reviews:\n%s", h.screen())
	}
	_, cmd := h.app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	var raw strings.Builder
	var collect func(tea.Cmd)
	collect = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch msg := c().(type) {
		case tea.RawMsg:
			fmt.Fprint(&raw, msg.Msg)
		case tea.BatchMsg:
			for _, c := range msg {
				collect(c)
			}
		}
	}
	collect(cmd)
	if !strings.Contains(raw.String(), "@120x19") {
		t.Errorf("resize: the new-size upload was not sent; got %q", raw.String())
	}
}
