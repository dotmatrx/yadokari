package ui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math/rand/v2"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jtharris/yadokari/internal/answer"
	"github.com/jtharris/yadokari/internal/gfx"
	"github.com/jtharris/yadokari/internal/glyph"
	"github.com/jtharris/yadokari/internal/ime"
	"github.com/jtharris/yadokari/internal/session"
	"github.com/jtharris/yadokari/internal/store"
	"github.com/jtharris/yadokari/internal/wk"
	"github.com/jtharris/yadokari/internal/wksync"
)

// sessionName keys the persisted review session in the store.
const sessionName = "reviews"

// Type colours (WaniKani's), used for the glyph field.
var typeColour = map[string]color.RGBA{
	wk.ObjectRadical:        {0x00, 0xaa, 0xff, 0xff},
	wk.ObjectKanji:          {0xff, 0x00, 0xaa, 0xff},
	wk.ObjectVocabulary:     {0xaa, 0x00, 0xff, 0xff},
	wk.ObjectKanaVocabulary: {0xaa, 0x00, 0xff, 0xff},
}

var glyphFG = color.RGBA{0xff, 0xff, 0xff, 0xff}

var typeName = map[string]string{
	wk.ObjectRadical: "Radical", wk.ObjectKanji: "Kanji",
	wk.ObjectVocabulary: "Vocabulary", wk.ObjectKanaVocabulary: "Vocabulary",
}

var (
	meaningBar = lipgloss.NewStyle().Background(lipgloss.Color("#e9e9e9")).Foreground(lipgloss.Color("#333333")).Bold(true)
	readingBar = lipgloss.NewStyle().Background(lipgloss.Color("#333333")).Foreground(lipgloss.Color("#ffffff")).Bold(true)
	inputSt    = lipgloss.NewStyle().Padding(0, 1)
	correctSt  = inputSt.Background(lipgloss.Color("#88cc00")).Foreground(lipgloss.Color("#ffffff"))
	wrongSt    = inputSt.Background(lipgloss.Color("#ff0033")).Foreground(lipgloss.Color("#ffffff"))
	warnSt     = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffcc00")).Bold(true)
	pendingSt  = lipgloss.NewStyle().Underline(true)
)

// ReviewDeps are what the review screen needs beyond Deps.
type ReviewDeps struct {
	Renderer   gfx.Renderer
	Font       *glyph.Font // nil: no font found, glyphs fall back to plain text
	Images     glyph.ImageCache
	Uploader   *wksync.Uploader
	Strictness answer.Strictness
	// Disabled, when set, explains why reviews can't start (shown on the
	// dashboard instead of opening the review screen).
	Disabled string
}

type reviewState int

const (
	answering reviewState = iota
	graded
)

// Review is the review session screen.
type Review struct {
	deps   Deps
	rdeps  ReviewDeps
	sess   *session.Session
	items  []store.ReviewItem // indexed like the session's items
	resume bool

	width, height int
	ready         bool // alt screen active: safe to send kitty uploads

	conv  ime.Converter
	plain []rune
	state reviewState
	// answered is the question just graded; it stays on screen until Enter,
	// even when it finished the session.
	answered   session.Question
	answeredIt store.ReviewItem
	result     answer.Result
	warn       string
	info       bool
	correct    int
	wrong      int

	images  map[int][]byte // subject ID → SVG bytes
	loading map[int]bool
	imgErr  map[int]error

	uploading bool
	again     bool // another flush requested while one runs
	pending   int
	held      bool
	upErr     string
}

type (
	imageMsg struct {
		subject int
		raw     []byte
		err     error
	}
	uploadMsg struct {
		res     wksync.UploadResult
		err     error
		pending int
	}
	reviewReadyMsg struct{}
	// ReviewDoneMsg asks the app to show the summary.
	ReviewDoneMsg struct{ Summary Summary }
	// BackMsg asks the app to return to the dashboard.
	BackMsg struct{}
)

