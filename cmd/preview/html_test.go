package main

import (
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/dotmatrx/yadokari/internal/gfx"
)

// A kitty-tier glyph comes out of ansiToHTML as one image over its cells:
// the upload's PNG, sized cols×rows, with the other placeholder cells as
// spaces so the columns still line up.
func TestAnsiToHTMLKittyImage(t *testing.T) {
	const cols, rows = 6, 3
	r := gfx.New(gfx.Kitty, 10, 20)
	src := gfx.Source{Key: "test", Text: "人", Mask: func(w, h int) (*image.Alpha, error) {
		m := image.NewAlpha(image.Rect(0, 0, w, h))
		m.SetAlpha(1, 1, color.Alpha{A: 0xff})
		return m, nil
	}}
	cells, err := r.Draw(src, cols, rows, color.White, color.Black)
	if err != nil {
		t.Fatal(err)
	}
	out := ansiToHTML(r.Flush() + "a" + strings.ReplaceAll(cells, "\n", "b\na") + "b")

	imgs := regexp.MustCompile(`<img alt="" src="data:image/png;base64,([^"]+)" style="width:(\d+)ch;height:(\d+)lh">`).FindAllStringSubmatch(out, -1)
	if len(imgs) != 1 || imgs[0][2] != "6" || imgs[0][3] != "3" {
		t.Fatalf("want one 6ch×3lh image, got %v in\n%s", imgs, out)
	}
	raw, err := base64.StdEncoding.DecodeString(imgs[0][1])
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != cols*10 || b.Dy() != rows*20 {
		t.Errorf("image %v, want %d×%d", b, cols*10, rows*20)
	}
	if strings.ContainsRune(out, kitty.Placeholder) {
		t.Error("placeholder characters left in the HTML")
	}
	// Every row is still cols cells between its markers.
	text := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(out, "")
	for i, line := range strings.Split(text, "\n") {
		if len([]rune(line)) != cols+2 {
			t.Errorf("row %d is %d cells: %q", i, len([]rune(line))-2, line)
		}
	}
}

// Without kitty graphics, nothing changes: no images.
func TestAnsiToHTMLPlain(t *testing.T) {
	if out := ansiToHTML("\x1b[1mbold\x1b[m 人"); strings.Contains(out, "<img") {
		t.Errorf("unexpected image: %s", out)
	}
}
