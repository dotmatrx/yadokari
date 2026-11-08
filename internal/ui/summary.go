package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jtharris/yadokari/internal/store"
)

// SummaryItem is one reviewed item in the summary.
type SummaryItem struct {
	Characters string
	Meaning    string
	Type       string

	AssignmentID int
	From         int // SRS stage before the review
	To           int // SRS stage the API returned; 0 until uploaded
	Rejected     bool
}

// srsNames are WaniKani's stage names; positions 1–9 of every SRS.
var srsNames = [...]string{"Lesson", "Apprentice 1", "Apprentice 2", "Apprentice 3", "Apprentice 4",
	"Guru 1", "Guru 2", "Master", "Enlightened", "Burned"}

func srsName(stage int) string {
	if stage >= 0 && stage < len(srsNames) {
		return srsNames[stage]
	}
	return fmt.Sprintf("stage %d", stage)
}

// movement describes where an item went, e.g. "Guru 1 ↑". Empty until the
// review is uploaded.
func (it SummaryItem) movement() string {
	switch {
	case it.Rejected:
		return errSt.Render("rejected")
	case it.To == 0:
		return ""
	case it.To > it.From:
		return mutedSt.Render(srsName(it.To) + " ↑")
	case it.To < it.From:
		return warnSt.Render(srsName(it.To) + " ↓")
	}
	return mutedSt.Render(srsName(it.To))
}

// Summary is the end-of-session report.
type Summary struct {
	Correct   []SummaryItem
	Incorrect []SummaryItem
	Pending   int  // reviews still waiting to upload
	Held      bool // waiting because live writes are disabled
}

// Refresh fills in SRS movement for reviews that have been uploaded since,
// from the assignments the API returned, and the pending count.
func (s *Summary) Refresh(ctx context.Context, st *store.Store) error {
	var ids []int
	for _, l := range [][]SummaryItem{s.Correct, s.Incorrect} {
		for _, it := range l {
			ids = append(ids, it.AssignmentID)
		}
	}
	out, err := st.ReviewOutcomes(ctx, ids)
	if err != nil {
		return err
	}
	for _, l := range [][]SummaryItem{s.Correct, s.Incorrect} {
		for i := range l {
			o, ok := out[l[i].AssignmentID]
			switch {
			case !ok:
			case o.Failed != "":
				l[i].Rejected = true
			case !o.Queued:
				l[i].To = o.SRSStage
			}
		}
	}
	s.Pending, _, err = st.OutboxCounts(ctx)
	return err
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
			cell := mark + it.Characters + mutedSt.Render(" "+it.Meaning)
			if mv := it.movement(); mv != "" {
				cell += " " + mv
			}
			cells = append(cells, cell)
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
