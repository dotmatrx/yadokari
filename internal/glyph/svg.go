package glyph

import (
	"bytes"
	"fmt"
	"image"
	"regexp"
	"strconv"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// WaniKani's radical SVGs are stroked paths whose style comes from a CSS
// <style> block (classes), with the colour as var(--color-text, #000) and
// some clip paths. oksvg understands none of those, so SVGs are normalized
// first: class rules are inlined as style attributes, CSS variables are
// replaced by their fallback, and <defs> (styles, clip paths) are dropped.
// Dropping the clips can lengthen a stroke end slightly.
var (
	reStyleBlock = regexp.MustCompile(`(?s)<style[^>]*>(.*?)</style>`)
	reRule       = regexp.MustCompile(`([^{}]+)\{([^}]*)\}`)
	reDefs       = regexp.MustCompile(`(?s)<defs>.*?</defs>`)
	reTag        = regexp.MustCompile(`<[a-zA-Z][^>]*>`)
	reClass      = regexp.MustCompile(`\sclass="([^"]*)"`)
	reStyleAttr  = regexp.MustCompile(`\sstyle="([^"]*)"`)
	reCSSVar     = regexp.MustCompile(`var\(\s*--[\w-]+\s*,\s*([^)]+?)\s*\)`)
	reClip       = regexp.MustCompile(`clip-path\s*:\s*url\([^)]*\)\s*;?`)
	rePx         = regexp.MustCompile(`(\d)px`)
	reStrokeAttr = regexp.MustCompile(`\sstroke-width="([\d.]+)(?:px)?"`)
)

// normalizeSVG also multiplies stroke widths by strokeScale: oksvg scales
// path coordinates to the target size but not stroke widths.
func normalizeSVG(raw []byte, strokeScale float64) []byte {
	s := string(raw)
	rules := map[string]string{} // class → declarations
	for _, block := range reStyleBlock.FindAllStringSubmatch(s, -1) {
		for _, r := range reRule.FindAllStringSubmatch(block[1], -1) {
			decls := strings.TrimSpace(r[2])
			for _, sel := range strings.Split(r[1], ",") {
				if sel = strings.TrimSpace(sel); strings.HasPrefix(sel, ".") {
					rules[sel[1:]] += decls + ";"
				}
			}
		}
	}
	s = reDefs.ReplaceAllString(s, "")
	s = reStyleBlock.ReplaceAllString(s, "")
	s = reTag.ReplaceAllStringFunc(s, func(tag string) string {
		tag = reStrokeAttr.ReplaceAllStringFunc(tag, func(a string) string {
			f, _ := strconv.ParseFloat(reStrokeAttr.FindStringSubmatch(a)[1], 64)
			return ` stroke-width="` + strconv.FormatFloat(f*strokeScale, 'f', 3, 64) + `"`
		})
		style := ""
		if m := reClass.FindStringSubmatch(tag); m != nil {
			for _, c := range strings.Fields(m[1]) {
				style += rules[c]
			}
			tag = reClass.ReplaceAllString(tag, "")
		}
		if m := reStyleAttr.FindStringSubmatch(tag); m != nil {
			style += m[1]
			tag = reStyleAttr.ReplaceAllString(tag, "")
		}
		if style == "" {
			return tag
		}
		style = reCSSVar.ReplaceAllString(style, "$1")
		style = reClip.ReplaceAllString(style, "")
		style = rePx.ReplaceAllString(style, "$1")
		// Emit presentation attributes (fill="none") rather than a style
		// attribute, which oksvg doesn't apply reliably.
		attrs := ""
		for _, decl := range strings.Split(style, ";") {
			k, v, ok := strings.Cut(decl, ":")
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			if !ok || k == "" || v == "" {
				continue
			}
			if k == "stroke-width" {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					v = strconv.FormatFloat(f*strokeScale, 'f', 3, 64)
				}
			}
			attrs += " " + k + `="` + v + `"`
		}
		end := ">"
		if strings.HasSuffix(tag, "/>") {
			end = "/>"
		}
		return strings.TrimSuffix(strings.TrimSuffix(tag, "/>"), ">") + attrs + end
	})
	return []byte(s)
}

// SVGMask renders an SVG's coverage centred into a w×h mask, with the same
// margins as Text.
func SVGMask(raw []byte, w, h int) (*image.Alpha, error) {
	// Read once to learn the viewBox (and so the scale), then again with
	// stroke widths scaled to match.
	probe, err := oksvg.ReadIconStream(bytes.NewReader(normalizeSVG(raw, 1)), oksvg.WarnErrorMode)
	if err != nil {
		return nil, fmt.Errorf("glyph: svg: %w", err)
	}
	vb := probe.ViewBox
	if vb.W <= 0 || vb.H <= 0 {
		return nil, fmt.Errorf("glyph: svg: no viewBox")
	}
	scale := min(float64(w)*0.9/vb.W, float64(h)*0.85/vb.H)
	icon, err := oksvg.ReadIconStream(bytes.NewReader(normalizeSVG(raw, scale)), oksvg.WarnErrorMode)
	if err != nil {
		return nil, fmt.Errorf("glyph: svg: %w", err)
	}
	dw, dh := vb.W*scale, vb.H*scale
	icon.SetTarget((float64(w)-dw)/2, (float64(h)-dh)/2, dw, dh)

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	icon.Draw(rasterx.NewDasher(w, h, scanner), 1)

	out := image.NewAlpha(rgba.Bounds())
	for i := range out.Pix {
		out.Pix[i] = rgba.Pix[4*i+3]
	}
	return out, nil
}
