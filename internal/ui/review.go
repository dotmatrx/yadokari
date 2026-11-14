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

var typeName = map[string]string{
	wk.ObjectRadical: "Radical", wk.ObjectKanji: "Kanji",
	wk.ObjectVocabulary: "Vocabulary", wk.ObjectKanaVocabulary: "Vocabulary",
}

// ReviewDeps are what the review screen needs beyond Deps.
type ReviewDeps struct {
	Renderer gfx.Renderer
	Font     *glyph.Font // nil: no font found, glyphs fall back to plain text
	// Roulette, when set, gives each question a random font (kitty tier).
	Roulette *glyph.Pool
	Images   glyph.ImageCache
	Uploader *wksync.Uploader
	Settings Settings // from the config file
	// Disabled, when set, explains why reviews can't start (shown on the
	// dashboard instead of opening the review screen).
	Disabled string

	// Audio plays pronunciations from files Sounds downloads; nil Audio
	// means no player was found (AudioErr says why).
	Audio    AudioPlayer
	Sounds   Downloads
	AudioErr error

	// shared is the App's state (graphics ready, audio): set by NewApp.
	shared *shared
}

// graphicsReady reports whether glyphs can be drawn as kitty images yet;
// until then they draw as a blank block.
func (rd ReviewDeps) graphicsReady() bool { return rd.shared != nil && rd.shared.graphicsReady }

// page is what an item page needs: its glyph's renderer and the shared state.
func (rd ReviewDeps) page() PageDeps {
	return PageDeps{Renderer: rd.Renderer, Font: rd.Font, shared: rd.shared}
}

type reviewState int

const (
	answering reviewState = iota
	graded
)

// Review is the review session screen.
type Review struct {
	name   string         // the persisted session's name: reviews, or a lesson quiz
	prefs  wk.Preferences // autoplay
	deps   Deps
	rdeps  ReviewDeps
	sess   *session.Session
	items  []store.ReviewItem // indexed like the session's items
	resume bool

	width, height int

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
	item       *ItemPage // open over the review after an answer (i)
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

	// qfont is the roulette's font for the question on screen, picked when
	// it is first drawn and kept until the next question (an undo keeps it).
	qfont   *glyph.Font
	fontRng *rand.Rand
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
	r := newReview(deps, rdeps, sessionName)
	r.prefs = rdeps.Settings.prefs(user.Preferences)

	var snap session.Snapshot
	if ok, err := deps.Store.LoadSession(ctx, sessionName, &snap); err == nil && ok {
		if sess, ordered, ok := resumable(snap, byAssignment, rng); ok {
			r.sess, r.items, r.resume = sess, ordered, true
			return r, nil
		}
		_ = deps.Store.ClearSession(ctx, sessionName)
	}

	if len(items) == 0 {
		return nil, userError("no reviews are due")
	}
	opts := rdeps.Settings.sessionOptions(user.Preferences)
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

func newReview(deps Deps, rdeps ReviewDeps, name string) *Review {
	return &Review{name: name, deps: deps, rdeps: rdeps, images: map[int][]byte{}, loading: map[int]bool{}, imgErr: map[int]error{},
		fontRng: rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0x2545f4914f6cdd1d))}
}

// lessons reports whether this is a lesson quiz: a finished item is started
// on WaniKani rather than reviewed, and misses cost nothing.
func (r *Review) lessons() bool { return r.name == lessonSessionName }

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
	return tea.Batch(r.fetchImage(), r.upload())
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
		if r.item != nil {
			r.item.SetSize(msg.Width, msg.Height)
		}
	case ItemClosedMsg:
		r.item = nil
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
			r.upErr = problem("upload", msg.err)
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
	return r, tea.Batch(cmds...)
}

// DrawGlyph draws the glyph on screen (the item page's, when open) so the
// renderer queues any upload. Only App calls it, then sends the queue.
func (r *Review) DrawGlyph() {
	if r.item != nil {
		r.item.DrawGlyph()
		return
	}
	if _, it, ok := r.shown(); ok {
		cols, rows := r.glyphSize()
		_, _ = r.drawGlyph(it, cols, rows, typeColour[it.Subject.Object])
	}
}

