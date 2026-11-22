package ui

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/dotmatrx/yadokari/internal/gfx"
)

// Background is what yadokari knows of the terminal's background.
type Background struct {
	Light bool
	// Colour and Text are the background and text colours the terminal
	// reported; alpha 0: not reported.
	Colour, Text color.RGBA
	// From says how Light was decided, in words for --terminal.
	From string
}

// DetectBackground decides whether the terminal's background is light:
// the setting (display.background: "auto" or "" asks), then the terminal's
// own answer (probe), then COLORFGBG, else dark, the common case.
func DetectBackground(setting string, getenv func(string) string, probe func() (gfx.Probed, error)) Background {
	var b Background
	if p, err := probe(); err == nil {
		b.Colour, b.Text = p.Background, p.Foreground
	}
	switch {
	case setting == "dark" || setting == "light":
		b.Light, b.From = setting == "light", "set in the config file"
	case b.Colour.A != 0:
		b.Light, b.From = isLight(b.Colour), "reported by the terminal"
	default:
		if light, ok := colorFGBG(getenv("COLORFGBG")); ok {
			b.Light, b.From = light, "from COLORFGBG"
		} else {
			b.From = "not reported; assumed dark"
		}
	}
	// A text colour that doesn't suit the background (a setting that
	// overrides the terminal's answer) is no use as glyph ink.
	if b.Text.A != 0 && isLight(b.Text) == b.Light {
		b.Text = color.RGBA{}
	}
	return b
}

// isLight: black text reads better on c than white text.
func isLight(c color.RGBA) bool {
	return contrast(c, color.RGBA{0, 0, 0, 0xff}) > contrast(c, rgb(0xffffff))
}

// colorFGBG reads COLORFGBG ("fg;bg", sometimes "fg;x;bg"), set by rxvt,
// Konsole and others: palette 7 and 9–15 are light backgrounds.
func colorFGBG(v string) (light, ok bool) {
	if v == "" {
		return false, false
	}
	n, err := strconv.Atoi(v[strings.LastIndexByte(v, ';')+1:])
	if err != nil || n < 0 || n > 15 {
		return false, false
	}
	return n == 7 || n >= 9, true
}

var (
	background  Background
	themeInk    color.RGBA
	themeInkSet bool
	frameText   color.Color = lipgloss.NoColor{} // text-tier ink inside a frame
)

// SetBackground tells the UI about the terminal's background (glyph ink).
func SetBackground(b Background) {
	background = b
	setFrameInk()
}

// setFrameInk picks the ink of a framed glyph: the config's, else the
// terminal's own text colour, else white on dark and near-black on light.
// The text tier leaves the colour to the terminal unless the config sets it.
func setFrameInk() {
	frameText = lipgloss.NoColor{}
	switch {
	case themeInkSet:
		frameFG, frameText = themeInk, themeInk
	case background.Text.A != 0:
		frameFG = background.Text
	case background.Light:
		frameFG = darkInk
	default:
		frameFG = rgb(0xffffff)
	}
}

// FrameInk reports the ink a framed glyph is drawn in, for --terminal.
func FrameInk() color.RGBA { return frameFG }
