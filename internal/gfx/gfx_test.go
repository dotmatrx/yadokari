package gfx

import (
	"errors"
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
	return Source{Key: key, Text: "口", Mask: func(w, h int) (*image.Alpha, error) {
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
		{map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}, Plain}, // e.g. Ptyxis
		{map[string]string{"TERM": "xterm-kitty", "TMUX": "/tmp/tmux"}, Plain},
	}
	for _, tt := range tests {
		if got := Detect(func(k string) string { return tt.env[k] }); got != tt.want {
			t.Errorf("Detect(%v) = %s, want %s", tt.env, got, tt.want)
		}
	}
	if _, ok, err := ParseTier("auto"); ok || err != nil {
		t.Error("auto")
	}
	if tier, ok, _ := ParseTier("Kitty"); !ok || tier != Kitty {
		t.Error("case-insensitive parse")
	}
	if _, _, err := ParseTier("halfblock"); err == nil {
		t.Error("the removed half-block tier should be rejected")
	}
}

func TestPlain(t *testing.T) {
	r := New(Plain, 0, 0)
	cells, _ := r.Draw(square("k"), 12, 5, white, pink)
	checkGrid(t, "plain", cells, 12, 5)
	if !strings.Contains(ansi.Strip(cells), "口") || r.Flush() != "" {
		t.Error("plain shows the text and sends nothing")
	}
	img := square("r")
	img.Text = ""
	cells, _ = r.Draw(img, 12, 5, white, pink)
	if !strings.Contains(ansi.Strip(cells), "画像") {
		t.Error("image-only radical needs a placeholder, not its name")
	}
}

func TestKitty(t *testing.T) {
	r := New(Kitty, 10, 20)
	cells, err := r.Draw(square("kanji:1"), 8, 4, white, pink)
	if err != nil {
		t.Fatal(err)
	}
	checkGrid(t, "kitty", cells, 8, 4)
	if !strings.Contains(cells, "\x1b[38;5;1m") || !strings.ContainsRune(cells, kitty.Placeholder) {
		t.Error("cells should be placeholders coloured with image ID 1")
	}
	setup := r.Flush()
	for _, want := range []string{"a=T", "U=1", "c=8", "r=4", "f=100", "i=1", ";iVBORw0KGgo"} {
		if !strings.Contains(setup, want) {
			t.Errorf("upload missing %s", want)
		}
	}
	if r.Flush() != "" {
		t.Error("Flush must clear the queue")
	}

	// Same glyph again: nothing queued. Different glyph: next ID.
	r.Draw(square("kanji:1"), 8, 4, white, pink)
	if r.Flush() != "" {
		t.Error("cached image re-uploaded")
	}
	r.Draw(square("kanji:2"), 8, 4, white, pink)
	if s := r.Flush(); !strings.Contains(s, "i=2,") && !strings.Contains(s, "i=2;") {
		t.Errorf("second image should get ID 2: %.80q", s)
	}

	// Reset frees both and starts over.
	if reset := r.Reset(); strings.Count(reset, "a=d") != 2 {
		t.Errorf("reset: %q", reset)
	}
	r.Draw(square("kanji:2"), 8, 4, white, pink)
	if r.Flush() == "" {
		t.Error("after reset the image must be uploaded again")
	}
}

