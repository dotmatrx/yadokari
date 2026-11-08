package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// SummaryItem is one reviewed item in the summary.
type SummaryItem struct {
	Characters string
	Meaning    string
	Type       string
}

// Summary is the end-of-session report.
type Summary struct {
	Correct   []SummaryItem
	Incorrect []SummaryItem
	Pending   int  // reviews still waiting to upload
	Held      bool // waiting because live writes are disabled
}

func (s Summary) View(width int) string {
	width = max(width, 40)
	total := len(s.Correct) + len(s.Incorrect)
	var b strings.Builder
	b.WriteString(" " + boldSt.Render("Review summary") + "\n\n")
	if total == 0 {
		b.WriteString(" No items were completed.\n")
	} else {
		b.WriteString(fmt.Sprintf(" %d reviewed · %d%% correct\n", total, len(s.Correct)*100/total))
	}
	list := func(title string, items []SummaryItem) {
		if len(items) == 0 {
			return
		}
		b.WriteString("\n " + boldSt.Render(fmt.Sprintf("%s (%d)", title, len(items))) + "\n")
		var cells []string
		for _, it := range items {
			mark := lipgloss.NewStyle().Foreground(typeColour[it.Type]).Render("▌")
			cells = append(cells, mark+it.Characters+mutedSt.Render(" "+it.Meaning))
		}
		// Wrap entries across the width.
		line := " "
		for _, c := range cells {
			if lipgloss.Width(line)+lipgloss.Width(c)+3 > width {
				b.WriteString(line + "\n")
				line = " "
			}
			line += c + "   "
		}
		b.WriteString(line + "\n")
	}
	list("Answered incorrectly", s.Incorrect)
	list("Answered correctly", s.Correct)

	b.WriteString("\n")
	switch {
	case s.Held && s.Pending > 0:
		b.WriteString(" " + warnSt.Render(fmt.Sprintf("%d reviews held, not sent: writes to the live API need --live", s.Pending)) + "\n")
	case s.Pending > 0:
		b.WriteString(mutedSt.Render(fmt.Sprintf(" %d reviews still uploading", s.Pending)) + "\n")
	default:
		b.WriteString(mutedSt.Render(" All reviews uploaded.") + "\n")
	}
	b.WriteString("\n" + mutedSt.Render(" Enter dashboard"))
	return b.String()
}

// isEnter reports whether a key returns from the summary.
func isEnter(msg tea.KeyPressMsg) bool {
	s := msg.String()
	return s == "enter" || s == "esc" || s == "q"
}
