package ui

import (
	"image/color"
	"slices"

	"charm.land/lipgloss/v2"

	"github.com/jtharris/yadokari/internal/markup"
	"github.com/jtharris/yadokari/internal/wk"
)

// Theme is every colour the UI uses, by role. Colour only marks meaning
// (subject types, right and wrong, counts); nothing paints a background
// except the glyph field.
type Theme struct {
	Name string
	// Subject types: the glyph field behind the characters, and marks
	// beside items and in mnemonics.
	Radical, Kanji, Vocabulary color.RGBA
	// Ink is the glyph drawn on its field.
	Ink color.RGBA
	// Lessons and Reviews mark the dashboard's counts; Bar fills the
	// review forecast.
	Lessons, Reviews, Bar color.Color
	// Correct and Wrong colour the answer box; Warn is a nudge (wrapping
	// up, a near miss); Error a failure (sync, upload, rejected).
	Correct, Wrong, Warn, Error color.Color
	// Line draws panel borders and the input mode badge. ANSI 8, the
	// terminal's own grey, follows the terminal's theme.
	Line color.Color
}

func rgb(hex uint32) color.RGBA {
	return color.RGBA{uint8(hex >> 16), uint8(hex >> 8), uint8(hex), 0xff}
}

// Themes, by name; DefaultTheme is used when none is set.
var (
	themeWaniKani = Theme{
		Name:    "wanikani",
		Radical: rgb(0x00aaff), Kanji: rgb(0xff00aa), Vocabulary: rgb(0xaa00ff),
		Ink:     rgb(0xffffff),
		Lessons: rgb(0xff00aa), Reviews: rgb(0x00aaff), Bar: rgb(0x00aaff),
		Correct: rgb(0x88cc00), Wrong: rgb(0xff0033), Warn: rgb(0xffcc00), Error: rgb(0xff4455),
		Line: lipgloss.Color("8"),
	}
	// yadokari (宿借り, hermit crab): a tide pool, warm and sea-coloured
	// where WaniKani is blue, pink and purple. White ink reads on every
	// type colour (contrast 3.9–6.9).
	themeYadokari = Theme{
		Name:    "yadokari",
		Radical: rgb(0x1f8a7d), Kanji: rgb(0xd9572b), Vocabulary: rgb(0x4c4fae),
		Ink:     rgb(0xffffff),
		Lessons: rgb(0xd9572b), Reviews: rgb(0x1f8a7d), Bar: rgb(0xd8b27a),
		Correct: rgb(0x6aa84f), Wrong: rgb(0xc8283c), Warn: rgb(0xe0b04a), Error: rgb(0xe0364a),
		Line: lipgloss.Color("8"),
	}
	Themes       = []Theme{themeWaniKani, themeYadokari}
	DefaultTheme = themeWaniKani
)

// ThemeNamed returns the theme of that name.
func ThemeNamed(name string) (Theme, bool) {
	i := slices.IndexFunc(Themes, func(t Theme) bool { return t.Name == name })
	if i < 0 {
		return Theme{}, false
	}
	return Themes[i], true
}

// The styles screens draw with, set from the theme by SetTheme. One theme
// per process (cmd/preview switches between renders).
var (
	typeColour map[string]color.RGBA
	glyphFG    color.RGBA

	boldSt    = lipgloss.NewStyle().Bold(true)
	mutedSt   = lipgloss.NewStyle().Faint(true)
	titleSt   = lipgloss.NewStyle().Bold(true)
	labelSt   = mutedSt
	pendingSt = lipgloss.NewStyle().Underline(true)

	lineSt, badgeSt                   lipgloss.Style
	errSt, warnSt, correctSt, wrongSt lipgloss.Style
	lessonSt, reviewSt, barSt         lipgloss.Style
	markupSt                          map[markup.Kind]lipgloss.Style
)

func init() { SetTheme(DefaultTheme) }

// SetTheme makes t the UI's colours.
func SetTheme(t Theme) {

	typeColour = map[string]color.RGBA{
		wk.ObjectRadical:        t.Radical,
		wk.ObjectKanji:          t.Kanji,
		wk.ObjectVocabulary:     t.Vocabulary,
		wk.ObjectKanaVocabulary: t.Vocabulary,
	}
	glyphFG = t.Ink
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	lineSt = fg(t.Line)
	badgeSt = lipgloss.NewStyle().Background(t.Line).Foreground(lipgloss.Color("15")).Bold(true).Padding(0, 1)
	errSt, warnSt = fg(t.Error), fg(t.Warn).Bold(true)
	correctSt, wrongSt = fg(t.Correct), fg(t.Wrong)
	lessonSt, reviewSt, barSt = fg(t.Lessons), fg(t.Reviews), fg(t.Bar)
	markupSt = map[markup.Kind]lipgloss.Style{
		markup.Radical:    fg(t.Radical),
		markup.Kanji:      fg(t.Kanji),
		markup.Vocabulary: fg(t.Vocabulary),
		markup.Reading:    lipgloss.NewStyle().Bold(true),
		markup.Meaning:    lipgloss.NewStyle().Bold(true),
		markup.Link:       lipgloss.NewStyle().Underline(true),
	}
}