// LoadReview builds a review session from the store, resuming a saved one
// if its items are still due.
func LoadReview(ctx context.Context, deps Deps, rdeps ReviewDeps, user wk.User) (*Review, error) {
	items, err := deps.Store.ReviewItems(ctx, deps.Now(), user.Subscription.MaxLevelGranted)
	if err != nil {
		return nil, err
	}
	byAssignment := map[int]store.ReviewItem{}
	for _, it := range items {
		byAssignment[it.Assignment.ID] = it
	}
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0x9e3779b97f4a7c15))
	r := &Review{deps: deps, rdeps: rdeps, images: map[int][]byte{}, loading: map[int]bool{}, imgErr: map[int]error{}}

	var snap session.Snapshot
	if ok, err := deps.Store.LoadSession(ctx, sessionName, &snap); err == nil && ok {
		if sess, ordered, ok := resumable(snap, byAssignment, rng); ok {
			r.sess, r.items, r.resume = sess, ordered, true
			return r, nil
		}
		_ = deps.Store.ClearSession(ctx, sessionName)
	}

	if len(items) == 0 {
		return nil, errors.New("no reviews are due")
	}
	opts := session.Options{}
	if user.Preferences.ReviewsPresentationOrder == "lower_levels_first" {
		opts.Order = session.LowerLevelsFirst
	}
	sItems := make([]session.Item, len(items))
	for i, it := range items {
		sItems[i] = session.Item{
			AssignmentID: it.Assignment.ID, SubjectID: it.Subject.ID, SubjectType: it.Subject.Object,
			Level: it.Subject.Data.Level, SRSStage: it.Assignment.Data.SRSStage,
		}
	}
	r.sess, r.items = session.New(sItems, opts, rng), items
	return r, nil
}

// resumable restores a snapshot when every item still in play is still due.
func resumable(snap session.Snapshot, due map[int]store.ReviewItem, rng *rand.Rand) (*session.Session, []store.ReviewItem, bool) {
	sess, err := session.Restore(snap, rng)
	if err != nil || sess.Done() {
		return nil, nil, false
	}
	items := make([]store.ReviewItem, len(snap.Items))
	inPlay := append(append([]int{}, snap.Active...), snap.Backlog...)
	for _, i := range inPlay {
		it, ok := due[snap.Items[i].AssignmentID]
		if !ok {
			return nil, nil, false
		}
		items[i] = it
	}
	return sess, items, true
}

func (r *Review) Init() tea.Cmd {
	return tea.Batch(
		tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return reviewReadyMsg{} }),
		r.fetchImage(),
		r.upload(),
	)
}

// shown is the question on screen: the graded one until Enter, else current.
func (r *Review) shown() (session.Question, store.ReviewItem, bool) {
	if r.state == graded {
		return r.answered, r.answeredIt, true
	}
	return r.current()
}

func (r *Review) current() (session.Question, store.ReviewItem, bool) {
	q, ok := r.sess.Current()
	if !ok {
		return q, store.ReviewItem{}, false
	}
	return q, r.items[q.Index], true
}

// fetchImage loads the current radical's SVG if it has no characters.
func (r *Review) fetchImage() tea.Cmd {
	_, it, ok := r.current()
	if !ok || it.Subject.Data.Characters != nil {
		return nil
	}
	id := it.Subject.ID
	if r.images[id] != nil || r.loading[id] || r.imgErr[id] != nil {
		return nil
	}
	url, _ := glyph.PickImage(it.Subject.Data.CharacterImages)
	if url == "" {
		r.imgErr[id] = errors.New("no image")
		return nil
	}
	r.loading[id] = true
	cache := r.rdeps.Images
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		raw, err := cache.Get(ctx, url)
		return imageMsg{subject: id, raw: raw, err: err}
	}
}

// upload flushes the outbox in the background, one flush at a time.
func (r *Review) upload() tea.Cmd {
	if r.rdeps.Uploader == nil {
		return nil
	}
	if r.uploading {
		r.again = true
		return nil
	}
	r.uploading = true
	up, st := r.rdeps.Uploader, r.deps.Store
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		res, err := up.Flush(ctx)
		pending, _, _ := st.OutboxCounts(ctx)
		return uploadMsg{res: res, err: err, pending: pending}
	}
}

