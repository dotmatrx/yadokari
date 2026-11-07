package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestRasterizeAndSequence(t *testing.T) {
	_, f, err := loadFont()
	if err != nil {
		t.Skip(err)
	}
	img, err := rasterize(f, "鬱", 200, 200)
	if err != nil {
		t.Fatal(err)
	}
	ink, minX, maxX := 0, 200, 0
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			if img.RGBAAt(x, y).A > 128 {
				ink++
				minX, maxX = min(minX, x), max(maxX, x)
			}
		}
	}
	t.Logf("ink pixels %d, x span %d..%d", ink, minX, maxX)
	if ink < 2000 {
		t.Fatal("glyph barely drawn")
	}
	m := &model{face: f, sent: map[string]bool{}, cellW: 10, cellH: 20, ready: true}
	m.transmit()
	if m.err != nil {
		t.Fatal(m.err)
	}
	t.Logf("sent %d bytes", m.sentBytes)
	m.sent = map[string]bool{}
	seq := m.transmit()().(tea.RawMsg).Msg.(string)
	chunks := strings.Count(seq, "\x1b_G")
	first := seq[:strings.Index(seq[3:], "\x1b_G")+3]
	second := seq[len(first):]
	t.Logf("chunks %d; delete cmd %q; first opts %q", chunks, first, second[:min(60, len(second))])
	for _, want := range []string{"a=T", "U=1", "c=20", "r=10", "f=100", "m=1"} {
		if !strings.Contains(second[:strings.Index(second, ";")], want) {
			t.Errorf("first chunk missing %s", want)
		}
	}
	if !strings.Contains(seq, "m=0") {
		t.Error("no final chunk")
	}
}
