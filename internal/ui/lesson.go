package ui

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jtharris/yadokari/internal/glyph"
	"github.com/jtharris/yadokari/internal/session"
	"github.com/jtharris/yadokari/internal/store"
	"github.com/jtharris/yadokari/internal/wk"
)

// lessonSessionName keys the persisted lesson quiz in the store.
const lessonSessionName = "lessons"

// defaultLessonBatch is WaniKani's default batch size.
const defaultLessonBatch = 5

type (
	// StartLessonsMsg asks the app to start a lesson batch.
	StartLessonsMsg struct{ User wk.User }
	lessonLoadedMsg struct {
		lesson *Lesson
		quiz   *Review // set instead of lesson when resuming a quiz
		err    error
	}
	// lessonQuizMsg hands the finished learning phase's quiz to the app.
	lessonQuizMsg  struct{ quiz *Review }
	lessonImageMsg imageMsg
)

// Lesson is the learning phase of a lesson batch: each item's page, tab by
// tab. After the last one comes the quiz (a Review in lesson mode), which
// starts each item on WaniKani once both its parts are answered correctly.
type Lesson struct {
	deps  Deps
	rdeps ReviewDeps
	items []store.ReviewItem // the batch
	idx   int
	page  *ItemPage
	rng   *rand.Rand

	images map[int][]byte // subject ID → SVG, for image-only radicals

	width, height int
}

// LoadLessons resumes a saved lesson quiz whose items are all still
// lessons, or picks a new batch.
func LoadLessons(ctx context.Context, deps Deps, rdeps ReviewDeps, user wk.User) (*Lesson, *Review, error) {
	items, err := deps.Store.LessonItems(ctx, user.Subscription.MaxLevelGranted)
	if err != nil {
		return nil, nil, err
	}
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0x9e3779b97f4a7c15))
	byAssignment := map[int]store.ReviewItem{}
	for _, it := range items {
		byAssignment[it.Assignment.ID] = it
	}
	var snap session.Snapshot
	if ok, err := deps.Store.LoadSession(ctx, lessonSessionName, &snap); err == nil && ok {
		if sess, ordered, ok := resumable(snap, byAssignment, rng); ok {
			q := newReview(deps, rdeps, lessonSessionName)
			q.sess, q.items, q.resume = sess, ordered, true
			return nil, q, nil
		}
		_ = deps.Store.ClearSession(ctx, lessonSessionName)
	}
	if len(items) == 0 {
		return nil, nil, errors.New("no lessons are available")
	}
	l := &Lesson{deps: deps, rdeps: rdeps, rng: rng, images: map[int][]byte{},
		items: pickLessons(items, user.Preferences, rng)}
	if err := l.open(0, 0); err != nil {
		return nil, nil, err
	}
	return l, nil, nil
}

// pickLessons takes a batch in the user's presentation order. items come in
// WaniKani's order (level, then lesson position).
func pickLessons(items []store.ReviewItem, prefs wk.Preferences, rng *rand.Rand) []store.ReviewItem {
	n := prefs.LessonsBatchSize
	if n <= 0 {
		n = defaultLessonBatch
	}
	items = slices.Clone(items)
	switch prefs.LessonsPresentationOrder {
	case "shuffled":
		rng.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	case "ascending_level_then_shuffled":
		for start := 0; start < len(items); {
			end := start
			for end < len(items) && items[end].Subject.Data.Level == items[start].Subject.Data.Level {
				end++
			}
			lvl := items[start:end]
			rng.Shuffle(len(lvl), func(i, j int) { lvl[i], lvl[j] = lvl[j], lvl[i] })
			start = end
		}
	}
	return items[:min(n, len(items))]
}

