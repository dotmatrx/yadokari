package ui

import (
	"strings"
	"testing"

	"github.com/jtharris/yadokari/internal/answer"
	"github.com/jtharris/yadokari/internal/config"
	"github.com/jtharris/yadokari/internal/session"
	"github.com/jtharris/yadokari/internal/wk"
)

func TestSettingsPrecedence(t *testing.T) {
	no, yes := false, true
	wkPrefs := wk.Preferences{
		LessonsBatchSize: 5, LessonsPresentationOrder: "ascending_level_then_shuffled",
		LessonsAutoplayAudio: true, ReviewsAutoplayAudio: true, ReviewsPresentationOrder: "lower_levels_first",
	}

	// Nothing set: WaniKani's preferences, unchanged.
	var zero Settings
	if got := zero.prefs(wkPrefs); got != wkPrefs {
		t.Errorf("zero settings changed the preferences: %+v", got)
	}
	if o := zero.sessionOptions(wkPrefs); o.Order != session.LowerLevelsFirst || o.ActiveSize != 0 || o.AllowUndo {
		t.Errorf("zero settings: %+v", o)
	}
	if zero.strictness() != answer.WaniKaniLike {
		t.Error("zero strictness")
	}

	// Everything set: the config file wins.
	s := Settings{
		Reviews: config.Reviews{Order: "by_type", ActiveSize: 5, Limit: 20, BackToBack: true, ReadingFirst: true,
			Strictness: "strict", AllowUndo: true, AutoplayAudio: &no},
		Lessons: config.Lessons{BatchSize: 3, Order: "shuffled", AutoplayAudio: &no},
	}
	p := s.prefs(wkPrefs)
	if p.LessonsBatchSize != 3 || p.LessonsPresentationOrder != "shuffled" || p.LessonsAutoplayAudio || p.ReviewsAutoplayAudio {
		t.Errorf("overrides: %+v", p)
	}
	want := session.Options{ActiveSize: 5, Limit: 20, Order: session.ByType, BackToBack: true, ReadingFirst: true, AllowUndo: true}
	if o := s.sessionOptions(wkPrefs); o != want {
		t.Errorf("options: %+v, want %+v", o, want)
	}
	if s.strictness() != answer.Strict {
		t.Error("strict")
	}

	// "wanikani" is the same as unset; an override can turn autoplay on.
	s = Settings{Reviews: config.Reviews{Order: "wanikani", AutoplayAudio: &yes}, Lessons: config.Lessons{Order: "wanikani"}}
	quiet := wkPrefs
	quiet.ReviewsAutoplayAudio = false
	if p := s.prefs(quiet); p.LessonsPresentationOrder != "ascending_level_then_shuffled" || !p.ReviewsAutoplayAudio {
		t.Errorf("wanikani order / autoplay on: %+v", p)
	}
	if o := s.sessionOptions(wkPrefs); o.Order != session.LowerLevelsFirst {
		t.Errorf("wanikani review order: %+v", o)
	}
}

func TestUndoWrongAnswer(t *testing.T) {
	h := newHarness(t)
	h.app.rdeps.Settings.Reviews.AllowUndo = true
	h.key("r")
	for {
		q, _, _ := h.app.review.current()
		if q.Kind == session.Meaning {
			break
		}
		h.key(h.answerFor())
		h.key("enter")
		if strings.Contains(h.screen(), "Backspace retype") {
			t.Fatalf("no undo after a correct answer:\n%s", h.screen())
		}
		h.key("enter")
	}
	q, _, _ := h.app.review.current()
	h.key("zzzz")
	h.key("enter")
	if !strings.Contains(h.screen(), "Backspace retype") {
		t.Fatalf("the hint should show after a wrong answer:\n%s", h.screen())
	}
	h.key("backspace")
	r := h.app.review
	if r.state != answering || string(r.plain) != "zzzz" || r.wrong != 0 {
		t.Fatalf("undo: state %v, input %q, wrong %d\n%s", r.state, string(r.plain), r.wrong, h.screen())
	}
	if again, _, _ := r.current(); again != q || r.sess.Item(q.Index).MeaningMisses != 0 {
		t.Errorf("same question, no miss: %+v %+v", again, r.sess.Item(q.Index))
	}
	// The answer can be edited and resubmitted.
	for range 4 {
		h.key("backspace")
	}
	h.key(h.answerFor())
	h.key("enter")
	if !strings.Contains(h.screen(), "✓") {
		t.Errorf("resubmitted answer:\n%s", h.screen())
	}
}

func TestNoUndoByDefault(t *testing.T) {
	h := newHarness(t)
	h.key("r")
	for {
		q, _, _ := h.app.review.current()
		if q.Kind == session.Meaning {
			break
		}
		h.key(h.answerFor())
		h.key("enter")
		h.key("enter")
	}
	h.key("zzzz")
	h.key("enter")
	h.key("backspace")
	if strings.Contains(h.screen(), "Backspace retype") || h.app.review.state != graded {
		t.Errorf("undo is off by default:\n%s", h.screen())
	}
}

func TestThemeNamesMatchConfig(t *testing.T) {
	var names []string
	for _, th := range Themes {
		names = append(names, th.Name)
	}
	if strings.Join(names, ",") != strings.Join(config.ThemeNames, ",") {
		t.Errorf("ui.Themes %v, config.ThemeNames %v", names, config.ThemeNames)
	}
}
