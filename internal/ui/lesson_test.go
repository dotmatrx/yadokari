package ui

import (
	"context"
	"math/rand/v2"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jtharris/yadokari/internal/session"
	"github.com/jtharris/yadokari/internal/store"
	"github.com/jtharris/yadokari/internal/wk"
)

// learn steps through every lesson page into the quiz.
func (h *harness) learn() {
	h.t.Helper()
	for n := 0; h.app.screen == lessonScreen; n++ {
		if n > 50 {
			h.t.Fatalf("never reached the quiz:\n%s", h.screen())
		}
		h.key("enter")
	}
	if h.app.screen != reviewScreen || !h.app.review.lessons() {
		h.t.Fatalf("expected the lesson quiz:\n%s", h.screen())
	}
}

func TestLessonsLearnQuizStart(t *testing.T) {
	h := newHarness(t)
	if !strings.Contains(h.screen(), "Lessons      2") {
		t.Fatalf("dashboard:\n%s", h.screen())
	}
	h.key("l")
	if h.app.screen != lessonScreen {
		t.Fatalf("l should open lessons:\n%s", h.screen())
	}
	out := h.screen()
	if !strings.Contains(out, "yadokari · lessons") || !strings.Contains(out, "Esc dashboard") {
		t.Fatalf("lesson page:\n%s", out)
	}
	// ← on the first page stays put; → then ← comes back.
	first := h.app.lesson.page.data.Subject.ID
	h.send(tea.KeyPressMsg{Code: tea.KeyLeft})
	h.send(tea.KeyPressMsg{Code: tea.KeyRight})
	h.send(tea.KeyPressMsg{Code: tea.KeyLeft})
	if h.app.lesson.idx != 0 || h.app.lesson.page.tab != 0 || h.app.lesson.page.data.Subject.ID != first {
		t.Fatalf("left/right should move between tabs: idx %d tab %d", h.app.lesson.idx, h.app.lesson.page.tab)
	}

	h.learn()
	if !strings.Contains(h.screen(), "yadokari · lessons") {
		t.Errorf("quiz header:\n%s", h.screen())
	}
	// A wrong answer just comes back round.
	wrong := "zzzz"
	if q, _, _ := h.app.review.current(); q.Kind == session.Reading {
		wrong = "ぬぬ"
	}
	h.key(wrong)
	h.key("enter")
	if !strings.Contains(h.screen(), "✗") {
		t.Fatalf("expected a wrong answer:\n%s", h.screen())
	}
	h.key("enter")
	for n := 0; h.app.screen == reviewScreen; n++ {
		if n > 20 {
			t.Fatalf("quiz did not end:\n%s", h.screen())
		}
		h.key(h.answerFor())
		h.key("enter")
		h.key("enter")
	}
	out = h.screen()
	for _, want := range []string{"2 learned", "Learned (2)", "Apprentice 1 ↑", "All lessons started."} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
	starts := 0
	for _, w := range h.mock.Writes() {
		if w.Method == "PUT" && strings.HasSuffix(w.Path, "/start") && w.Status == 200 {
			starts++
		}
	}
	if starts != 2 {
		t.Errorf("mock started %d assignments, want 2: %+v", starts, h.mock.Writes())
	}
	h.key("enter")
	if !strings.Contains(h.screen(), "Lessons      0") {
		t.Errorf("no lessons left:\n%s", h.screen())
	}
	if ok, _ := h.st.LoadSession(context.Background(), lessonSessionName, &session.Snapshot{}); ok {
		t.Error("finished quiz should be cleared")
	}
}

func TestLessonQuizResumes(t *testing.T) {
	h := newHarness(t)
	h.key("l")
	h.learn()
	// Finish one item, then leave.
	var done int
	for done == 0 {
		h.key(h.answerFor())
		h.key("enter")
		h.key("enter")
		done = h.app.review.sess.Stats().Done
	}
	h.key("esc")
	if h.app.screen != dashScreen || !strings.Contains(h.screen(), "Lessons      1") {
		t.Fatalf("one lesson started, one left:\n%s", h.screen())
	}
	h.key("l")
	if h.app.screen != reviewScreen || !h.app.review.lessons() || !h.app.review.resume {
		t.Fatalf("l should resume the quiz:\n%s", h.screen())
	}
	if st := h.app.review.sess.Stats(); st.Done != done || st.Total != 2 {
		t.Errorf("resumed stats %+v", st)
	}
}

func TestPickLessons(t *testing.T) {
	var items []store.ReviewItem
	for i, lvl := range []int{1, 1, 1, 2, 2, 3} {
		items = append(items, store.ReviewItem{Subject: wk.Resource[wk.Subject]{ID: i + 1, Data: wk.Subject{Level: lvl}}})
	}
	ids := func(its []store.ReviewItem) (out []int) {
		for _, it := range its {
			out = append(out, it.Subject.ID)
		}
		return out
	}
	rng := rand.New(rand.NewPCG(1, 2))
	if got := ids(pickLessons(items, wk.Preferences{LessonsBatchSize: 4}, rng)); len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Errorf("in order: %v", got)
	}
	if got := pickLessons(items, wk.Preferences{}, rng); len(got) != defaultLessonBatch {
		t.Errorf("default batch: %d", len(got))
	}
	if got := pickLessons(items[:2], wk.Preferences{LessonsBatchSize: 10}, rng); len(got) != 2 {
		t.Errorf("short batch: %d", len(got))
	}
	for range 20 {
		got := ids(pickLessons(items, wk.Preferences{LessonsBatchSize: 4, LessonsPresentationOrder: "ascending_level_then_shuffled"}, rng))
		levels := []int{}
		for _, id := range got {
			levels = append(levels, items[id-1].Subject.Data.Level)
		}
		if levels[0] != 1 || levels[1] != 1 || levels[2] != 1 || levels[3] != 2 {
			t.Fatalf("levels must ascend: %v", levels)
		}
	}
	if items[0].Subject.ID != 1 {
		t.Error("pickLessons must not reorder its input")
	}
}
