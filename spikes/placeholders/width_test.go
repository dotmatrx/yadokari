package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

func TestPlaceholderWidth(t *testing.T) {
	var b strings.Builder
	for c := 0; c < 20; c++ {
		b.WriteRune(kitty.Placeholder)
		b.WriteRune(kitty.Diacritic(3))
		b.WriteRune(kitty.Diacritic(c))
	}
	row := "\x1b[38;5;7m" + b.String() + "\x1b[m"
	t.Logf("lipgloss.Width=%d ansi.StringWidth=%d (want 20)", lipgloss.Width(row), ansi.StringWidth(row))
	if lipgloss.Width(row) != 20 || ansi.StringWidth(row) != 20 {
		t.Fail()
	}
}
