package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/dotmatrx/yadokari/internal/store"
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
	Held      bool // waiting: a development build against the live API
	Lessons   bool // a lesson quiz: items were started, not reviewed
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

func (s Summary) View(w, h int) string {
	w, h = max(w, 40), max(h, 10)
	total := len(s.Correct) + len(s.Incorrect)
	right, noun, done := "no items completed", "reviews", "All reviews uploaded."
	switch {
	case s.Lessons:
		right, noun, done = fmt.Sprintf("%d learned", total), "uploads", "All lessons started."
	case total > 0:
		right = fmt.Sprintf("%d reviewed · %d%% correct", total, len(s.Correct)*100/total)
	}
	header := headerLine(appTitle("summary"), []segment{{text: mutedSt.Render(right), rank: rankKey}}, w, nil)

	var status string
	switch {
	case s.Held && s.Pending > 0:
		status = warnSt.Render(fmt.Sprintf("%d %s held, not sent: development builds don't write to the live API", s.Pending, noun))
	case s.Pending > 0:
		status = mutedSt.Render(fmt.Sprintf("%d %s still uploading", s.Pending, noun))
	default:
		status = mutedSt.Render(done)
	}
	footer := spread(" "+status, mutedSt.Render("Enter dashboard")+" ", w)

	var body []string
	if len(s.Incorrect) > 0 {
		body = append(body, panel(titleSt.Render(fmt.Sprintf("Answered incorrectly (%d)", len(s.Incorrect))), itemGrid(s.Incorrect, w-4), w, 0))
	}
	if len(s.Correct) > 0 {
		title := fmt.Sprintf("Answered correctly (%d)", len(s.Correct))
		if s.Lessons {
			title = fmt.Sprintf("Learned (%d)", len(s.Correct))
		}
		body = append(body, panel(titleSt.Render(title), itemGrid(s.Correct, w-4), w, 0))
	}
	return frame(header, strings.Join(body, "\n"), footer, h)
}

// itemGrid lays items out in equal columns: the item on the left of each
// cell, its SRS movement right-aligned.
func itemGrid(items []SummaryItem, width int) string {
	const gap = 3
	cellW := 0
	for _, it := range items {
		cellW = max(cellW, lipgloss.Width(summaryCell(it, 0)))
	}
	cellW = min(cellW+2, width)
	cols := max((width+gap)/(cellW+gap), 1)
	cellW = (width+gap)/cols - gap // share the spare width out
	var lines []string
	for start := 0; start < len(items); start += cols {
		var row []string
		for _, it := range items[start:min(start+cols, len(items))] {
			row = append(row, summaryCell(it, cellW))
		}
		lines = append(lines, strings.Join(row, strings.Repeat(" ", gap)))
	}
	return strings.Join(lines, "\n")
}

// summaryCell renders one item; with w > 0 the movement is right-aligned
// in w cells.
func summaryCell(it SummaryItem, w int) string {
	mark := lipgloss.NewStyle().Foreground(typeColour[it.Type]).Render("▌")
	left := mark + it.Characters + mutedSt.Render(" "+it.Meaning)
	mv := it.movement()
	if w == 0 {
		return left + "  " + mv
	}
	if mv == "" {
		return padRight(left, w)
	}
	return spread(padRight(left, max(w-lipgloss.Width(mv)-1, 1)), mv, w)
}

// isEnter reports whether a key returns from the summary.
func isEnter(msg tea.KeyPressMsg) bool {
	s := msg.String()
	return s == "enter" || s == "esc" || s == "q"
}
