package ui

import (
	"image/color"
	"math"
	"slices"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dotmatrx/yadokari/internal/config"
	"github.com/dotmatrx/yadokari/internal/markup"
	"github.com/dotmatrx/yadokari/internal/wk"
)

// Theme is every colour the UI uses, by role. Colour only marks meaning
// (subject types, right and wrong, counts); nothing paints a background
// except the glyph field.
type Theme struct {
	Name string
	// Subject types: the glyph field behind the characters, and marks
	// beside items and in mnemonics.
	// A palette colour (lipgloss.Color("6")) paints the terminal's own shade.
	Radical, Kanji, Vocabulary color.Color
	// Ink is the glyph drawn on a filled field (fieldFill). A framed
	// field draws white on the terminal's background, unless the config
	// file sets the ink (inkSet).
	Ink    color.RGBA
	inkSet bool
	// Lessons and Reviews mark the dashboard's counts; Bar fills the
	// review forecast.
	Lessons, Reviews, Bar color.Color
	// Correct and Wrong colour the answer box; Warn is a nudge (wrapping
	// up, a near miss); Error a failure (sync, upload, rejected).
	Correct, Wrong, Warn, Error color.Color
	// Line draws panel borders and the input mode badge. ANSI 8, the
	// terminal's own grey, follows the terminal's theme. NoColor: the
	// terminal's text colour (the badge is drawn reversed).
	Line color.Color
	// Muted is secondary text (labels, hints); nil draws it faint.
	Muted color.Color
	// Light marks a theme's light-terminal version (see ForBackground).
	Light bool
	// Strong makes coloured text (right, wrong, warnings, errors) bold.
	Strong bool
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
	// yadokari: electric colour on a black terminal, retro-future. Blues
	// from ice to cobalt, with orange kanji the only warm colour; navy
	// borders and blue-grey secondary text. The fields are bright, so the
	// ink is near-black (contrast 5.2–12).
	themeYadokari = Theme{
		Name:    "yadokari",
		Radical: rgb(0x5cd6ff), Kanji: rgb(0xff7b29), Vocabulary: rgb(0x3b7bff),
		Ink:     rgb(0x05070c),
		Lessons: rgb(0xff7b29), Reviews: rgb(0x5cd6ff), Bar: rgb(0x3b7bff),
		Correct: rgb(0x5cffc8), Wrong: rgb(0xff4a3d), Warn: rgb(0xffb347), Error: rgb(0xff5c4d),
		Line: rgb(0x163a5c), Muted: rgb(0x6d93b8),
	}
	// blackbox: yadokari's restrained sibling. Deep saturated fields behind
	// white ink (contrast 4.0–6.9); the electric colour is only in text.
	themeBlackbox = Theme{
		Name:    "blackbox",
		Radical: rgb(0x0a62e8), Kanji: rgb(0xd9530a), Vocabulary: rgb(0x6a2be0),
		Ink:     rgb(0xffffff),
		Lessons: rgb(0xff8c42), Reviews: rgb(0x4da3ff), Bar: rgb(0x4da3ff),
		Correct: rgb(0x4fd18b), Wrong: rgb(0xff4d5e), Warn: rgb(0xf5b942), Error: rgb(0xff5a67),
		Line: rgb(0x2b3442), Muted: rgb(0x7c8798),
	}
	// terminal: the terminal's own palette (cyan radicals, magenta kanji,
	// blue vocabulary), so the app follows its colour scheme. The ink is
	// white unless the terminal's shades want dark ink (InkFor).
	themeTerminal = Theme{
		Name:    "terminal",
		Radical: lipgloss.Color("6"), Kanji: lipgloss.Color("5"), Vocabulary: lipgloss.Color("4"),
		Ink:     rgb(0xffffff),
		Lessons: lipgloss.Color("5"), Reviews: lipgloss.Color("6"), Bar: lipgloss.Color("6"),
		Correct: lipgloss.Color("2"), Wrong: lipgloss.Color("1"), Warn: lipgloss.Color("3"), Error: lipgloss.Color("1"),
		Line: lipgloss.Color("8"),
	}
	// high-contrast: WaniKani's hues, deep enough for white ink at 7:1 or
	// more; text in the terminal's own palette (made for its background),
	// bold; borders and secondary text in the full text colour, never faint.
	themeHighContrast = Theme{
		Name:    "high-contrast",
		Radical: rgb(0x005a9e), Kanji: rgb(0xa3005c), Vocabulary: rgb(0x5c00a3),
		Ink:     rgb(0xffffff),
		Lessons: lipgloss.Color("5"), Reviews: lipgloss.Color("6"), Bar: lipgloss.Color("6"),
		Correct: lipgloss.Color("2"), Wrong: lipgloss.Color("1"), Warn: lipgloss.Color("3"), Error: lipgloss.Color("1"),
		Line:   lipgloss.NoColor{},
		Muted:  lipgloss.NoColor{},
		Strong: true,
	}
	Themes       = []Theme{themeWaniKani, themeYadokari, themeBlackbox, themeTerminal, themeHighContrast}
	DefaultTheme = themeWaniKani

	// lightThemes are the themes' versions for a light terminal, by name:
	// each colour keeps its hue, darkened to 4.5:1 or more on a light grey
	// (#eeeeee) terminal (a test checks); borders are pale tints. terminal
	// and high-contrast use the terminal's palette and need none.
	lightThemes = map[string]Theme{
		"wanikani": {
			Name: "wanikani", Light: true,
			Radical: rgb(0x0072ab), Kanji: rgb(0xcf008a), Vocabulary: rgb(0xa600f9),
			Ink:     rgb(0xffffff),
			Lessons: rgb(0xcf008a), Reviews: rgb(0x0072ab), Bar: rgb(0x0072ab),
			Correct: rgb(0x507700), Wrong: rgb(0xda002c), Warn: rgb(0x846900), Error: rgb(0xdb0014),
			Line: lipgloss.Color("8"),
		},
		"yadokari": {
			Name: "yadokari", Light: true,
			Radical: rgb(0x00749b), Kanji: rgb(0xba4700), Vocabulary: rgb(0x0d5cff),
			Ink:     rgb(0xffffff),
			Lessons: rgb(0xba4700), Reviews: rgb(0x00749b), Bar: rgb(0x0d5cff),
			Correct: rgb(0x007c52), Wrong: rgb(0xd90f00), Warn: rgb(0x9e5c00), Error: rgb(0xd81200),
			Line: rgb(0x95b1cc), Muted: rgb(0x486f95),
		},
		"blackbox": {
			Name: "blackbox", Light: true,
			Radical: rgb(0x0a62e8), Kanji: rgb(0xbb4709), Vocabulary: rgb(0x6a2be0),
			Ink:     rgb(0xffffff),
			Lessons: rgb(0xb94800), Reviews: rgb(0x0068d8), Bar: rgb(0x0068d8),
			Correct: rgb(0x207b4a), Wrong: rgb(0xda0015), Warn: rgb(0x916308), Error: rgb(0xdb0011),
			Line: rgb(0xa2afc3), Muted: rgb(0x636d7d),
		},
	}
)

