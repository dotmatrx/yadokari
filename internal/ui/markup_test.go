package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderMarkup(t *testing.T) {
	src := "The <radical>ground</radical> is <kanji>one</kanji>, read <reading>いち</reading>. " +
		`<em>Really</em>, see <a href="https://example.com">this</a>.` + "\n\nSecond paragraph."
	got := renderMarkup(src, 20)
	plain := ansi.Strip(got)
	for _, line := range strings.Split(plain, "\n") {
		if w := lipgloss.Width(line); w > 20 {
			t.Errorf("line %q is %d wide, want ≤ 20", line, w)
		}
	}
	if want := "The ground is one,"; !strings.HasPrefix(plain, want) {
		t.Errorf("text = %q, want prefix %q", plain, want)
	}
	if !strings.Contains(plain, "\n\nSecond paragraph.") {
		t.Errorf("paragraphs not separated by a blank line: %q", plain)
	}
	if strings.ContainsAny(plain, "<>") {
		t.Errorf("markup left in %q", plain)
	}
	if !strings.Contains(got, "https://example.com") {
		t.Errorf("no hyperlink in %q", got)
	}
}
