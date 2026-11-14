// Package session is the review engine: it decides which question to ask
// next, tracks misses per item, and reports an item as complete once every
// part has been answered correctly. A completion carries exactly what
// POST /reviews needs.
//
// The engine mirrors WaniKani's web app: a shuffled backlog feeds a small
// active set, and each question is drawn at random from the active set, which
// spaces an item's meaning and reading apart. It does no I/O; a Snapshot can
// be persisted after every answer and restored after a crash.
package session

import (
	"cmp"
	"errors"
	"math/rand/v2"
	"slices"
	"time"
)

// Kind is the part of an item a question asks about.
type Kind int

const (
	Meaning Kind = iota
	Reading
)

func (k Kind) String() string {
	if k == Reading {
		return "reading"
	}
	return "meaning"
}

// Item is one assignment under review.
type Item struct {
	AssignmentID int    `json:"assignment_id"`
	SubjectID    int    `json:"subject_id"`
	SubjectType  string `json:"subject_type"` // radical, kanji, vocabulary, kana_vocabulary
	Level        int    `json:"level"`
	SRSStage     int    `json:"srs_stage"`

	MeaningDone   bool `json:"meaning_done"`
	ReadingDone   bool `json:"reading_done"`
	MeaningMisses int  `json:"meaning_misses"`
	ReadingMisses int  `json:"reading_misses"`
}

// HasReading reports whether the item has a reading question.
func (it Item) HasReading() bool {
	return it.SubjectType == "kanji" || it.SubjectType == "vocabulary"
}

func (it Item) complete() bool { return it.MeaningDone && (!it.HasReading() || it.ReadingDone) }

func (it Item) pending() []Kind {
	var ks []Kind
	if !it.MeaningDone {
		ks = append(ks, Meaning)
	}
	if it.HasReading() && !it.ReadingDone {
		ks = append(ks, Reading)
	}
	return ks
}

// Question is what to ask now. Index identifies the item within the session.
type Question struct {
	Index        int  `json:"index"`
	AssignmentID int  `json:"assignment_id"`
	SubjectID    int  `json:"subject_id"`
	Kind         Kind `json:"kind"`
}

// Completion is a finished item: post it as a review.
type Completion struct {
	AssignmentID            int       `json:"assignment_id"`
	SubjectID               int       `json:"subject_id"`
	IncorrectMeaningAnswers int       `json:"incorrect_meaning_answers"`
	IncorrectReadingAnswers int       `json:"incorrect_reading_answers"`
	CompletedAt             time.Time `json:"completed_at"`
}

// Order is how the backlog is arranged. Ties are broken randomly.
type Order string

const (
	Shuffled         Order = "shuffled"
	LowerLevelsFirst Order = "lower_levels_first" // WaniKani's own option
	LowerStagesFirst Order = "lower_stages_first"
	ByType           Order = "by_type" // radicals, then kanji, then vocabulary
)

// Options configure a session. The zero value behaves like WaniKani.
type Options struct {
	ActiveSize   int   `json:"active_size"`   // items in play at once; default 10
	Limit        int   `json:"limit"`         // max items this session; 0 = all
	Order        Order `json:"order"`         // default Shuffled
	BackToBack   bool  `json:"back_to_back"`  // ask an item's two questions consecutively
	ReadingFirst bool  `json:"reading_first"` // with BackToBack: reading before meaning
	AllowUndo    bool  `json:"allow_undo"`    // permit Undo of a wrong answer
}

// ErrNothingToUndo is returned by Undo when the last answer cannot be undone.
var ErrNothingToUndo = errors.New("session: nothing to undo")

// Session is a review session in progress.
type Session struct {
	opts    Options
	rng     *rand.Rand
	items   []Item
	backlog []int // item indexes not yet in play, in draw order
	active  []int // item indexes in play
	done    []int // completed item indexes, in completion order
	taken   int   // items moved from backlog to active
	wrapUp  bool

	current *Question
	last    *Question // most recently answered question (for spacing and pairing)
	undo    *Question // last answer if it was wrong and is still undoable
}