// TestKittyUploadSurvivesDrawInView is the regression test for the blank
// first image: a View that draws (and discards nothing it can't get back)
// before the program is ready must leave the upload queued for Flush.
func TestKittyUploadSurvivesDrawInView(t *testing.T) {
	r := New(Kitty, 10, 20)
	for range 3 { // View may run several times before Update flushes
		r.Draw(square("first"), 8, 4, white, pink)
	}
	if s := r.Flush(); strings.Count(s, "a=T") != 1 {
		t.Fatalf("want exactly one queued upload, got %d", strings.Count(s, "a=T"))
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
	bad.Mask = func(w, h int) (*image.Alpha, error) { return inner(w*2, h*2) }
	if _, err := New(Kitty, 4, 8).Draw(bad, 10, 5, white, pink); err == nil {
		t.Error("oversized mask should be rejected")
	}
}

func TestParseProbe(t *testing.T) {
	da1 := "\x1b[?62;22c"
	for _, tt := range []struct {
		reply string
		want  Probed
		done  bool
	}{
		{"", Probed{}, false},
		{"\x1b_Gi=31;OK\x1b\\", Probed{Kitty: true}, false},
		// Ghostty in a toolbox.
		{"\x1b_Gi=31;OK\x1b\\\x1b[6;30;12t" + da1, Probed{Kitty: true, CellW: 12, CellH: 30}, true},
		// No graphics, but a cell size.
		{"\x1b[6;17;8t" + da1, Probed{CellW: 8, CellH: 17}, true},
		{da1, Probed{}, true},
		// Graphics refused.
		{"\x1b_Gi=31;EINVAL:bad\x1b\\" + da1, Probed{}, true},
		// DA1 not complete yet.
		{"\x1b[?62;22", Probed{}, false},
		// Palette colours: 16-bit (xterm, Ghostty), 8-bit, ended by BEL or ST.
		{"\x1b]4;4;rgb:8181/a2a2/bebe\x1b\\\x1b]4;6;rgb:8a/be/b7\a" + da1,
			func() Probed {
				var p Probed
				p.Palette[4] = color.RGBA{0x81, 0xa2, 0xbe, 0xff}
				p.Palette[6] = color.RGBA{0x8a, 0xbe, 0xb7, 0xff}
				return p
			}(), true},
		// Text and background colours (a light terminal), ended by ST or BEL.
		{"\x1b]10;rgb:3b3b/3b3b/3b3b\x1b\\\x1b]11;rgb:fd/f6/e3\a" + da1,
			Probed{Foreground: color.RGBA{0x3b, 0x3b, 0x3b, 0xff}, Background: color.RGBA{0xfd, 0xf6, 0xe3, 0xff}}, true},
		// Feature query for placeholders: supported, refused, and a terminal
		// without feature queries treating it as a bad ordinary query.
		{"\x1b_Gi=31;OK\x1b\\\x1b_Gi=32,e=T,U=1;OK\x1b\\" + da1, Probed{Kitty: true, Placeholders: Supported}, true},
		{"\x1b_Gi=31;OK\x1b\\\x1b_Ge=T,i=32;OK\x1b\\" + da1, Probed{Kitty: true, Placeholders: Unsupported}, true},
		{"\x1b_Gi=31;OK\x1b\\\x1b_Gi=32;EINVAL:bad\x1b\\" + da1, Probed{Kitty: true}, true},
		// Real replies to the graphics and feature queries (2026-11-23): none
		// of these terminals has feature queries yet.
		{"\x1b_Gi=31;OK\x1b\\\x1b_Gi=32;OK\x1b\\\x1b[?65;4;6;18;22c", Probed{Kitty: true}, true},                          // WezTerm
		{"\x1b_Gi=31;OK\x1b\\\x1b_Gi=32;EINVAL: dimensions required\x1b\\\x1b[?62;4;6;22;52c", Probed{Kitty: true}, true}, // Rio
		{"\x1b_Gi=31;OK\x1b\\\x1b_Gi=32;EINVAL: dimensions required\x1b\\\x1b[?62;22;52c", Probed{Kitty: true}, true},     // Ghostty
		// XTVERSION, ended by ST or BEL.
		{"\x1b_Gi=31;OK\x1b\\\x1bP>|WezTerm 20240203-110809-5046fc22\x1b\\" + da1,
			Probed{Kitty: true, Terminal: "WezTerm 20240203-110809-5046fc22"}, true},
		{"\x1bP>|kitty(0.44.0)\a" + da1, Probed{Terminal: "kitty(0.44.0)"}, true},
		// Only the background reported.
		{"\x1b]11;rgb:0000/0000/0000\x1b\\" + da1, Probed{Background: color.RGBA{0, 0, 0, 0xff}}, true},
	} {
		got, done := parseProbe([]byte(tt.reply))
		if got != tt.want || done != tt.done {
			t.Errorf("parseProbe(%q) = %+v, %v; want %+v, %v", tt.reply, got, done, tt.want, tt.done)
		}
	}
}

func TestChoose(t *testing.T) {
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	yes := func() (Probed, error) { return Probed{Kitty: true}, nil }
	no := func() (Probed, error) { return Probed{}, nil }
	wezterm := func() (Probed, error) { return Probed{Kitty: true, Terminal: "WezTerm 20240203-110809-5046fc22"}, nil }
	says := func(s Support, term string) func() (Probed, error) {
		return func() (Probed, error) { return Probed{Kitty: true, Placeholders: s, Terminal: term}, nil }
	}
	rio := func() (Probed, error) { return Probed{Kitty: true, Terminal: "Rio 0.2.30"}, nil }
	fails := func() (Probed, error) { return Probed{}, errors.New("no tty") }
	mustNot := func() (Probed, error) { t.Error("probe should not run"); return Probed{}, nil }
	for _, tt := range []struct {
		name     string
		override string
		getenv   func(string) string
		probe    func() (Probed, error)
		want     Tier
	}{
		{"override plain", "plain", env("TERM", "xterm-kitty"), mustNot, Plain},
		{"override kitty", "kitty", env("TMUX", "1"), mustNot, Kitty},
		{"tmux", "auto", env("TMUX", "1", "TERM", "xterm-ghostty"), mustNot, Plain},
		{"env ghostty", "", env("TERM", "xterm-ghostty"), mustNot, Kitty},
		{"toolbox hides ghostty; probe says yes", "auto", env("TERM", "xterm-256color"), yes, Kitty},
		{"wezterm by XTVERSION", "auto", env("TERM", "xterm-256color"), wezterm, Plain},
		{"wezterm by env", "auto", env("TERM_PROGRAM", "WezTerm"), mustNot, Plain},
		{"wezterm forced", "kitty", env("TERM_PROGRAM", "WezTerm"), mustNot, Kitty},
		{"feature query: yes", "auto", env(), says(Supported, ""), Kitty},
		{"feature query: no", "auto", env(), says(Unsupported, "kitty(0.50.1)"), Plain},
		{"feature query beats the list", "auto", env(), says(Supported, "WezTerm 2027"), Kitty},
		{"rio", "auto", env("TERM", "xterm-256color"), rio, Kitty},
		{"ptyxis", "auto", env("TERM", "xterm-256color"), no, Plain},
		{"no tty", "auto", env(), fails, Plain},
		{"NO_COLOR", "auto", env("NO_COLOR", "1", "TERM", "xterm-ghostty"), mustNot, Plain},
		{"NO_COLOR beats an override", "kitty", env("NO_COLOR", "1"), mustNot, Plain},
	} {
		got, err := Choose(tt.override, tt.getenv, tt.probe)
		if err != nil || got != tt.want {
			t.Errorf("%s: got %s, %v; want %s", tt.name, got, err, tt.want)
		}
	}
	if _, err := Choose("sixel", env(), mustNot); err == nil {
		t.Error("unknown override should fail")
	}
}

func TestBgSGR(t *testing.T) {
	for _, tt := range []struct {
		c    color.Color
		want string
	}{
		{ansi.BasicColor(4), "\x1b[44m"},
		{ansi.BasicColor(13), "\x1b[105m"},
		{ansi.IndexedColor(100), "\x1b[48;5;100m"},
		{color.RGBA{1, 2, 3, 255}, "\x1b[48;2;1;2;3m"},
	} {
		if got := BgSGR(tt.c); got != tt.want {
			t.Errorf("BgSGR(%v) = %q, want %q", tt.c, got, tt.want)
		}
	}
}
