package glyph

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
)

const fcSample = `/f/NotoSansCJK-Bold.ttc	Noto Sans CJK JP	Bold	0	False
/f/NotoSansCJK-Regular.ttc	Noto Sans CJK JP	Regular	0	False
/f/NotoSansCJK-Regular.ttc	Noto Sans Mono CJK JP	Regular	5	False
/f/NotoSansCJK-Regular.ttc	Noto Sans CJK SC	Regular	2	False
/f/NotoSerifCJK-VF.ttc	Noto Serif CJK JP	Regular	0	True
/f/NotoSerifCJK-ExtraLight.ttc	Noto Serif CJK JP	ExtraLight	0	False
/f/ipaexm.ttf	IPAexMincho	Regular	0	False
/f/Yomogi-Regular.ttf	Yomogi	Regular	0	False
garbage line
`

func TestParseFCList(t *testing.T) {
	got := parseFCList(fcSample)
	want := []FontRef{
		{"/f/NotoSansCJK-Regular.ttc", 0, "Noto Sans CJK JP"},      // Regular beats Bold
		{"/f/NotoSerifCJK-ExtraLight.ttc", 0, "Noto Serif CJK JP"}, // the VF is skipped
		{"/f/ipaexm.ttf", 0, "IPAexMincho"},
		{"/f/Yomogi-Regular.ttf", 0, "Yomogi"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestSelectFonts(t *testing.T) {
	installed := parseFCList(fcSample)
	if got, _ := SelectFonts(installed, nil); len(got) != len(installed) {
		t.Errorf("no names: all fonts, got %d", len(got))
	}
	got, err := SelectFonts(installed, []string{"yomogi", "IPAexMincho"})
	if err != nil || len(got) != 2 || got[0].Family != "Yomogi" || got[1].Family != "IPAexMincho" {
		t.Errorf("by family: %+v %v", got, err)
	}
	if _, err := SelectFonts(installed, []string{"Comic Sans"}); err == nil {
		t.Error("an unknown font must be an error")
	}
}

func TestPoolNeverRepeatsAndDropsBroken(t *testing.T) {
	refs := []FontRef{{Path: "a"}, {Path: "b"}, {Path: "broken"}, {Path: "c"}}
	p := NewPool(refs)
	p.load = func(r FontRef) (*Font, error) {
		if r.Path == "broken" {
			return nil, errors.New("bad font")
		}
		return &Font{Path: r.Path, Family: r.Path}, nil
	}
	rng := rand.New(rand.NewPCG(1, 2))
	seen := map[string]bool{}
	prev := ""
	for range 200 {
		f := p.Pick(rng, "漢")
		if f == nil {
			t.Fatal("no font")
		}
		if f.Path == prev {
			t.Fatalf("%s twice in a row", f.Path)
		}
		prev = f.Path
		seen[f.Path] = true
	}
	if len(seen) != 3 || p.Len() != 3 {
		t.Errorf("seen %v, pool %d", seen, p.Len())
	}

	one := NewPool([]FontRef{{Path: "a"}})
	one.load = p.load
	if one.Pick(rng, "漢") == nil || one.Pick(rng, "漢") == nil {
		t.Error("a pool of one repeats its font")
	}
	none := NewPool([]FontRef{{Path: "broken"}})
	none.load = p.load
	if none.Pick(rng, "漢") != nil {
		t.Error("nothing loads: nil")
	}
}

// The real fonts on this machine, when fontconfig lists any.
func TestListFontsLoad(t *testing.T) {
	refs, err := ListFonts()
	if err != nil || len(refs) == 0 {
		t.Skip("no fontconfig or no Japanese fonts")
	}
	for _, r := range refs {
		f, err := LoadFontAt(r)
		if err != nil {
			t.Errorf("%+v: %v", r, err)
			continue
		}
		if _, err := f.Text("漢", 32, 32); err != nil {
			t.Errorf("%s: %v", f.Name(), err)
		}
		t.Logf("%s", f.Name())
	}
}

// A radical some handwritten fonts lack (Yomogi has no 扌): the roulette
// only picks fonts that have it.
func TestPoolPicksCoveringFonts(t *testing.T) {
	refs, err := ListFonts()
	if err != nil || len(refs) < 2 {
		t.Skip("needs two or more Japanese fonts")
	}
	p := NewPool(refs)
	rng := rand.New(rand.NewPCG(3, 4))
	for range 50 {
		f := p.Pick(rng, "扌")
		if f != nil && !f.Covers("扌") {
			t.Fatalf("%s lacks 扌", f.Name())
		}
	}
	if p.Pick(rng, "\U000F0000") != nil { // private use: no font has it
		t.Error("no font covers it: nil")
	}
}