// New starts a session over items. rng drives ordering and question choice.
func New(items []Item, opts Options, rng *rand.Rand) *Session {
	if opts.ActiveSize <= 0 {
		opts.ActiveSize = 10
	}
	if opts.Order == "" {
		opts.Order = Shuffled
	}
	s := &Session{opts: opts, rng: rng, items: slices.Clone(items)}
	for i := range s.items {
		s.backlog = append(s.backlog, i)
	}
	rng.Shuffle(len(s.backlog), func(i, j int) { s.backlog[i], s.backlog[j] = s.backlog[j], s.backlog[i] })
	key := func(i int) int {
		it := s.items[i]
		switch opts.Order {
		case LowerLevelsFirst:
			return it.Level
		case LowerStagesFirst:
			return it.SRSStage
		case ByType:
			switch it.SubjectType {
			case "radical":
				return 0
			case "kanji":
				return 1
			}
			return 2
		}
		return 0
	}
	slices.SortStableFunc(s.backlog, func(a, b int) int { return cmp.Compare(key(a), key(b)) })
	s.refill()
	return s
}

func (s *Session) refill() {
	for !s.wrapUp && len(s.active) < s.opts.ActiveSize && len(s.backlog) > 0 &&
		(s.opts.Limit <= 0 || s.taken < s.opts.Limit) {
		s.active = append(s.active, s.backlog[0])
		s.backlog = s.backlog[1:]
		s.taken++
	}
}

// Done reports whether no questions remain.
func (s *Session) Done() bool {
	_, ok := s.Current()
	return !ok
}

// Current returns the question to ask now. It is stable until answered.
func (s *Session) Current() (Question, bool) {
	if s.current != nil {
		return *s.current, true
	}
	if len(s.active) == 0 {
		return Question{}, false
	}
	q := s.pick()
	s.current = &q
	return q, true
}

func (s *Session) question(idx int, k Kind) Question {
	it := s.items[idx]
	return Question{Index: idx, AssignmentID: it.AssignmentID, SubjectID: it.SubjectID, Kind: k}
}

func (s *Session) pick() Question {
	// Back-to-back: finish the item just asked before moving on.
	if s.opts.BackToBack && s.last != nil && slices.Contains(s.active, s.last.Index) {
		if p := s.items[s.last.Index].pending(); len(p) > 0 {
			return s.question(s.last.Index, s.ordered(p)[0])
		}
	}

	// Otherwise a random active item, avoiding the one just asked when
	// another is available so its two questions are spaced apart.
	candidates := s.active
	if s.last != nil && len(s.active) > 1 {
		candidates = slices.DeleteFunc(slices.Clone(s.active), func(i int) bool { return i == s.last.Index })
	}
	idx := candidates[s.rng.IntN(len(candidates))]
	p := s.items[idx].pending()
	if s.opts.BackToBack {
		return s.question(idx, s.ordered(p)[0])
	}
	return s.question(idx, p[s.rng.IntN(len(p))])
}

func (s *Session) ordered(p []Kind) []Kind {
	if s.opts.ReadingFirst && len(p) == 2 {
		return []Kind{Reading, Meaning}
	}
	return p
}

// Answer records the result of the current question. When the answer
// finishes an item, the completion is returned and the item leaves play.
func (s *Session) Answer(correct bool, at time.Time) (*Completion, error) {
	q, ok := s.Current()
	if !ok {
		return nil, errors.New("session: no question to answer")
	}
	it := &s.items[q.Index]
	s.current, s.last, s.undo = nil, &q, nil

	if !correct {
		if q.Kind == Meaning {
			it.MeaningMisses++
		} else {
			it.ReadingMisses++
		}
		if s.opts.AllowUndo {
			s.undo = &q
		}
		return nil, nil
	}

	if q.Kind == Meaning {
		it.MeaningDone = true
	} else {
		it.ReadingDone = true
	}
	if !it.complete() {
		return nil, nil
	}
	s.active = slices.DeleteFunc(s.active, func(i int) bool { return i == q.Index })
	s.done = append(s.done, q.Index)
	s.refill()
	return &Completion{
		AssignmentID:            it.AssignmentID,
		SubjectID:               it.SubjectID,
		IncorrectMeaningAnswers: it.MeaningMisses,
		IncorrectReadingAnswers: it.ReadingMisses,
		CompletedAt:             at,
	}, nil
}

