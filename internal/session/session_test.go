package session

import (
	"encoding/json"
	"math/rand/v2"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func rng(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) }

// mixed makes n items cycling through the four subject types.
func mixed(n int) []Item {
	types := []string{"radical", "kanji", "vocabulary", "kana_vocabulary"}
	items := make([]Item, n)
	for i := range items {
		items[i] = Item{AssignmentID: 1000 + i, SubjectID: 1 + i, SubjectType: types[i%4], Level: 1 + i%5, SRSStage: 1 + i%8}
	}
	return items
}

func questionsFor(items []Item) int {
	n := 0
	for _, it := range items {
		n++
		if it.HasReading() {
			n++
		}
	}
	return n
}

func TestAllCorrect(t *testing.T) {
	items := mixed(25)
	s := New(items, Options{}, rng(1))
	asked := 0
	var done []Completion
	for !s.Done() {
		c, err := s.Answer(true, t0)
		if err != nil {
			t.Fatal(err)
		}
		asked++
		if c != nil {
			done = append(done, *c)
		}
	}
	if asked != questionsFor(items) {
		t.Errorf("asked %d questions, want %d", asked, questionsFor(items))
	}
	if len(done) != len(items) {
		t.Fatalf("%d completions, want %d", len(done), len(items))
	}
	for _, c := range done {
		if c.IncorrectMeaningAnswers != 0 || c.IncorrectReadingAnswers != 0 || !c.CompletedAt.Equal(t0) {
			t.Errorf("completion %+v", c)
		}
	}
	if st := s.Stats(); st.Done != 25 || st.Perfect != 25 || st.Active != 0 || st.Remaining != 0 {
		t.Errorf("stats %+v", st)
	}
}

// TestRandomAnswers is the main property test: over many seeds and random
// right/wrong answers, every item completes exactly once, reported misses
// equal the wrong answers given, finished parts are never asked again, and
// the active set never exceeds its size.
func TestRandomAnswers(t *testing.T) {
	for seed := uint64(0); seed < 300; seed++ {
		r := rng(seed + 1000)
		items := mixed(1 + r.IntN(40))
		opts := Options{ActiveSize: 1 + r.IntN(12), BackToBack: r.IntN(2) == 0, ReadingFirst: r.IntN(2) == 0}
		s := New(items, opts, rng(seed))

		wrong := map[[2]int]int{} // [assignment, kind] → wrong answers given
		answered := map[[2]int]bool{}
		completed := map[int]Completion{}
		for steps := 0; !s.Done(); steps++ {
			if steps > 10000 {
				t.Fatalf("seed %d: session never ends", seed)
			}
			q, _ := s.Current()
			key := [2]int{q.AssignmentID, int(q.Kind)}
			if answered[key] {
				t.Fatalf("seed %d: asked %v of %d again after a correct answer", seed, q.Kind, q.AssignmentID)
			}
			if _, ok := completed[q.AssignmentID]; ok {
				t.Fatalf("seed %d: asked about completed item %d", seed, q.AssignmentID)
			}
			if a := s.Stats().Active; a > opts.ActiveSize {
				t.Fatalf("seed %d: %d active > %d", seed, a, opts.ActiveSize)
			}
			correct := r.IntN(3) > 0
			c, err := s.Answer(correct, t0)
			if err != nil {
				t.Fatal(err)
			}
			if correct {
				answered[key] = true
			} else {
				wrong[key]++
			}
			if c != nil {
				if _, dup := completed[c.AssignmentID]; dup {
					t.Fatalf("seed %d: item %d completed twice", seed, c.AssignmentID)
				}
				completed[c.AssignmentID] = *c
			}
		}
		if len(completed) != len(items) {
			t.Fatalf("seed %d: %d of %d items completed", seed, len(completed), len(items))
		}
		for id, c := range completed {
			if c.IncorrectMeaningAnswers != wrong[[2]int{id, int(Meaning)}] ||
				c.IncorrectReadingAnswers != wrong[[2]int{id, int(Reading)}] {
				t.Fatalf("seed %d: item %d reported %d/%d misses, gave %d/%d", seed, id,
					c.IncorrectMeaningAnswers, c.IncorrectReadingAnswers,
					wrong[[2]int{id, int(Meaning)}], wrong[[2]int{id, int(Reading)}])
			}
		}
	}
}

func TestRadicalHasNoReading(t *testing.T) {
	s := New([]Item{{AssignmentID: 1, SubjectType: "radical"}, {AssignmentID: 2, SubjectType: "kana_vocabulary"}}, Options{}, rng(2))
	for !s.Done() {
		q, _ := s.Current()
		if q.Kind != Meaning {
			t.Fatalf("asked %v for a meaning-only item", q.Kind)
		}
		c, _ := s.Answer(true, t0)
		if c == nil {
			t.Fatal("a meaning-only item completes after one correct answer")
		}
	}
}

func TestQuestionsSpacedApart(t *testing.T) {
	// Without back-to-back, an item's next question never follows its last
	// one while other items are in play.
	for seed := uint64(0); seed < 50; seed++ {
		s := New(mixed(20), Options{}, rng(seed))
		answers := rng(seed + 500)
		var prev *Question
		for !s.Done() {
			q, _ := s.Current()
			if prev != nil && prev.Index == q.Index && s.Stats().Active > 1 {
				t.Fatalf("seed %d: item %d asked twice in a row with others active", seed, q.AssignmentID)
			}
			s.Answer(answers.IntN(4) > 0, t0)
			prev = &q
		}
	}
}