func (r *Review) key(msg tea.KeyPressMsg) tea.Cmd {
	if r.item != nil {
		return r.item.Key(msg)
	}
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
	case "i":
		if r.state == graded {
			r.openItem()
			return nil
		}
	case "p":
		if r.state == graded && r.audible() {
			r.warn = ""
			return playAudio(r.answeredIt.Subject)
		}
	case "enter":
		return r.enter()
	case "backspace":
		if r.state == graded && r.sess.CanUndo() {
			r.undo()
			return nil
		}
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

// undo takes back a wrong answer (allow_undo): the question comes back with
// the answer as typed, ready to edit, and the miss is forgotten.
func (r *Review) undo() {
	if err := r.sess.Undo(); err != nil {
		return
	}
	r.wrong--
	r.state, r.info, r.warn = answering, false, ""
	r.save()
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
		r.qfont = nil // the next question draws its own
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
		res = answer.CheckMeaning(string(r.plain), it.Subject.Data, it.Synonyms, r.rdeps.Settings.strictness())
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
		r.warn = problem("answer", err)
		return nil
	}
	var cmd tea.Cmd
	if done != nil && r.lessons() {
		if err := r.deps.Store.QueueStart(context.Background(), done.AssignmentID, done.SubjectID, done.CompletedAt.UTC()); err != nil {
			r.upErr = problem("queueing", err)
		}
		cmd = r.upload()
	} else if done != nil {
		created := done.CompletedAt.UTC()
		err := r.deps.Store.QueueReview(context.Background(), done.SubjectID, wk.ReviewInput{
			AssignmentID:            done.AssignmentID,
			IncorrectMeaningAnswers: done.IncorrectMeaningAnswers,
			IncorrectReadingAnswers: done.IncorrectReadingAnswers,
			CreatedAt:               &created,
		})
		if err != nil {
			r.upErr = problem("queueing", err)
		}
		cmd = r.upload()
	}
	r.save()
	if correct && q.Kind == session.Reading && r.autoplay() {
		cmd = tea.Batch(cmd, playAudio(it.Subject))
	}
	return cmd
}

// autoplay follows the user's WaniKani setting for this kind of session.
func (r *Review) autoplay() bool {
	if r.lessons() {
		return r.prefs.LessonsAutoplayAudio
	}
	return r.prefs.ReviewsAutoplayAudio
}

// audible reports whether the answered item has a recording that may be
// played now: its reading is no secret (asked already, or kana vocabulary).
func (r *Review) audible() bool {
	it := r.answeredIt.Subject
	if !hasAudio(it) {
		return false
	}
	_, reading := r.revealed(r.answered)
	return reading || it.Object == wk.ObjectKanaVocabulary
}

func (r *Review) save() {
	ctx := context.Background()
	if r.sess.Done() {
		_ = r.deps.Store.ClearSession(ctx, r.name)
		return
	}
	_ = r.deps.Store.SaveSession(ctx, r.name, r.sess.Snapshot())
}