// Undo reverts the last answer if it was wrong (the "I made a typo" key),
// making that question current again. Correct answers can't be undone: a
// completed item may already be queued for upload.
func (s *Session) Undo() error {
	if s.undo == nil {
		return ErrNothingToUndo
	}
	q := *s.undo
	it := &s.items[q.Index]
	if q.Kind == Meaning {
		it.MeaningMisses--
	} else {
		it.ReadingMisses--
	}
	s.current, s.undo = &q, nil
	return nil
}

// CanUndo reports whether Undo would succeed.
func (s *Session) CanUndo() bool { return s.undo != nil }

// WrapUp stops (or resumes) drawing new items into play.
func (s *Session) WrapUp(on bool) {
	s.wrapUp = on
	s.refill()
}

// WrappingUp reports whether wrap-up is on.
func (s *Session) WrappingUp() bool { return s.wrapUp }

// Item returns the state of an item by index.
func (s *Session) Item(idx int) Item { return s.items[idx] }

// Stats summarizes progress.
type Stats struct {
	Total     int // items in the session (after Limit)
	Done      int
	Active    int
	Remaining int // not yet in play
	Perfect   int // completed with no misses
}

// Stats returns progress counts.
func (s *Session) Stats() Stats {
	st := Stats{Done: len(s.done), Active: len(s.active), Remaining: len(s.backlog)}
	if s.opts.Limit > 0 {
		st.Remaining = min(st.Remaining, max(s.opts.Limit-s.taken, 0))
	}
	if s.wrapUp {
		st.Remaining = 0
	}
	st.Total = st.Done + st.Active + st.Remaining
	for _, i := range s.done {
		if s.items[i].MeaningMisses == 0 && s.items[i].ReadingMisses == 0 {
			st.Perfect++
		}
	}
	return st
}

// Completed returns finished items in completion order.
func (s *Session) Completed() []Item {
	out := make([]Item, len(s.done))
	for i, idx := range s.done {
		out[i] = s.items[idx]
	}
	return out
}

// Snapshot is the persistable state of a session.
type Snapshot struct {
	Options Options   `json:"options"`
	Items   []Item    `json:"items"`
	Backlog []int     `json:"backlog"`
	Active  []int     `json:"active"`
	Done    []int     `json:"done"`
	Taken   int       `json:"taken"`
	WrapUp  bool      `json:"wrap_up"`
	Current *Question `json:"current,omitempty"`
	Last    *Question `json:"last,omitempty"`
}

// Snapshot captures the session for persistence. Undo state is not kept:
// after a restart the last wrong answer stands.
func (s *Session) Snapshot() Snapshot {
	return Snapshot{
		Options: s.opts, Items: slices.Clone(s.items), Backlog: slices.Clone(s.backlog),
		Active: slices.Clone(s.active), Done: slices.Clone(s.done), Taken: s.taken, WrapUp: s.wrapUp,
		Current: clonePtr(s.current), Last: clonePtr(s.last),
	}
}

// Restore resumes a session from a snapshot.
func Restore(snap Snapshot, rng *rand.Rand) (*Session, error) {
	n := len(snap.Items)
	seen := make([]bool, n)
	for _, list := range [][]int{snap.Backlog, snap.Active, snap.Done} {
		for _, i := range list {
			if i < 0 || i >= n || seen[i] {
				return nil, errors.New("session: corrupt snapshot")
			}
			seen[i] = true
		}
	}
	if slices.Contains(seen, false) {
		return nil, errors.New("session: corrupt snapshot")
	}
	s := &Session{
		opts: snap.Options, rng: rng, items: slices.Clone(snap.Items),
		backlog: slices.Clone(snap.Backlog), active: slices.Clone(snap.Active), done: slices.Clone(snap.Done),
		taken: snap.Taken, wrapUp: snap.WrapUp, current: clonePtr(snap.Current), last: clonePtr(snap.Last),
	}
	if s.current != nil && (!slices.Contains(s.active, s.current.Index) ||
		!slices.Contains(s.items[s.current.Index].pending(), s.current.Kind)) {
		s.current = nil
	}
	return s, nil
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
