package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"strings"
	"testing"

	"github.com/dotmatrx/yadokari/internal/answer"
	"github.com/dotmatrx/yadokari/internal/config"
	"github.com/dotmatrx/yadokari/internal/session"
	"github.com/dotmatrx/yadokari/internal/wk"
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

func TestSettingsVoice(t *testing.T) {
	rec := func(id int, name string) wk.PronunciationAudio {
		return wk.PronunciationAudio{Metadata: wk.AudioMetadata{VoiceActorID: id, VoiceActorName: name}}
	}
	audios := []wk.PronunciationAudio{rec(1, "Kyoko"), rec(2, "Kenichi")}
	for _, tt := range []struct {
		setting string
		audios  []wk.PronunciationAudio
		want    int
	}{
		{"", audios, 1},          // the WaniKani default
		{"Kenichi", audios, 2},   // named
		{" kenichi ", audios, 2}, // any case, spaces trimmed
		{"Miyuki", audios, 1},    // no recording by Miyuki: the default
		{"Kenichi", nil, 1},      // no recordings at all
		{"Kyoko", []wk.PronunciationAudio{rec(2, "Kenichi"), rec(1, "Kyoko")}, 1},
	} {
		s := Settings{Audio: config.Audio{Voice: tt.setting}}
		if got := s.voice(tt.audios, 1); got != tt.want {
			t.Errorf("voice %q: got %d, want %d", tt.setting, got, tt.want)
		}
	}
}

func TestFieldNamesMatchConfig(t *testing.T) {
	if strings.Join(FieldStyles, ",") != strings.Join(config.FieldNames, ",") {
		t.Errorf("ui.FieldStyles %v, config.FieldNames %v", FieldStyles, config.FieldNames)
	}
}

func TestThemeWithColours(t *testing.T) {
	th := themeYadokari.WithColours(config.Colours{Kanji: "#112233", Line: "7", Ink: "#000000"})
	if r, g, b, _ := th.Kanji.RGBA(); r>>8 != 0x11 || g>>8 != 0x22 || b>>8 != 0x33 {
		t.Errorf("kanji %v", th.Kanji)
	}
	if th.Line != lipgloss.Color("7") || th.Ink != (color.RGBA{0, 0, 0, 0xff}) {
		t.Errorf("line %v ink %v", th.Line, th.Ink)
	}
	if th.Radical != themeYadokari.Radical || th.Vocabulary != themeYadokari.Vocabulary {
		t.Error("unset colours keep the theme's")
	}
}

func TestTerminalThemeInk(t *testing.T) {
	th, _ := ThemeNamed("terminal")
	if !th.UsesPalette() || themeYadokari.UsesPalette() {
		t.Error("UsesPalette")
	}
	var pal [16]color.RGBA
	if got := th.InkFor(pal, false).Ink; got != rgb(0xffffff) {
		t.Errorf("nothing reported: white ink, got %v", got)
	}
	// A dark scheme (Tomorrow Night's blue, magenta, cyan... muted, mid-light):
	// pastel fields want dark ink.
	pal[4], pal[5], pal[6] = rgb(0x81a2be), rgb(0xb294bb), rgb(0x8abeb7)
	if got := th.InkFor(pal, false).Ink; got != darkInk {
		t.Errorf("pastel fields: dark ink, got %v", got)
	}
	// Deep, saturated shades keep white.
	pal[4], pal[5], pal[6] = rgb(0x1e3a8a), rgb(0x86198f), rgb(0x0e7490)
	if got := th.InkFor(pal, false).Ink; got != rgb(0xffffff) {
		t.Errorf("deep fields: white ink, got %v", got)
	}
	if got := th.WithColours(config.Colours{Ink: "#ff0000"}).InkFor(pal, true).Ink; got != rgb(0xff0000) {
		t.Errorf("a configured ink is kept, got %v", got)
	}
}
