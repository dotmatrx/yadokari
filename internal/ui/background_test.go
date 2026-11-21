package ui

import (
	"errors"
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/jtharris/yadokari/internal/config"
	"github.com/jtharris/yadokari/internal/gfx"
)

func TestDetectBackground(t *testing.T) {
	black, cream := color.RGBA{0, 0, 0, 0xff}, rgb(0xfdf6e3)
	grey, white := rgb(0x586e75), rgb(0xffffff)
	answer := func(fg, bg color.RGBA) func() (gfx.Probed, error) {
		return func() (gfx.Probed, error) { return gfx.Probed{Foreground: fg, Background: bg}, nil }
	}
	silent := func() (gfx.Probed, error) { return gfx.Probed{}, errors.New("no answer") }
	for _, tt := range []struct {
		name, setting, colorfgbg string
		probe                    func() (gfx.Probed, error)
		light                    bool
		text                     color.RGBA
		from                     string
	}{
		{"dark terminal", "auto", "", answer(white, black), false, white, "reported by the terminal"},
		{"light terminal", "", "", answer(grey, cream), true, grey, "reported by the terminal"},
		{"setting wins", "light", "", answer(white, black), true, color.RGBA{}, "set in the config file"},
		{"setting agrees", "dark", "", answer(white, black), false, white, "set in the config file"},
		{"COLORFGBG light", "auto", "0;15", silent, true, color.RGBA{}, "from COLORFGBG"},
		{"COLORFGBG three fields", "auto", "0;default;7", silent, true, color.RGBA{}, "from COLORFGBG"},
		{"COLORFGBG dark", "auto", "15;0", silent, false, color.RGBA{}, "from COLORFGBG"},
		{"COLORFGBG junk", "auto", "default;default", silent, false, color.RGBA{}, "not reported; assumed dark"},
		{"nothing", "auto", "", silent, false, color.RGBA{}, "not reported; assumed dark"},
		{"text only", "auto", "", answer(white, color.RGBA{}), false, white, "not reported; assumed dark"},
	} {
		getenv := func(k string) string {
			if k == "COLORFGBG" {
				return tt.colorfgbg
			}
			return ""
		}
		b := DetectBackground(tt.setting, getenv, tt.probe)
		if b.Light != tt.light || b.Text != tt.text || b.From != tt.from {
			t.Errorf("%s: got light %v text %v %q; want %v %v %q", tt.name, b.Light, b.Text, b.From, tt.light, tt.text, tt.from)
		}
	}
}

func TestFrameInk(t *testing.T) {
	defer func() { SetTheme(DefaultTheme); SetBackground(Background{}) }()
	SetTheme(DefaultTheme)
	for _, tt := range []struct {
		name string
		bg   Background
		ink  string // config [theme] ink
		want color.RGBA
		text color.Color
	}{
		{"dark, nothing known", Background{}, "", rgb(0xffffff), lipgloss.NoColor{}},
		{"light, nothing known", Background{Light: true}, "", darkInk, lipgloss.NoColor{}},
		{"terminal's text colour", Background{Light: true, Text: rgb(0x586e75)}, "", rgb(0x586e75), lipgloss.NoColor{}},
		{"config ink wins", Background{Text: rgb(0x586e75)}, "#ff0000", rgb(0xff0000), rgb(0xff0000)},
	} {
		SetTheme(DefaultTheme.WithColours(config.Colours{Ink: tt.ink}))
		SetBackground(tt.bg)
		if frameFG != tt.want || frameText != tt.text {
			t.Errorf("%s: ink %v, text %v; want %v, %v", tt.name, frameFG, frameText, tt.want, tt.text)
		}
	}
}
