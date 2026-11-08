package ui

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jtharris/yadokari/internal/gfx"
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
	app := NewApp(
		Deps{Store: st, Syncer: syncer, Now: func() time.Time { return clock }, Location: time.UTC, Endpoint: "mock"},
		ReviewDeps{Renderer: gfx.New(gfx.Plain, 0, 0), Uploader: &wksync.Uploader{Client: client, Store: st}},
	)
	h := &harness{t: t, app: app, mock: m, st: st}
	h.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	h.send(h.app.dash.load()())
	return h
}

// send delivers a message and runs the resulting commands to completion,
// skipping timers and raw output.
func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	_, cmd := h.app.Update(msg)
	h.run(cmd, 0)
}

func (h *harness) run(cmd tea.Cmd, depth int) {
	if cmd == nil || depth > 20 {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case nil, tea.RawMsg:
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c, depth+1)
		}
	case tickMsg, reviewReadyMsg:
		// timers: deliver once but don't follow up with another tick
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
	if !strings.Contains(h.screen(), "Reviews  4") {
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
	for _, want := range []string{"4 reviewed · 100% correct", "Answered correctly (4)", "All reviews uploaded."} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
	if w := h.mock.Writes(); len(w) != 4 {
		t.Errorf("mock received %d reviews, want 4", len(w))
	}
	h.key("enter")
	if h.app.screen != dashScreen || !strings.Contains(h.screen(), "Reviews  0") {
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
	h.app.rdeps.Disabled = "Reviews against the live API need --live."
	h.key("r")
	if h.app.screen != dashScreen || !strings.Contains(h.screen(), "need --live") {
		t.Errorf("expected a notice on the dashboard:\n%s", h.screen())
	}
	if len(h.mock.Writes()) != 0 {
		t.Error("nothing should be written")
	}
}
