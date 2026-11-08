package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Frame pieces shared by every screen. Lines use the terminal's own grey
// (ANSI 8) so they follow the user's theme, and nothing paints a
// background: a translucent terminal shows through.
var (
	lineSt  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	titleSt = lipgloss.NewStyle().Bold(true)
	labelSt = mutedSt
)

// padRight pads (or truncates) s to exactly w cells.
func padRight(s string, w int) string {
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

// spread puts left and right on one line of width w.
func spread(left, right string, w int) string {
	gap := max(w-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

// panel draws a thin rounded box w cells wide with the title (already
// styled) set into its top border. h is the total height including borders (0: fit the body).
func panel(title, body string, w, h int) string { return panelWith(lineSt, title, body, w, h) }

// panelWith is panel with its border drawn in the given style.
func panelWith(border lipgloss.Style, title, body string, w, h int) string {
	inner := max(w-4, 1)
	lines := strings.Split(body, "\n")
	if h > 0 {
		for len(lines) < h-2 {
			lines = append(lines, "")
		}
		lines = lines[:max(h-2, 0)]
	}
	var b strings.Builder
	b.WriteString(border.Render("╭─"))
	if title != "" {
		b.WriteString(" " + title + " ")
	}
	used := 2 + lipgloss.Width(title)
	if title != "" {
		used += 2
	}
	b.WriteString(border.Render(strings.Repeat("─", max(w-used-1, 0)) + "╮"))
	for _, l := range lines {
		b.WriteString("\n" + border.Render("│") + " " + padRight(l, inner) + " " + border.Render("│"))
	}
	b.WriteString("\n" + border.Render("╰"+strings.Repeat("─", max(w-2, 0))+"╯"))
	return b.String()
}

// sideBySide joins blocks horizontally with a gap.
func sideBySide(gap int, blocks ...string) string {
	var parts []string
	for i, bl := range blocks {
		if i > 0 && gap > 0 {
			parts = append(parts, strings.Repeat(" ", gap))
		}
		parts = append(parts, bl)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// frame stacks header, body and footer into exactly h lines: the footer
// sits on the last row however short the body is, and a body too tall for
// the terminal is clipped.
func frame(header, body, footer string, h int) string {
	lines := []string{}
	if header != "" {
		lines = append(lines, strings.Split(header, "\n")...)
	}
	lines = append(lines, strings.Split(body, "\n")...)
	foot := strings.Split(footer, "\n")
	if len(lines)+len(foot) > h { // too short a terminal: clip the body
		lines = lines[:max(h-len(foot), 0)]
	}
	for len(lines)+len(foot) < h {
		lines = append(lines, "")
	}
	return strings.Join(append(lines, foot...), "\n")
}

// srsGroups are WaniKani's stage groups as index ranges into SRSCounts.
var srsGroups = []struct {
	name     string
	from, to int
}{
	{"Apprentice", 1, 4}, {"Guru", 5, 6}, {"Master", 7, 7}, {"Enlightened", 8, 8}, {"Burned", 9, 9},
}

func groupCounts(c [10]int) []int {
	out := make([]int, len(srsGroups))
	for i, g := range srsGroups {
		for s := g.from; s <= g.to; s++ {
			out[i] += c[s]
		}
	}
	return out
}

// stageColumns lays the stage groups out as evenly spaced columns: the
// name above, the count below.
func stageColumns(c [10]int, width int) string {
	counts := groupCounts(c)
	col := max(width/len(srsGroups), 12)
	var names, nums strings.Builder
	for i, g := range srsGroups {
		names.WriteString(padRight(labelSt.Render(g.name), col))
		nums.WriteString(padRight(boldSt.Render(comma(counts[i])), col))
	}
	return strings.TrimRight(names.String(), " ") + "\n" + strings.TrimRight(nums.String(), " ")
}

// progressBar is the level bar: passed kanji filled, the level-up point
// marked.
func progressBar(p progressLike, barW int) string {
	if p.kanji == 0 {
		return ""
	}
	filled := p.passed * barW / p.kanji
	mark := min(p.needed*barW/p.kanji, barW-1)
	var bar strings.Builder
	for i := range barW {
		switch {
		case i < filled:
			bar.WriteString(lessonSt.Render("━"))
		case i == mark:
			bar.WriteString("┃")
		default:
			bar.WriteString(lineSt.Render("━"))
		}
	}
	return bar.String()
}

type progressLike struct{ passed, needed, kanji int }
