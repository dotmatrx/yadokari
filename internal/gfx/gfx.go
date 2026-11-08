// Package gfx draws a large glyph into a block of terminal cells using the
// best tier the terminal supports:
//
//	Kitty  kitty graphics via Unicode placeholders (kitty, Ghostty): the
//	       glyph rasterized at full size
//	Plain  the characters in the terminal's own font, centred in a coloured
//	       box (every other terminal)
//
// A half-block tier (▀ with truecolor) was tried and dropped: at two pixels
// per cell, dense kanji are an unreadable blur.
//
// Every tier returns ordinary cell content for a Bubble Tea View. The kitty
// tier also queues an upload the first time it draws an image; the caller
// must send Flush() to the terminal (tea.Raw, from Update) for the cells to
// show. Because uploads wait in the queue until flushed, calling Draw from
// View can never lose one.
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
	Kitty Tier = "kitty"
	Plain Tier = "plain"
)

// ParseTier parses a config value; "" or "auto" returns ok=false.
func ParseTier(s string) (Tier, bool, error) {
	switch Tier(strings.ToLower(s)) {
	case "", "auto":
		return "", false, nil
	case Kitty, Plain:
		return Tier(strings.ToLower(s)), true, nil
	}
	return "", false, fmt.Errorf("gfx: unknown graphics tier %q (auto, kitty, plain)", s)
}

// Detect picks a tier from the environment: kitty and Ghostty identify
// themselves; everything else, tmux included (kitty graphics would need
// passthrough), gets Plain.
func Detect(getenv func(string) string) Tier {
	if getenv("TMUX") != "" {
		return Plain
	}
	if getenv("KITTY_WINDOW_ID") != "" || getenv("TERM") == "xterm-kitty" ||
		getenv("TERM_PROGRAM") == "ghostty" || getenv("TERM") == "xterm-ghostty" {
		return Kitty
	}
	return Plain
}

// Source is something to draw: a mask producer plus a text fallback.
type Source struct {
	// Key identifies the content (e.g. "kanji:440"); it keys image caches.
	Key string
	// Text is drawn by the Plain tier ("" for image-only radicals).
	Text string
	// Mask returns a coverage mask of exactly w×h.
	Mask func(w, h int) (*image.Alpha, error)
}

// Renderer draws sources into cols×rows cells on a solid background.
type Renderer interface {
	Tier() Tier
	// Draw returns rows lines of exactly cols cells, joined by "\n".
	Draw(src Source, cols, rows int, fg, bg color.Color) (string, error)
	// Flush returns (and clears) escape sequences that Draw queued for the
	// terminal; "" when there are none.
	Flush() string
	// Reset forgets everything sent to the terminal (e.g. after a screen
	// change), drops queued uploads, and returns the sequence that frees
	// the uploaded images there.
	Reset() string
}

// New returns the renderer for a tier. cellW and cellH are the cell size in
// pixels (used by Kitty; see CellSize).
func New(t Tier, cellW, cellH int) Renderer {
	if t == Kitty {
		return newKitty(cellW, cellH)
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

func bgSGR(c color.Color) string {
	r, g, b := rgb(c)
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
}