// open shows item i on the given tab (-1: its last).
func (l *Lesson) open(i, tab int) error {
	it := l.items[i]
	d, err := LoadItem(context.Background(), l.deps.Store, it.Subject.ID)
	if err != nil {
		return err
	}
	p := NewItemPage(d, l.rdeps.glyphs(), l.images[it.Subject.ID])
	p.Section = "lessons"
	p.Strip = l.strip(i)
	p.Progress = fmt.Sprintf("%d / %d", i+1, len(l.items))
	p.Help = "←/→ next · ↑/↓ scroll · Esc dashboard"
	if tab < 0 {
		tab = len(p.tabs) - 1
	}
	p.setTab(tab)
	p.SetSize(l.width, l.height)
	l.idx, l.page = i, p
	return nil
}

// strip is the batch for the header: the current item in its type colour.
func (l *Lesson) strip(cur int) string {
	parts := make([]string, len(l.items))
	for i, it := range l.items {
		c := characters(it.Subject)
		if i == cur {
			parts[i] = lipgloss.NewStyle().Foreground(typeColour[it.Subject.Object]).Bold(true).Render(c)
		} else {
			parts[i] = mutedSt.Render(c)
		}
	}
	return strings.Join(parts, " ")
}

func (l *Lesson) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, it := range l.items {
		if it.Subject.Data.Characters != nil {
			continue
		}
		url, _ := glyph.PickImage(it.Subject.Data.CharacterImages)
		if url == "" {
			continue
		}
		id, cache := it.Subject.ID, l.rdeps.Images
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			raw, err := cache.Get(ctx, url)
			return lessonImageMsg{subject: id, raw: raw, err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (l *Lesson) Update(msg tea.Msg) (*Lesson, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.width, l.height = msg.Width, msg.Height
		l.page.SetSize(msg.Width, msg.Height)
	case lessonImageMsg:
		if msg.err == nil {
			l.images[msg.subject] = msg.raw
			if l.page.data.Subject.ID == msg.subject {
				l.page.svg = msg.raw
			}
		}
	case tea.KeyPressMsg:
		cmd = l.key(msg)
	}
	return l, cmd
}

// DrawGlyph draws the current page's glyph; see Review.DrawGlyph.
func (l *Lesson) DrawGlyph() { l.page.DrawGlyph() }

func (l *Lesson) key(msg tea.KeyPressMsg) tea.Cmd {
	p := l.page
	switch msg.String() {
	case "esc":
		return func() tea.Msg { return BackMsg{} }
	case "right", "l", "tab", "enter":
		switch {
		case p.tab < len(p.tabs)-1:
			p.setTab(p.tab + 1)
		case l.idx < len(l.items)-1:
			return l.openOrWarn(l.idx+1, 0)
		default:
			return l.quiz()
		}
	case "left", "h", "shift+tab":
		switch {
		case p.tab > 0:
			p.setTab(p.tab - 1)
		case l.idx > 0:
			return l.openOrWarn(l.idx-1, -1)
		}
	case "q", "i":
		// The item page's own close keys mean nothing here.
	default:
		return p.Key(msg)
	}
	return nil
}

func (l *Lesson) openOrWarn(i, tab int) tea.Cmd {
	if err := l.open(i, tab); err != nil {
		return func() tea.Msg { return lessonLoadedMsg{err: err} }
	}
	return nil
}

// quiz builds the lesson quiz over the batch: every item in play at once,
// shuffled, as on WaniKani.
func (l *Lesson) quiz() tea.Cmd {
	sItems := make([]session.Item, len(l.items))
	for i, it := range l.items {
		sItems[i] = session.Item{
			AssignmentID: it.Assignment.ID, SubjectID: it.Subject.ID, SubjectType: it.Subject.Object,
			Level: it.Subject.Data.Level,
		}
	}
	q := newReview(l.deps, l.rdeps, lessonSessionName)
	q.sess = session.New(sItems, session.Options{ActiveSize: len(sItems)}, l.rng)
	q.items = l.items
	for id, raw := range l.images {
		q.images[id] = raw
	}
	q.save()
	return func() tea.Msg { return lessonQuizMsg{quiz: q} }
}

// View renders the current item's page.
func (l *Lesson) View() string { return l.page.View() }
