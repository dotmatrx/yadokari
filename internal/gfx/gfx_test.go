package gfx

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

var (
	white = color.RGBA{255, 255, 255, 255}
	pink  = color.RGBA{0xff, 0x00, 0xaa, 255}
)

// square is a source whose mask is a filled square in the middle.
func square(key string) Source {
	return Source{Key: key, Text: "口", Mask: func(w, h, ss int) (*image.Alpha, error) {
		m := image.NewAlpha(image.Rect(0, 0, w, h))
		for y := h / 4; y < 3*h/4; y++ {
			for x := w / 4; x < 3*w/4; x++ {
				m.SetAlpha(x, y, color.Alpha{255})
			}
		}
		return m, nil
	}}
}

func checkGrid(t *testing.T, name, cells string, cols, rows int) {
	t.Helper()
	lines := strings.Split(cells, "\n")
	if len(lines) != rows {
		t.Fatalf("%s: %d lines, want %d", name, len(lines), rows)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != cols {
			t.Errorf("%s: line %d is %d cells, want %d", name, i, w, cols)
		}
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		env  map[string]string
		want Tier
	}{
		{map[string]string{"TERM": "xterm-kitty"}, Kitty},
		{map[string]string{"KITTY_WINDOW_ID": "1", "TERM": "xterm-256color"}, Kitty},
		{map[string]string{"TERM_PROGRAM": "ghostty"}, Kitty},
		{map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}, HalfBlock}, // e.g. Ptyxis
		{map[string]string{"TERM": "xterm-kitty", "TMUX": "/tmp/tmux", "COLORTERM": "truecolor"}, HalfBlock},
		{map[string]string{"TERM": "xterm-256color"}, Plain},
		{map[string]string{"TERM": "xterm-kitty", "NO_COLOR": "1"}, Plain},
	}
	for _, tt := range tests {
		if got := Detect(func(k string) string { return tt.env[k] }); got != tt.want {
			t.Errorf("Detect(%v) = %s, want %s", tt.env, got, tt.want)
		}
	}
	if _, ok, err := ParseTier("auto"); ok || err != nil {
		t.Error("auto")
	}
	if tier, ok, _ := ParseTier("HalfBlock"); !ok || tier != HalfBlock {
		t.Error("case-insensitive parse")
	}
	if _, _, err := ParseTier("sixel"); err == nil {
		t.Error("unknown tier should fail")
	}
}

func TestHalfBlock(t *testing.T) {
	f, err := New(HalfBlock, 0, 0).Draw(square("k"), 20, 10, white, pink)
	if err != nil {
		t.Fatal(err)
	}
	if f.Setup != "" {
		t.Error("half-block needs no setup")
	}
	checkGrid(t, "halfblock", f.Cells, 20, 10)
	lines := strings.Split(f.Cells, "\n")
	// Corner: background over background. Centre: glyph over glyph.
	if !strings.HasPrefix(lines[0], "\x1b[38;2;255;0;170;48;2;255;0;170m") {
		t.Errorf("corner should be pure background: %q", lines[0][:40])
	}
	if !strings.Contains(lines[5], "\x1b[38;2;255;255;255;48;2;255;255;255m") {
		t.Errorf("centre should be pure foreground: %q", lines[5])
	}
}

func TestPlain(t *testing.T) {
	r := New(Plain, 0, 0)
	f, _ := r.Draw(square("k"), 12, 5, white, pink)
	checkGrid(t, "plain", f.Cells, 12, 5)
	if !strings.Contains(ansi.Strip(f.Cells), "口") {
		t.Error("plain should show the text")
	}
	img := square("r")
	img.Text = ""
	f, _ = r.Draw(img, 12, 5, white, pink)
	if !strings.Contains(ansi.Strip(f.Cells), "画像") {
		t.Error("image-only radical needs a placeholder, not its name")
	}
}

func TestKitty(t *testing.T) {
	r := New(Kitty, 10, 20)
	f, err := r.Draw(square("kanji:1"), 8, 4, white, pink)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"a=T", "U=1", "c=8", "r=4", "f=100", "i=1"} {
		if !strings.Contains(f.Setup, want) {
			t.Errorf("setup missing %s", want)
		}
	}
	if !strings.Contains(f.Setup, ";iVBORw0KGgo") {
		t.Error("setup carries no PNG data")
	}
	checkGrid(t, "kitty", f.Cells, 8, 4)
	if !strings.Contains(f.Cells, "\x1b[38;5;1m") || !strings.ContainsRune(f.Cells, kitty.Placeholder) {
		t.Error("cells should be placeholders coloured with image ID 1")
	}

	// Same glyph again: no upload. Different glyph: next ID.
	if f2, _ := r.Draw(square("kanji:1"), 8, 4, white, pink); f2.Setup != "" {
		t.Error("cached image re-uploaded")
	}
	if f3, _ := r.Draw(square("kanji:2"), 8, 4, white, pink); !strings.Contains(f3.Setup, "i=2,") && !strings.Contains(f3.Setup, "i=2;") {
		t.Errorf("second image should get ID 2: %.80q", f3.Setup)
	}

	// Reset frees both and starts over at ID 1.
	if reset := r.Reset(); strings.Count(reset, "a=d") != 2 {
		t.Errorf("reset: %q", reset)
	}
	if f4, _ := r.Draw(square("kanji:2"), 8, 4, white, pink); f4.Setup == "" {
		t.Error("after reset the image must be uploaded again")
	}
}

func TestKittyRecyclesIDs(t *testing.T) {
	r := New(Kitty, 2, 4).(*kittyRenderer)
	for i := range maxKittyImages + 5 {
		if _, err := r.Draw(square(string(rune('A'+i%26))+strings.Repeat("x", i)), 2, 1, white, pink); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.ids) != maxKittyImages {
		t.Errorf("%d images held, want %d", len(r.ids), maxKittyImages)
	}
	for _, id := range r.ids {
		if id < 1 || id > maxKittyImages {
			t.Fatalf("id %d out of range", id)
		}
	}
}

func TestWrongMaskSizeIsAnError(t *testing.T) {
	bad := square("bad")
	inner := bad.Mask
	bad.Mask = func(w, h, ss int) (*image.Alpha, error) { return inner(w*ss, h*ss, ss) }
	if _, err := New(HalfBlock, 0, 0).Draw(bad, 10, 5, white, pink); err == nil {
		t.Error("oversized mask should be rejected")
	}
}
