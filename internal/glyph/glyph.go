// Package glyph finds a Japanese font and turns subject characters or
// radical images into alpha masks (coverage only, no colour) at any pixel
// size. Rendering tiers in internal/gfx colour and display the masks.
package glyph

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Font is a loaded Japanese font.
type Font struct {
	Path   string
	Family string
	sfnt   *sfnt.Font
}

// Well-known locations of a Japanese CJK font, checked in order after any
// configured path and fontconfig. /run/host covers Fedora toolbox, where the
// host's fonts are not otherwise visible.
var candidates = []string{
	"/usr/share/fonts/google-noto-sans-cjk-fonts/NotoSansCJK-Regular.ttc", // Fedora
	"/run/host/usr/share/fonts/google-noto-sans-cjk-fonts/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc", // Debian, Ubuntu
	"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",      // Arch
	"/System/Library/Fonts/ヒラギノ角ゴシック W3.ttc",                 // macOS
}

// ErrNoFont means no usable Japanese font was found.
var ErrNoFont = errors.New("glyph: no Japanese font found (install Noto Sans CJK JP or set a font path)")

// FindFont loads the configured font if path is non-empty, else asks
// fontconfig for a Japanese font, else tries well-known paths.
func FindFont(path string) (*Font, error) {
	if path != "" {
		return LoadFont(path)
	}
	if out, err := exec.Command("fc-match", "-f", "%{file}", ":lang=ja").Output(); err == nil {
		if p := strings.TrimSpace(string(out)); p != "" {
			if f, err := LoadFont(p); err == nil {
				return f, nil
			}
		}
	}
	for _, p := range candidates {
		if f, err := LoadFont(p); err == nil {
			return f, nil
		}
	}
	return nil, ErrNoFont
}

// coverage is checked in every candidate font: fontconfig happily returns a
// Latin font for ":lang=ja" when it can't see any CJK fonts (e.g. in a
// toolbox, where the host's fonts live under /run/host).
const coverage = "漢鬱あア"

func covers(f *sfnt.Font) bool {
	var buf sfnt.Buffer
	for _, r := range coverage {
		if i, err := f.GlyphIndex(&buf, r); err != nil || i == 0 {
			return false
		}
	}
	return true
}

// LoadFont loads a font file that covers kanji and kana. In a collection
// (.ttc) it prefers the Japanese member ("… JP" family, or Hiragino) so
// kanji take their Japanese forms.
func LoadFont(path string) (*Font, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	coll, err := opentype.ParseCollection(raw)
	if err != nil {
		return nil, fmt.Errorf("glyph: %s: %w", path, err)
	}
	var first *Font
	for i := range coll.NumFonts() {
		f, err := coll.Font(i)
		if err != nil {
			continue
		}
		if !covers(f) {
			continue
		}
		family, _ := f.Name(nil, sfnt.NameIDFamily)
		cand := &Font{Path: path, Family: family, sfnt: f}
		if strings.HasSuffix(family, " JP") || strings.Contains(family, "Hiragino") || coll.NumFonts() == 1 {
			return cand, nil
		}
		if first == nil {
			first = cand
		}
	}
	if first == nil {
		return nil, fmt.Errorf("glyph: %s: no font covering kanji and kana", path)
	}
	return first, nil // a collection without a JP member: better than nothing
}

// Covers reports whether the font has a glyph for every character of text.
// A font without outlines (a test stand-in) covers everything.
func (f *Font) Covers(text string) bool {
	if f.sfnt == nil {
		return true
	}
	var buf sfnt.Buffer
	for _, r := range text {
		if i, err := f.sfnt.GlyphIndex(&buf, r); err != nil || i == 0 {
			return false
		}
	}
	return true
}

// Name is a short description, e.g. "NotoSansCJK-Regular.ttc [Noto Sans CJK JP]".
func (f *Font) Name() string { return fmt.Sprintf("%s [%s]", filepath.Base(f.Path), f.Family) }

// Text draws text centred and as large as fits in a w×h mask.
func (f *Font) Text(text string, w, h int) (*image.Alpha, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("glyph: bad size %dx%d", w, h)
	}
	n := max(len([]rune(text)), 1)
	size := min(float64(h)*0.85, float64(w)*0.9/float64(n))
	face, err := opentype.NewFace(f.sfnt, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, err
	}
	defer face.Close()

	m := image.NewAlpha(image.Rect(0, 0, w, h))
	d := &font.Drawer{Dst: m, Src: image.Opaque, Face: face}
	b, _ := d.BoundString(text)
	gw, gh := (b.Max.X - b.Min.X).Ceil(), (b.Max.Y - b.Min.Y).Ceil()
	d.Dot = fixed.P((w-gw)/2-b.Min.X.Floor(), (h-gh)/2-b.Min.Y.Floor())
	d.DrawString(text)
	return m, nil
}

// Mask fits an image's alpha channel (radical PNGs are black on
// transparent) centred into a w×h mask, keeping its aspect ratio and
// leaving a margin like Text does.
func Mask(src image.Image, w, h int) *image.Alpha {
	out := image.NewAlpha(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	if sb.Empty() || w <= 0 || h <= 0 {
		return out
	}
	box := min(float64(w)*0.9/float64(sb.Dx()), float64(h)*0.85/float64(sb.Dy()))
	dw, dh := max(int(float64(sb.Dx())*box), 1), max(int(float64(sb.Dy())*box), 1)
	alpha := image.NewAlpha(sb)
	draw.Draw(alpha, sb, src, sb.Min, draw.Src)
	dst := image.Rect((w-dw)/2, (h-dh)/2, (w-dw)/2+dw, (h-dh)/2+dh)
	xdraw.CatmullRom.Scale(out, dst, alpha, sb, xdraw.Src, nil)
	return out
}
