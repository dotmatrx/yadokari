package ui

import (
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFramedGlyphFillsItsCells(t *testing.T) {
	defer SetFieldStyle(DefaultFieldStyle)
	for _, style := range FieldStyles {
		if !SetFieldStyle(style) {
			t.Fatalf("SetFieldStyle(%q) refused", style)
		}
		for _, sz := range [][2]int{{40, 10}, {6, 3}, {5, 2}} {
			var gotFG color.Color
			out, err := framedGlyph(sz[0], sz[1], rgb(0x5cd6ff), false, func(cols, rows int, fg, _ color.Color) (string, error) {
				gotFG = fg
				return strings.TrimSuffix(strings.Repeat(strings.Repeat("x", cols)+"\n", rows), "\n"), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(out, "\n")
			if len(lines) != sz[1] {
				t.Errorf("%s %v: %d lines, want %d", style, sz, len(lines), sz[1])
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w != sz[0] {
					t.Errorf("%s %v: line %d is %d cells, want %d", style, sz, i, w, sz[0])
				}
			}
			framed := style != fieldFill && sz[0] >= 6 && sz[1] >= 3
			if want := map[bool]color.Color{true: frameFG, false: glyphFG}[framed]; gotFG != want {
				t.Errorf("%s %v: ink %v, want %v", style, sz, gotFG, want)
			}
		}
	}
	if SetFieldStyle("neon") {
		t.Error("SetFieldStyle accepted an unknown style")
	}
}