func TestBackToBack(t *testing.T) {
	for _, readingFirst := range []bool{false, true} {
		s := New(mixed(12), Options{BackToBack: true, ReadingFirst: readingFirst}, rng(3))
		for !s.Done() {
			q, _ := s.Current()
			if !s.Item(q.Index).HasReading() {
				s.Answer(true, t0)
				continue
			}
			first := Meaning
			if readingFirst {
				first = Reading
			}
			if q.Kind != first {
				t.Fatalf("readingFirst=%v: first question was %v", readingFirst, q.Kind)
			}
			s.Answer(true, t0)
			q2, _ := s.Current()
			if q2.Index != q.Index || q2.Kind == q.Kind {
				t.Fatalf("readingFirst=%v: pair split: %+v then %+v", readingFirst, q, q2)
			}
			s.Answer(true, t0)
		}
	}
}

func TestOrders(t *testing.T) {
	items := mixed(40)
	check := func(order Order, key func(Item) int) {
		s := New(items, Options{Order: order, ActiveSize: 1}, rng(4))
		prev := -1
		for !s.Done() {
			q, _ := s.Current()
			k := key(s.Item(q.Index))
			if k < prev {
				t.Fatalf("%s: key went %d → %d", order, prev, k)
			}
			prev = k
			s.Answer(true, t0)
		}
	}
	check(LowerLevelsFirst, func(it Item) int { return it.Level })
	check(LowerStagesFirst, func(it Item) int { return it.SRSStage })
	check(ByType, func(it Item) int {
		return map[string]int{"radical": 0, "kanji": 1, "vocabulary": 2, "kana_vocabulary": 2}[it.SubjectType]
	})
}

func TestLimitAndWrapUp(t *testing.T) {
	s := New(mixed(30), Options{Limit: 12, ActiveSize: 5}, rng(5))
	if st := s.Stats(); st.Total != 12 || st.Active != 5 || st.Remaining != 7 {
		t.Fatalf("limit stats %+v", st)
	}
	n := 0
	for !s.Done() {
		if c, _ := s.Answer(true, t0); c != nil {
			n++
		}
	}
	if n != 12 {
		t.Fatalf("limit: %d items done, want 12", n)
	}

	s = New(mixed(30), Options{ActiveSize: 5}, rng(6))
	s.Answer(true, t0)
	s.WrapUp(true)
	if st := s.Stats(); st.Remaining != 0 || st.Active != 5 {
		t.Fatalf("wrap-up stats %+v", st)
	}
	n = 0
	for !s.Done() {
		if c, _ := s.Answer(true, t0); c != nil {
			n++
		}
	}
	if n != 5 {
		t.Fatalf("wrap-up finished %d items, want the 5 active", n)
	}
	s.WrapUp(false)
	if s.Done() || s.Stats().Active != 5 {
		t.Fatal("turning wrap-up off should draw more items")
	}
}

func TestUndo(t *testing.T) {
	items := []Item{{AssignmentID: 1, SubjectType: "kanji"}}
	s := New(items, Options{AllowUndo: true}, rng(7))
	q, _ := s.Current()
	s.Answer(false, t0)
	if !s.CanUndo() {
		t.Error("CanUndo after a wrong answer")
	}
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if s.CanUndo() {
		t.Error("CanUndo after undoing")
	}
	if q2, _ := s.Current(); q2 != q {
		t.Errorf("after undo, current %+v want %+v", q2, q)
	}
	it := s.Item(0)
	if it.MeaningMisses+it.ReadingMisses != 0 {
		t.Errorf("miss not reverted: %+v", it)
	}
	if err := s.Undo(); err != ErrNothingToUndo {
		t.Error("double undo should fail")
	}
	s.Answer(true, t0)
	if err := s.Undo(); err != ErrNothingToUndo {
		t.Error("a correct answer must not be undoable")
	}

	s = New(items, Options{}, rng(7))
	s.Answer(false, t0)
	if err := s.Undo(); err != ErrNothingToUndo {
		t.Error("undo must be off unless AllowUndo")
	}
}

func TestSnapshotRestore(t *testing.T) {
	items := mixed(30)
	s := New(items, Options{ActiveSize: 6}, rng(8))
	r := rng(9)
	for range 25 {
		s.Answer(r.IntN(3) > 0, t0)
	}
	q, _ := s.Current()
	raw, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	s2, err := Restore(snap, rng(10))
	if err != nil {
		t.Fatal(err)
	}
	if q2, _ := s2.Current(); q2 != q {
		t.Errorf("restored current %+v, want %+v", q2, q)
	}
	if s2.Stats() != s.Stats() {
		t.Errorf("stats %+v vs %+v", s2.Stats(), s.Stats())
	}
	// Both finish with the same set of completions.
	finish := func(s *Session) int {
		n := len(s.Completed())
		for !s.Done() {
			if c, _ := s.Answer(true, t0); c != nil {
				n++
			}
		}
		return n
	}
	if a, b := finish(s), finish(s2); a != 30 || b != 30 {
		t.Errorf("completions %d and %d, want 30", a, b)
	}

	snap.Active = append(snap.Active, snap.Active[0])
	if _, err := Restore(snap, rng(1)); err == nil {
		t.Error("duplicate index should be rejected")
	}
}