func (r *Review) Update(msg tea.Msg) (*Review, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.width, r.height = msg.Width, msg.Height
	case reviewReadyMsg:
		r.ready = true
	case imageMsg:
		delete(r.loading, msg.subject)
		if msg.err != nil {
			r.imgErr[msg.subject] = msg.err
		} else {
			r.images[msg.subject] = msg.raw
		}
	case uploadMsg:
		r.uploading, r.pending = false, msg.pending
		r.held = errors.Is(msg.err, wksync.ErrHeld)
		r.upErr = ""
		if msg.err != nil && !r.held {
			r.upErr = msg.err.Error()
		}
		if r.again {
			r.again = false
			cmds = append(cmds, r.upload())
		}
	case tea.PasteMsg:
		r.insert(msg.Content)
	case tea.KeyPressMsg:
		cmd := r.key(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	cmds = append(cmds, r.flushGraphics())
	return r, tea.Batch(cmds...)
}

// flushGraphics draws the current glyph so any kitty upload is queued, then
// sends the queue. Called from Update so uploads never wait on a View.
func (r *Review) flushGraphics() tea.Cmd {
	if !r.ready || r.rdeps.Renderer == nil {
		return nil
	}
	if _, it, ok := r.shown(); ok {
		cols, rows := r.glyphSize()
		_, _ = r.rdeps.Renderer.Draw(r.source(it), cols, rows, glyphFG, typeColour[it.Subject.Object])
	}
	if s := r.rdeps.Renderer.Flush(); s != "" {
		return tea.Raw(s)
	}
	return nil
}

func (r *Review) key(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		return func() tea.Msg { return BackMsg{} }
	case "ctrl+w":
		r.sess.WrapUp(!r.sess.WrappingUp())
		r.save()
		return nil
	case "tab":
		if r.state == graded {
			r.info = !r.info
		}
		return nil
	case "enter":
		return r.enter()
	case "backspace":
		if r.state == answering {
			r.warn = ""
			if q, _, ok := r.current(); ok && q.Kind == session.Reading {
				r.conv.Backspace()
			} else if len(r.plain) > 0 {
				r.plain = r.plain[:len(r.plain)-1]
			}
		}
		return nil
	case "ctrl+u":
		if r.state == answering {
			r.conv.Reset()
			r.plain = nil
		}
		return nil
	}
	if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
		r.insert(msg.Text)
	}
	return nil
}

func (r *Review) insert(s string) {
	if r.state != answering {
		return
	}
	r.warn = ""
	if q, _, ok := r.current(); ok && q.Kind == session.Reading {
		r.conv.TypeString(s)
	} else {
		r.plain = append(r.plain, []rune(s)...)
	}
}

func (r *Review) enter() tea.Cmd {
	if r.state == graded {
		r.state, r.info, r.warn = answering, false, ""
		r.conv.Reset()
		r.plain = nil
		if r.sess.Done() {
			return r.finish()
		}
		return r.fetchImage()
	}

	q, it, ok := r.current()
	if !ok {
		return r.finish()
	}
	var res answer.Result
	if q.Kind == session.Reading {
		res = answer.CheckReading(r.conv.Finish(), it.Subject.Object, it.Subject.Data)
	} else {
		res = answer.CheckMeaning(string(r.plain), it.Subject.Data, it.Synonyms, r.rdeps.Strictness)
	}
	if res.Verdict == answer.Retry {
		r.warn = res.Message
		return nil
	}

	r.result, r.state = res, graded
	r.answered, r.answeredIt = q, it
	correct := res.Verdict == answer.Correct
	if correct {
		r.correct++
	} else {
		r.wrong++
		r.info = true
	}
	done, err := r.sess.Answer(correct, r.deps.Now())
	if err != nil {
		r.warn = err.Error()
		return nil
	}
	var cmd tea.Cmd
	if done != nil {
		created := done.CompletedAt.UTC()
		err := r.deps.Store.QueueReview(context.Background(), done.SubjectID, wk.ReviewInput{
			AssignmentID:            done.AssignmentID,
			IncorrectMeaningAnswers: done.IncorrectMeaningAnswers,
			IncorrectReadingAnswers: done.IncorrectReadingAnswers,
			CreatedAt:               &created,
		})
		if err != nil {
			r.upErr = err.Error()
		}
		cmd = r.upload()
	}
	r.save()
	return cmd
}

func (r *Review) save() {
	ctx := context.Background()
	if r.sess.Done() {
		_ = r.deps.Store.ClearSession(ctx, sessionName)
		return
	}
	_ = r.deps.Store.SaveSession(ctx, sessionName, r.sess.Snapshot())
}