// ForBackground is the theme's version for the terminal's background: its
// light version on a light terminal, if it has one. The config file's
// [theme] colours go on top (WithColours after this).
func (t Theme) ForBackground(light bool) Theme {
	if l, ok := lightThemes[t.Name]; ok && light {
		return l
	}
	return t
}

// WithColours applies the config file's [theme] overrides (validated by
// config.Load).
func (t Theme) WithColours(c config.Colours) Theme {
	set := func(dst *color.Color, s string) {
		if s != "" {
			*dst = lipgloss.Color(s)
		}
	}
	set(&t.Radical, c.Radical)
	set(&t.Kanji, c.Kanji)
	set(&t.Vocabulary, c.Vocabulary)
	set(&t.Lessons, c.Lessons)
	set(&t.Reviews, c.Reviews)
	set(&t.Bar, c.Bar)
	set(&t.Correct, c.Correct)
	set(&t.Wrong, c.Wrong)
	set(&t.Warn, c.Warn)
	set(&t.Error, c.Error)
	set(&t.Line, c.Line)
	if c.Ink != "" {
		r, g, b, _ := lipgloss.Color(c.Ink).RGBA()
		t.Ink = color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xff}
		t.inkSet = true
	}
	return t
}

// darkInk is the alternative to white ink on light glyph fields.
var darkInk = rgb(0x1a1a1a)

// UsesPalette reports whether a glyph field takes a terminal palette colour,
// whose shade only the terminal knows (InkFor needs the probe).
func (t Theme) UsesPalette() bool {
	for _, c := range []color.Color{t.Radical, t.Kanji, t.Vocabulary} {
		if _, ok := c.(ansi.BasicColor); ok {
			return true
		}
	}
	return false
}

// InkFor picks white or dark ink, whichever reads better on the glyph
// fields (their worst case), given the shades the terminal reported for
// its palette (alpha 0: unknown, the field is skipped). An ink set in the
// config file is kept.
func (t Theme) InkFor(palette [16]color.RGBA, inkSet bool) Theme {
	if inkSet {
		return t
	}
	worst := func(ink color.RGBA) float64 {
		w, seen := 99.0, false
		for _, c := range []color.Color{t.Radical, t.Kanji, t.Vocabulary} {
			field, ok := c.(color.RGBA)
			if b, isPal := c.(ansi.BasicColor); isPal && int(b) < len(palette) {
				field, ok = palette[b], palette[b].A != 0
			}
			if ok {
				w, seen = min(w, contrast(ink, field)), true
			}
		}
		if !seen {
			return 0
		}
		return w
	}
	if worst(darkInk) > worst(rgb(0xffffff)) {
		t.Ink = darkInk
	}
	return t
}

// contrast is the WCAG contrast ratio of two colours (1–21).
func contrast(a, b color.RGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

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
	typeColour map[string]color.Color
	glyphFG    color.RGBA // ink on a filled field
	frameFG    color.RGBA // ink inside a frame

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

	typeColour = map[string]color.Color{
		wk.ObjectRadical:        t.Radical,
		wk.ObjectKanji:          t.Kanji,
		wk.ObjectVocabulary:     t.Vocabulary,
		wk.ObjectKanaVocabulary: t.Vocabulary,
	}
	glyphFG = t.Ink
	themeInk, themeInkSet = t.Ink, t.inkSet
	setFrameInk()
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	lineSt = fg(t.Line)
	badgeSt = lipgloss.NewStyle().Background(t.Line).Foreground(lipgloss.Color("15")).Bold(true).Padding(0, 1)
	if t.Light {
		// White on a pale border is faint: the terminal's text colour.
		badgeSt = lipgloss.NewStyle().Background(t.Line).Bold(true).Padding(0, 1)
	}
	if _, ok := t.Line.(lipgloss.NoColor); ok {
		badgeSt = lipgloss.NewStyle().Reverse(true).Bold(true).Padding(0, 1)
	}
	mutedSt = lipgloss.NewStyle().Faint(true)
	if t.Muted != nil {
		mutedSt = fg(t.Muted)
	}
	labelSt = mutedSt
	errSt, warnSt = fg(t.Error).Bold(t.Strong), fg(t.Warn).Bold(true)
	correctSt, wrongSt = fg(t.Correct).Bold(t.Strong), fg(t.Wrong).Bold(t.Strong)
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
