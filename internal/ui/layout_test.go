package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHeaderLine(t *testing.T) {
	segs := []segment{
		{text: "12 / 40", rank: rankKey},
		{text: "85% correct", rank: rankExtra},
		{text: "wrapping up", rank: rankInfo},
		{text: "3 held (dry-run)", rank: rankInfo},
	}
	tests := []struct {
		w            int
		want, absent []string
	}{
		{120, []string{"12 / 40 · 85% correct · wrapping up · 3 held (dry-run)"}, nil},
		{62, []string{"12 / 40 · wrapping up · 3 held (dry-run)"}, []string{"85%"}},
		{60, []string{"12 / 40 · wrapping up"}, []string{"85%", "held"}},
		{42, []string{"12 / 40 · wrapping up"}, []string{"85%", "held"}},
		{30, []string{"12 / 40"}, []string{"wrapping"}},
		{12, []string{"yadokari"}, []string{"12 / 40"}},
	}
	for _, tt := range tests {
		got := ansi.Strip(headerLine(appTitle("reviews"), segs, tt.w, nil))
		if w := ansi.StringWidth(got); w != tt.w {
			t.Errorf("w=%d: header is %d wide: %q", tt.w, w, got)
		}
		for _, s := range tt.want {
			if !strings.Contains(got, s) {
				t.Errorf("w=%d: want %q in %q", tt.w, s, got)
			}
		}
		for _, s := range tt.absent {
			if strings.Contains(got, s) {
				t.Errorf("w=%d: %q should have been dropped: %q", tt.w, s, got)
			}
		}
	}

	// A segment's alt replaces it before anything else goes.
	strip := []segment{
		{text: "一 二 三 四 五 六 七 八", alt: "2 / 8", rank: rankInfo, sep: "   "},
		{text: "Kanji · level 3", rank: rankKey},
	}
	if got := ansi.Strip(headerLine(appTitle("lessons"), strip, 50, nil)); !strings.Contains(got, "2 / 8 · Kanji · level 3") {
		t.Errorf("strip should shrink to its alt: %q", got)
	}
	if got := ansi.Strip(headerLine(appTitle("lessons"), strip, 80, nil)); !strings.Contains(got, "一 二 三 四 五 六 七 八   Kanji") {
		t.Errorf("strip should show when it fits: %q", got)
	}

	// The audio badge leads and outlasts the rest.
	sh := &shared{audio: audioPlaying}
	got := ansi.Strip(headerLine(appTitle("reviews"), segs, 42, sh))
	if !strings.Contains(got, "🔊 playing · 12 / 40") || ansi.StringWidth(got) != 42 {
		t.Errorf("badge: %q", got)
	}
}