func (r *Review) finish() tea.Cmd {
	ctx := context.Background()
	_ = r.deps.Store.ClearSession(ctx, sessionName)
	sum := Summary{Pending: r.pending, Held: r.held}
	// Completed items may predate a resume, so look subjects up in the store.
	for _, it := range r.sess.Completed() {
		subj, err := r.deps.Store.Subject(ctx, it.SubjectID)
		if err != nil || subj == nil {
			continue
		}
		line := SummaryItem{Characters: characters(*subj), Meaning: primaryMeaning(subj.Data), Type: subj.Object}
		if it.MeaningMisses == 0 && it.ReadingMisses == 0 {
			sum.Correct = append(sum.Correct, line)
		} else {
			sum.Incorrect = append(sum.Incorrect, line)
		}
	}
	return func() tea.Msg { return ReviewDoneMsg{Summary: sum} }
}

func characters(s wk.Resource[wk.Subject]) string {
	if s.Data.Characters != nil {
		return *s.Data.Characters
	}
	return "（" + s.Data.Slug + "）"
}

func primaryMeaning(s wk.Subject) string {
	for _, m := range s.Meanings {
		if m.Primary {
			return m.Meaning
		}
	}
	if len(s.Meanings) > 0 {
		return s.Meanings[0].Meaning
	}
	return ""
}

// glyphSize is the glyph field: the full width, about 40% of the height.
func (r *Review) glyphSize() (cols, rows int) {
	return max(r.width, 20), max(r.height*2/5, 5)
}

func (r *Review) source(it store.ReviewItem) gfx.Source {
	subj := it.Subject
	src := gfx.Source{Key: fmt.Sprintf("subject:%d", subj.ID)}
	if subj.Data.Characters != nil {
		text := *subj.Data.Characters
		src.Text = text
		font := r.rdeps.Font
		src.Mask = func(w, h int) (*image.Alpha, error) {
			if font == nil {
				return nil, glyph.ErrNoFont
			}
			return font.Text(text, w, h)
		}
		return src
	}
	raw := r.images[subj.ID]
	src.Key += ":image"
	src.Mask = func(w, h int) (*image.Alpha, error) {
		if raw == nil {
			return nil, errors.New("image not loaded")
		}
		return glyph.ImageMask(raw, "image/svg+xml", w, h)
	}
	return src
}

func (r *Review) View() string {
	q, it, ok := r.shown()
	if !ok {
		return "\n  Session complete. Press Enter."
	}
	width := max(r.width, 40)
	var b strings.Builder

	// Header: progress and upload status.
	st := r.sess.Stats()
	acc := "—"
	if total := r.correct + r.wrong; total > 0 {
		acc = fmt.Sprintf("%d%%", r.correct*100/total)
	}
	left := boldSt.Render(fmt.Sprintf(" %d / %d", st.Done, st.Total)) + mutedSt.Render("  accuracy "+acc)
	if r.sess.WrappingUp() {
		left += warnSt.Render("  wrapping up")
	}
	right := mutedSt.Render(r.uploadStatus() + " ")
	b.WriteString(left + strings.Repeat(" ", max(width-lipgloss.Width(left)-lipgloss.Width(right), 1)) + right + "\n")

	// Glyph field.
	cols, rows := r.glyphSize()
	bg := typeColour[it.Subject.Object]
	cells, err := r.drawGlyph(it, cols, rows, bg)
	if err != nil {
		cells, _ = gfx.New(gfx.Plain, 0, 0).Draw(r.source(it), cols, rows, glyphFG, bg)
	}
	b.WriteString(cells + "\n")

	// Prompt bar.
	bar := meaningBar
	kind := "Meaning"
	if q.Kind == session.Reading {
		bar, kind = readingBar, "Reading"
	}
	if it.Subject.Object == wk.ObjectRadical {
		kind = "Name"
	}
	b.WriteString(bar.Width(width).Align(lipgloss.Center).Render(typeName[it.Subject.Object]+" "+kind) + "\n\n")

	// Input line.
	mode := "A"
	text := string(r.plain)
	if q.Kind == session.Reading {
		mode = "あ"
		text = r.conv.Committed() + pendingSt.Render(r.conv.Pending())
	}
	line := inputSt.Render(mode + "  " + text)
	switch {
	case r.state == graded && r.result.Verdict == answer.Correct:
		line = correctSt.Render(mode + "  " + text + "  ✓")
	case r.state == graded:
		line = wrongSt.Render(mode + "  " + text + "  ✗")
	}
	b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, line) + "\n")

	// Feedback.
	switch {
	case r.warn != "":
		b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, warnSt.Render("⚠ "+r.warn)) + "\n")
	case r.state == graded && r.result.Inexact:
		b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, warnSt.Render(r.result.Message)) + "\n")
	default:
		b.WriteString("\n")
	}
	if r.info {
		b.WriteString("\n" + r.infoPanel(it, width))
	}
	help := "Enter submit · Esc dashboard · ctrl+w wrap-up"
	if r.state == graded {
		help = "Enter next · Tab answer · Esc dashboard · ctrl+w wrap-up"
	}
	b.WriteString("\n" + mutedSt.Render(" "+help))
	return b.String()
}

