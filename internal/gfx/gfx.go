// Package gfx draws a large glyph into a block of terminal cells using the
// best tier the terminal supports:
//
//	Kitty      kitty graphics, Unicode placeholders (kitty, Ghostty)
//	HalfBlock  ▀ cells with truecolor, two pixels per cell (most terminals)
//	Plain      the characters, centred in a coloured box (everything)
//
// Every tier returns ordinary cell content for a Bubble Tea View. The kitty
// tier also returns a one-time escape sequence (send it with tea.Raw) that
// uploads the image; the cells then refer to it.
package gfx

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// Tier is a rendering strategy.
type Tier string

const (
	Kitty     Tier = "kitty"
	HalfBlock Tier = "halfblock"
	Plain     Tier = "plain"
)

// ParseTier parses a config value; "" or "auto" returns ok=false.
func ParseTier(s string) (Tier, bool, error) {
	switch Tier(strings.ToLower(s)) {
	case "", "auto":
		return "", false, nil
	case Kitty, HalfBlock, Plain:
		return Tier(strings.ToLower(s)), true, nil
	}
	return "", false, fmt.Errorf("gfx: unknown graphics tier %q (auto, kitty, halfblock, plain)", s)
}

// Detect picks a tier from the environment. It is a heuristic: kitty and
// Ghostty identify themselves; truecolor support is announced via COLORTERM.
// NO_COLOR forces Plain. Inside tmux, kitty graphics need passthrough, so
// tmux gets HalfBlock.
func Detect(getenv func(string) string) Tier {
	switch {
	case getenv("NO_COLOR") != "":
		return Plain
	case getenv("TMUX") != "":
		// fall through to the colour checks
	case getenv("KITTY_WINDOW_ID") != "" || getenv("TERM") == "xterm-kitty" ||
		getenv("TERM_PROGRAM") == "ghostty" || getenv("TERM") == "xterm-ghostty":
		return Kitty
	}
	if ct := getenv("COLORTERM"); ct == "truecolor" || ct == "24bit" {
		return HalfBlock
	}
	return Plain
}

// Source is something to draw: a mask producer plus a text fallback.
type Source struct {
	// Key identifies the content (e.g. "kanji:440"); it keys image caches.
	Key string
	// Text is drawn by the Plain tier ("" for image-only radicals).
	Text string
	// Mask returns a coverage mask of exactly w×h. ss is a hint that
	// supersampling would help (tiny masks); sources may ignore it.
	Mask func(w, h, ss int) (*image.Alpha, error)
}

// Frame is a drawn glyph.
type Frame struct {
	// Setup must be written to the terminal (tea.Raw) before Cells are shown;
	// empty when nothing needs sending.
	Setup string
	// Cells is rows lines of exactly cols cells each, joined by "\n".
	Cells string
}

// Renderer draws sources into cols×rows cells on a solid background.
type Renderer interface {
	Tier() Tier
	Draw(src Source, cols, rows int, fg, bg color.Color) (Frame, error)
	// Reset forgets anything sent to the terminal (after a screen change)
	// and returns the sequence that frees it there.
	Reset() string
}

// New returns the renderer for a tier. cellW and cellH are the cell size in
// pixels (used by Kitty; see CellSize).
func New(t Tier, cellW, cellH int) Renderer {
	switch t {
	case Kitty:
		return newKitty(cellW, cellH)
	case HalfBlock:
		return halfBlock{}
	}
	return plain{}
}

// CellSize reports the terminal cell size in pixels, or a 10×20 guess when
// the terminal doesn't say.
func CellSize() (w, h int) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return 10, 20
	}
	return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row)
}

func rgb(c color.Color) (r, g, b uint8) {
	cr, cg, cb, _ := c.RGBA()
	return uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8)
}

func blend(fg, bg color.Color, a uint8) (r, g, b uint8) {
	fr, fgg, fb := rgb(fg)
	br, bgg, bb := rgb(bg)
	mix := func(f, b uint8) uint8 { return uint8((int(f)*int(a) + int(b)*(255-int(a)) + 127) / 255) }
	return mix(fr, br), mix(fgg, bgg), mix(fb, bb)
}

func bgSGR(c color.Color) string {
	r, g, b := rgb(c)
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}