func (r *Review) finish() tea.Cmd {
	ctx := context.Background()
	_ = r.deps.Store.ClearSession(ctx, r.name)
	sum := Summary{Pending: r.pending, Held: r.held, Lessons: r.lessons()}
	// Completed items may predate a resume, so look subjects up in the store.
	for _, it := range r.sess.Completed() {
		subj, err := r.deps.Store.Subject(ctx, it.SubjectID)
		if err != nil || subj == nil {
			continue
		}
		line := SummaryItem{Characters: characters(*subj), Meaning: primaryMeaning(subj.Data), Type: subj.Object,
			AssignmentID: it.AssignmentID, From: it.SRSStage}
		if r.lessons() || (it.MeaningMisses == 0 && it.ReadingMisses == 0) {
			sum.Correct = append(sum.Correct, line)
		} else {
			sum.Incorrect = append(sum.Incorrect, line)
		}
	}
	_ = sum.Refresh(ctx, r.deps.Store)
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

// glyphSize is the glyph field: the full width, half the rows between
// header and footer.
func (r *Review) glyphSize() (cols, rows int) {
	return max(r.width, 20), max((r.height-2)/2, 5)
}

func (r *Review) source(it store.ReviewItem) gfx.Source {
	return subjectSource(it.Subject, r.font(it), r.images[it.Subject.ID])
}

// font is the font the question on screen is drawn in: the roulette's
// pick for it (one with all its characters), else the default font.
func (r *Review) font(it store.ReviewItem) *glyph.Font {
	if r.rdeps.Roulette == nil || it.Subject.Data.Characters == nil {
		return r.rdeps.Font
	}
	if r.qfont == nil {
		r.qfont = r.rdeps.Roulette.Pick(r.fontRng, *it.Subject.Data.Characters)
		if r.qfont == nil {
			r.qfont = r.rdeps.Font
		}
	}
	return r.qfont
}

// fontLine names the roulette's font under the glyph once the question is
// answered (blank otherwise, and for radicals drawn from images).
func (r *Review) fontLine(it store.ReviewItem, w int) string {
	if r.state != graded || r.rdeps.Roulette == nil || r.qfont == nil || it.Subject.Data.Characters == nil {
		return ""
	}
	return lipgloss.PlaceHorizontal(w, lipgloss.Center, mutedSt.Render(r.qfont.Family))
}

// subjectSource is what the renderer draws for a subject: its characters in
// the font, or its radical image (svg, nil while it loads).
func subjectSource(subj wk.Resource[wk.Subject], font *glyph.Font, svg []byte) gfx.Source {
	src := gfx.Source{Key: fmt.Sprintf("subject:%d", subj.ID)}
	if subj.Data.Characters != nil {
		if font != nil {
			src.Key += ":" + font.Name()
		}
		text := *subj.Data.Characters
		src.Text = text
		src.Mask = func(w, h int) (*image.Alpha, error) {
			if font == nil {
				return nil, glyph.ErrNoFont
			}
			return font.Text(text, w, h)
		}
		return src
	}
	src.Key += ":image"
	src.Mask = func(w, h int) (*image.Alpha, error) {
		if svg == nil {
			return nil, errors.New("image not loaded")
		}
		return glyph.ImageMask(svg, "image/svg+xml", w, h)
	}
	return src
}

func (r *Review) View() string {
	w, h := max(r.width, 40), max(r.height, 12)
	help := "Enter submit · Esc dashboard · ctrl+w wrap-up"
	if r.state == graded {
		help = "Enter next · Tab answer · i item · Esc dashboard · ctrl+w wrap-up"
		if r.audible() {
			help = "Enter next · Tab answer · i item · p audio · Esc dashboard · ctrl+w wrap-up"
		}
		if r.sess.CanUndo() {
			help = "Backspace retype · " + help
		}
	}
	if r.item != nil {
		return r.item.View()
	}
	header, footer := r.header(w), padRight(" "+mutedSt.Render(help), w)
	q, it, ok := r.shown()
	if !ok {
		return frame(header, "\n "+"Session complete. Press Enter.", footer, h)
	}

	var b []string
	cols, rows := r.glyphSize()
	bg := typeColour[it.Subject.Object]
	cells, err := r.drawGlyph(it, cols, rows, bg)
	if err != nil {
		cells, _ = gfx.New(gfx.Plain, 0, 0).Draw(r.source(it), cols, rows, glyphFG, bg)
	}
	b = append(b, cells, r.fontLine(it, w))

	left, bw := answerBox(w)
	indent := strings.Repeat(" ", left)
	for _, line := range strings.Split(r.answerPanel(q, it, bw), "\n") {
		b = append(b, indent+line)
	}
	switch {
	case r.warn != "":
		b = append(b, lipgloss.PlaceHorizontal(w, lipgloss.Center, warnSt.Render("⚠ "+r.warn)))
	case r.state == graded && r.result.Inexact:
		b = append(b, lipgloss.PlaceHorizontal(w, lipgloss.Center, warnSt.Render(r.result.Message)))
	default:
		b = append(b, "")
	}
	if r.info {
		for _, line := range strings.Split(r.infoPanel(q, it, bw), "\n") {
			b = append(b, indent+line)
		}
	}
	return frame(header, strings.Join(b, "\n"), footer, h)
}

// header: the app name left; progress, accuracy and uploads right.
func (r *Review) header(w int) string {
	st := r.sess.Stats()
	segs := []segment{{text: mutedSt.Render(fmt.Sprintf("%d / %d", st.Done, st.Total)), rank: rankKey}}
	if total := r.correct + r.wrong; total > 0 {
		segs = append(segs, segment{text: mutedSt.Render(fmt.Sprintf("%d%% correct", r.correct*100/total)), rank: rankExtra})
	}
	if r.sess.WrappingUp() {
		segs = append(segs, segment{text: warnSt.Render("wrapping up"), rank: rankInfo})
	}
	if u := r.uploadStatus(); u != "" {
		rank := rankInfo
		if r.upErr != "" {
			rank = rankUrgent
		}
		segs = append(segs, segment{text: mutedSt.Render(u), rank: rank})
	}
	return headerLine(appTitle(r.name), segs, w, r.rdeps.shared)
}

// answerBox is where the answer box sits: its left column and width.
func answerBox(w int) (left, width int) {
	width = min(w-2, 64)
	return (w - width) / 2, width
}

// promptKind names what is asked: "meaning", "reading", or a radical's
// "name".
func promptKind(q session.Question, it store.ReviewItem) string {
	switch {
	case it.Subject.Object == wk.ObjectRadical:
		return "name"
	case q.Kind == session.Reading:
		return "reading"
	}
	return "meaning"
}

// inputText is the typed text as shown in the box, and whether nothing has
// been typed yet.
func (r *Review) inputText(q session.Question) (text string, empty bool) {
	if q.Kind == session.Reading {
		return r.conv.Committed() + pendingSt.Render(r.conv.Pending()), r.conv.String() == ""
	}
	return string(r.plain), len(r.plain) == 0
}

// badgeSt draws the input mode's letter as a small badge in the theme's
// grey (the borders' colour): the one tiny painted block besides the glyph.

// inputMode says what the box expects, for the right of its top border: a
// badge with the mode's letter, then its name.
func inputMode(q session.Question) string {
	if q.Kind == session.Reading {
		return badgeSt.Render("あ") + mutedSt.Render(" Kana")
	}
	return badgeSt.Render("A") + mutedSt.Render(" English")
}

// placeholder fills an empty answer box, as on WaniKani's site.
func placeholder(q session.Question) string {
	if q.Kind == session.Reading {
		return "答え"
	}
	return "Your answer"
}

// answerPanel is the answer box: the question as its title, the typed
// answer inside, and after grading a coloured border and ✓ or ✗.
func (r *Review) answerPanel(q session.Question, it store.ReviewItem, width int) string {
	title := mutedSt.Render(typeName[it.Subject.Object]+" ") + titleSt.Render(promptKind(q, it))
	line, empty := r.inputText(q)
	if empty && r.state == answering {
		line = mutedSt.Render(placeholder(q))
	}
	border := lineSt
	if r.state == graded {
		mark := correctSt.Render("✓")
		border = correctSt
		if r.result.Verdict != answer.Correct {
			mark, border = wrongSt.Render("✗"), wrongSt
		}
		line = spread(line, mark, width-4)
	}
	return panelWith(border, title, inputMode(q), line, width, 0)
}

func (r *Review) drawGlyph(it store.ReviewItem, cols, rows int, bg color.Color) (string, error) {
	missing := it.Subject.Data.Characters == nil && r.images[it.Subject.ID] == nil
	return drawSubjectGlyph(r.rdeps.Renderer, r.rdeps.graphicsReady(), r.source(it), missing, cols, rows, bg)
}

// drawSubjectGlyph draws a glyph field: a blank coloured block until kitty
// uploads can be sent, an error while a radical image is missing (the
// caller falls back to plain text).
func drawSubjectGlyph(rd gfx.Renderer, ready bool, src gfx.Source, missing bool, cols, rows int, bg color.Color) (string, error) {
	if rd == nil || (rd.Tier() == gfx.Kitty && !ready) {
		return strings.TrimSuffix(strings.Repeat(gfx.BgSGR(bg)+strings.Repeat(" ", cols)+"\x1b[m\n", rows), "\n"), nil
	}
	if missing {
		return "", errors.New("image not loaded")
	}
	return rd.Draw(src, cols, rows, glyphFG, bg)
}

func (r *Review) uploadStatus() string {
	switch {
	case r.upErr != "":
		return fmt.Sprintf("%d waiting to upload: %s", r.pending, r.upErr)
	case r.held && r.pending > 0:
		return fmt.Sprintf("%d held (dev build)", r.pending)
	case r.uploading:
		return "uploading…"
	case r.pending > 0:
		return fmt.Sprintf("%d to upload", r.pending)
	}
	return ""
}

// openItem opens the item page for the question just answered, hiding a
// part not yet asked as the answer panel does.
func (r *Review) openItem() {
	q, it := r.answered, r.answeredIt
	d, err := LoadItem(context.Background(), r.deps.Store, it.Subject.ID)
	if err != nil {
		r.warn = problem("opening item", err)
		return
	}
	d.Synonyms = it.Synonyms
	showMeaning, showReading := r.revealed(q)
	p := NewItemPage(d, r.rdeps.page(), r.images[it.Subject.ID])
	p.HideMeaning, p.HideReading = !showMeaning, !showReading
	p.SetSize(r.width, r.height)
	r.item = p
}

// revealed reports which parts of the question's item may be shown: the
// part just asked, and any part already attempted.
func (r *Review) revealed(q session.Question) (meaning, reading bool) {
	state := r.sess.Item(q.Index)
	meaning = q.Kind == session.Meaning || state.MeaningDone || state.MeaningMisses > 0
	reading = q.Kind == session.Reading || state.ReadingDone || state.ReadingMisses > 0
	return meaning, reading
}

// infoPanel shows the answer for the part just asked, plus any part already
// attempted (answered or missed, so its answer is no secret). A part not yet
// asked stays hidden so it can still be recalled when its question comes up.
func (r *Review) infoPanel(q session.Question, it store.ReviewItem, width int) string {
	d := it.Subject.Data
	showMeaning, showReading := r.revealed(q)
	var lines []string
	if showMeaning {
		var meanings []string
		for _, m := range d.Meanings {
			if m.AcceptedAnswer {
				meanings = append(meanings, m.Meaning)
			}
		}
		meanings = append(meanings, it.Synonyms...)
		lines = append(lines, boldSt.Render("Meaning  ")+strings.Join(meanings, ", "))
	}
	if showReading && len(d.Readings) > 0 {
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
	return panel(titleSt.Render("Answer"), strings.Join(lines, "\n"), width, 0)
}

// Cursor returns where the terminal cursor belongs (after the typed text),
// or nil when not answering.
func (r *Review) Cursor() *tea.Cursor {
	q, _, ok := r.current()
	if !ok || r.state != answering {
		return nil
	}
	w := max(r.width, 40)
	left, _ := answerBox(w)
	text, empty := r.inputText(q)
	if empty {
		text = "" // the cursor sits at the start of the placeholder
	}
	_, rows := r.glyphSize()
	x := left + 2 + lipgloss.Width(text) // border, space, then the text
	y := 1 + rows + 1 + 1                // header, glyph, blank, box top
	c := tea.NewCursor(x, y)
	c.Shape = tea.CursorBar
	return c
}
