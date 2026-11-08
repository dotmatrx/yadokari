package glyph

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jtharris/yadokari/internal/wk"
)

func loadTestFont(t *testing.T) *Font {
	t.Helper()
	f, err := FindFont("")
	if err != nil {
		t.Skip(err)
	}
	return f
}

func ink(m *image.Alpha) (n int, minX, maxX, minY, maxY int) {
	b := m.Bounds()
	minX, minY = b.Max.X, b.Max.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if m.AlphaAt(x, y).A > 127 {
				n++
				minX, maxX, minY, maxY = min(minX, x), max(maxX, x), min(minY, y), max(maxY, y)
			}
		}
	}
	return
}

func TestFindFontPicksJapanese(t *testing.T) {
	f := loadTestFont(t)
	t.Logf("font: %s", f.Name())
	if !covers(f.sfnt) {
		t.Error("chosen font lacks kanji/kana")
	}
}

func TestTextMask(t *testing.T) {
	f := loadTestFont(t)
	m, err := f.Text("鬱", 200, 200)
	if err != nil {
		t.Fatal(err)
	}
	n, x0, x1, y0, y1 := ink(m)
	if n < 4000 {
		t.Fatalf("too little ink: %d", n)
	}
	// Roughly centred, inside the margins.
	if cx, cy := (x0+x1)/2, (y0+y1)/2; cx < 85 || cx > 115 || cy < 85 || cy > 115 {
		t.Errorf("not centred: x %d..%d y %d..%d", x0, x1, y0, y1)
	}

	// Two characters fit side by side in a wide box.
	m, _ = f.Text("勉強", 200, 100)
	_, x0, x1, _, _ = ink(m)
	if x1-x0 < 120 || x0 < 2 || x1 > 197 {
		t.Errorf("two-character word spans x %d..%d", x0, x1)
	}

	if _, err := f.Text("x", 0, 10); err == nil {
		t.Error("zero size should fail")
	}
}

func TestMaskFromImage(t *testing.T) {
	// A black square on transparent, like a radical PNG.
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 16; y < 48; y++ {
		for x := 16; x < 48; x++ {
			src.Set(x, y, color.Black)
		}
	}
	m := Mask(src, 100, 50)
	n, x0, x1, y0, y1 := ink(m)
	if n == 0 {
		t.Fatal("no ink")
	}
	// The 64px source scales to ~42px tall (85% of 50); the square is half of it.
	if w, h := x1-x0, y1-y0; w < 17 || w > 25 || h < 17 || h > 25 || (x0+x1)/2 < 45 || (x0+x1)/2 > 55 {
		t.Errorf("square at x %d..%d y %d..%d", x0, x1, y0, y1)
	}
}

func TestPickImage(t *testing.T) {
	img := func(ct, dims, url string) wk.CharacterImage {
		meta, _ := json.Marshal(map[string]string{"dimensions": dims})
		return wk.CharacterImage{URL: url, ContentType: ct, Metadata: meta}
	}
	all := []wk.CharacterImage{img("image/png", "1024x1024", "big"), img("image/png", "64x64", "small"), img("image/svg+xml", "", "svg")}
	if u, ct := PickImage(all); u != "svg" || ct != "image/svg+xml" {
		t.Errorf("should prefer SVG: %s %s", u, ct)
	}
	if u, _ := PickImage(all[:2]); u != "big" {
		t.Errorf("PNG fallback should be the largest: %s", u)
	}
	if u, _ := PickImage(nil); u != "" {
		t.Errorf("no images: %q", u)
	}
}

// radicalSVG mimics WaniKani's radical SVGs: a class-based <style> with a CSS
// variable colour, a clip path, px stroke widths, and open stroked paths.
const radicalSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1000 1000"><defs>` +
	`<clipPath id="a"><path d="M0 0H1000V1000H0z" style="fill:none"/></clipPath>` +
	`<style>.a,.b{fill:none}.b{stroke:var(--color-text, #000);stroke-linecap:square;stroke-width:68px}</style></defs>` +
	`<path d="M200 300H800" class="a b"/><g style="clip-path:url(#a)"><path d="M500 300V800" class="b"/></g></svg>`

func TestSVGMask(t *testing.T) {
	m, err := ImageMask([]byte(radicalSVG), "image/svg+xml", 200, 200)
	if err != nil {
		t.Fatal(err)
	}
	n, x0, x1, y0, y1 := ink(m)
	// A T shape about 108×81 px with ~14 px strokes: a few thousand pixels,
	// not a filled blob (open paths must not be filled; strokes must scale).
	if n < 1500 || n > 4000 {
		t.Errorf("ink %d: strokes unscaled or paths filled", n)
	}
	if w, h := x1-x0, y1-y0; w < 100 || w > 130 || h < 70 || h > 100 {
		t.Errorf("T spans %dx%d (x %d..%d, y %d..%d)", w, h, x0, x1, y0, y1)
	}
	if _, err := SVGMask([]byte("<svg"), 10, 10); err == nil {
		t.Error("broken SVG should fail")
	}
}

func TestLatinFontRejected(t *testing.T) {
	// The toolbox's fontconfig returns this for ":lang=ja"; it must not be used.
	for _, p := range []string{"/usr/share/fonts/google-noto-vf/NotoSans[wght].ttf", "/usr/share/fonts/dejavu-sans-fonts/DejaVuSans.ttf"} {
		if _, err := LoadFont(p); err == nil {
			t.Errorf("%s accepted as a Japanese font", p)
		}
	}
}

func TestImageCache(t *testing.T) {
	var buf bytes.Buffer
	src := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	src.Set(3, 3, color.Black)
	png.Encode(&buf, src)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write(buf.Bytes())
	}))
	defer srv.Close()

	c := ImageCache{Dir: filepath.Join(t.TempDir(), "images")}
	for i := range 2 {
		raw, err := c.Get(context.Background(), srv.URL+"/r.png")
		if err != nil {
			t.Fatal(err)
		}
		if m, err := ImageMask(raw, "image/png", 16, 16); err != nil || m.Bounds().Dx() != 16 {
			t.Fatalf("fetch %d: %v", i, err)
		}
	}
	if hits != 1 {
		t.Errorf("server hit %d times; the second fetch should come from disk", hits)
	}
	files, _ := os.ReadDir(c.Dir)
	if len(files) != 1 {
		t.Errorf("cache dir holds %d files", len(files))
	}
}
