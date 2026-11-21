package ui

import (
	"image/color"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jtharris/yadokari/internal/gfx"
)

// How a glyph field shows its subject type's colour, for every theme:
// filled (the glyph on a block of the colour, in the theme's ink), or the
// terminal's own background inside a frame of the colour, the glyph white.
const (
	fieldFill    = "fill"
	fieldBlock   = "block"   // half blocks ▛▀▜ ▌▐ ▙▄▟
	fieldThick   = "thick"   // heavy lines ┏━┓
	fieldRails   = "rails"   // a bar down each side ▌ ▐
	fieldCorners = "corners" // brackets at the corners
)

// FieldStyles are the styles SetFieldStyle takes; DefaultFieldStyle is
// the app's.
var (
	FieldStyles       = []string{fieldBlock, fieldFill, fieldThick, fieldRails, fieldCorners}
	DefaultFieldStyle = fieldBlock
	fieldStyle        = DefaultFieldStyle
)

// SetFieldStyle sets how glyph fields are drawn; it reports false (and
// changes nothing) for an unknown style.
func SetFieldStyle(s string) bool {
	if !slices.Contains(FieldStyles, s) {
		return false
	}
	fieldStyle = s
	return true
}

// imageSize is the glyph's own size inside a cols×rows field.
func imageSize(cols, rows int) (int, int) {
	switch {
	case fieldStyle == fieldFill || cols < 6 || rows < 3:
		return cols, rows
	case fieldStyle == fieldRails:
		return cols - 2, rows
	}
	return cols - 2, rows - 2
}

// framedGlyph draws a glyph field of cols×rows cells in the type colour c.
// draw draws the glyph into the cells it is given, in ink fg on fill;
// text says it draws text (the plain tier), whose colour inside a frame
// can be left to the terminal.
func framedGlyph(cols, rows int, c color.Color, text bool, draw func(cols, rows int, fg, fill color.Color) (string, error)) (string, error) {
	inCols, inRows := imageSize(cols, rows)
	if inCols == cols && inRows == rows {
		return draw(cols, rows, glyphFG, c)
	}
	fg := color.Color(frameFG)
	if text {
		fg = frameText
	}
	inner, err := draw(inCols, inRows, fg, lipgloss.NoColor{})
	if err != nil {
		return "", err
	}
	return fieldFrame(fieldStyle, c, cols, strings.Split(inner, "\n")), nil
}

// plainGlyph is the text fallback, in the same field.
func plainGlyph(src gfx.Source, cols, rows int, c color.Color) (string, error) {
	return framedGlyph(cols, rows, c, true, func(cols, rows int, fg, fill color.Color) (string, error) {
		return gfx.New(gfx.Plain, 0, 0).Draw(src, cols, rows, fg, fill)
	})
}

// fieldFrame wraps the inside lines (cols-2 cells wide) in a frame of c.
func fieldFrame(style string, c color.Color, cols int, in []string) string {
	paint := lipgloss.NewStyle().Foreground(c).Render
	inW := cols - 2
	var out []string
	switch style {
	case fieldRails:
		for _, l := range in {
			out = append(out, paint("▌")+l+paint("▐"))
		}
		return strings.Join(out, "\n")
	case fieldCorners:
		arm := max(min(inW/6, 6), 2)
		gap := strings.Repeat(" ", max(inW-2*arm, 0))
		out = append(out, paint("┏"+strings.Repeat("━", arm))+gap+paint(strings.Repeat("━", arm)+"┓"))
		for i, l := range in {
			side := " "
			if i == 0 || i == len(in)-1 {
				side = paint("┃")
			}
			out = append(out, side+l+side)
		}
		out = append(out, paint("┗"+strings.Repeat("━", arm))+gap+paint(strings.Repeat("━", arm)+"┛"))
		return strings.Join(out, "\n")
	}
	b := lipgloss.OuterHalfBlockBorder()
	if style == fieldThick {
		b = lipgloss.ThickBorder()
	}
	out = append(out, paint(b.TopLeft+strings.Repeat(b.Top, inW)+b.TopRight))
	for _, l := range in {
		out = append(out, paint(b.Left)+l+paint(b.Right))
	}
	out = append(out, paint(b.BottomLeft+strings.Repeat(b.Bottom, inW)+b.BottomRight))
	return strings.Join(out, "\n")
}
