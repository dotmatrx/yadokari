package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jtharris/yadokari/internal/markup"
)

// renderMarkup renders WaniKani markup wrapped to width, paragraphs separated
// by a blank line.
func renderMarkup(src string, width int) string {
	paras := markup.Parse(src)
	out := make([]string, 0, len(paras))
	for _, p := range paras {
		var b strings.Builder
		for _, sp := range p {
			st := markupSt[sp.Kind]
			if sp.Emph {
				st = st.Italic(true)
			}
			if sp.URL != "" {
				st = st.Hyperlink(sp.URL)
			}
			b.WriteString(st.Render(sp.Text))
		}
		out = append(out, lipgloss.NewStyle().Width(max(width, 1)).Render(b.String()))
	}
	return strings.Join(out, "\n\n")
}