func (r *Review) drawGlyph(it store.ReviewItem, cols, rows int, bg color.Color) (string, error) {
	if r.rdeps.Renderer == nil || (r.rdeps.Renderer.Tier() == gfx.Kitty && !r.ready) {
		return strings.TrimSuffix(strings.Repeat(bgSGR(bg)+strings.Repeat(" ", cols)+"\x1b[m\n", rows), "\n"), nil
	}
	if it.Subject.Data.Characters == nil && r.images[it.Subject.ID] == nil {
		return "", errors.New("image not loaded")
	}
	return r.rdeps.Renderer.Draw(r.source(it), cols, rows, glyphFG, bg)
}

func bgSGR(c color.Color) string {
	cr, cg, cb, _ := c.RGBA()
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", cr>>8, cg>>8, cb>>8)
}

func (r *Review) uploadStatus() string {
	switch {
	case r.upErr != "":
		return "upload problem: " + shortErr(errors.New(r.upErr))
	case r.held && r.pending > 0:
		return fmt.Sprintf("%d held (dry-run)", r.pending)
	case r.uploading:
		return "uploading…"
	case r.pending > 0:
		return fmt.Sprintf("%d to upload", r.pending)
	}
	return ""
}

func (r *Review) infoPanel(it store.ReviewItem, width int) string {
	d := it.Subject.Data
	var meanings []string
	for _, m := range d.Meanings {
		if m.AcceptedAnswer {
			meanings = append(meanings, m.Meaning)
		}
	}
	meanings = append(meanings, it.Synonyms...)
	lines := []string{boldSt.Render("Meaning  ") + strings.Join(meanings, ", ")}
	if len(d.Readings) > 0 {
		var readings []string
		for _, rd := range d.Readings {
			if !rd.AcceptedAnswer {
				continue
			}
			s := rd.Reading
			switch rd.Type {
			case wk.ReadingOnyomi:
				s += mutedSt.Render(" on'yomi")
			case wk.ReadingKunyomi:
				s += mutedSt.Render(" kun'yomi")
			case wk.ReadingNanori:
				s += mutedSt.Render(" nanori")
			}
			readings = append(readings, s)
		}
		lines = append(lines, boldSt.Render("Reading  ")+strings.Join(readings, ", "))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(min(width-4, 80))
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, box.Render(strings.Join(lines, "\n")))
}

// Cursor returns where the terminal cursor belongs (after the typed text),
// or nil when not answering.
func (r *Review) Cursor() *tea.Cursor {
	q, _, ok := r.current()
	if !ok || r.state != answering {
		return nil
	}
	width := max(r.width, 40)
	mode, text := "A", string(r.plain)
	if q.Kind == session.Reading {
		mode, text = "あ", r.conv.String()
	}
	line := inputSt.Render(mode + "  " + text)
	_, rows := r.glyphSize()
	x := (width-lipgloss.Width(line))/2 + lipgloss.Width(inputSt.Render(mode+"  "+text)) - 1
	y := 1 + rows + 1 + 1 // header, glyph, prompt bar, blank line
	c := tea.NewCursor(x, y)
	c.Shape = tea.CursorBar
	return c
}
