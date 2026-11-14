package ui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
func panel(title, body string, w, h int) string { return panelWith(lineSt, title, "", body, w, h) }

// panelWith is panel with its border drawn in the given style and an
// optional (styled) note set into the right of the top border.
func panelWith(border lipgloss.Style, title, note, body string, w, h int) string {
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
	right := border.Render("╮")
	if note != "" {
		right = " " + note + " " + border.Render("─╮")
	}
	b.WriteString(border.Render(strings.Repeat("─", max(w-used-lipgloss.Width(right), 0))) + right)
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

// segment is one part of the header's right side (already styled). When
// the row is too narrow, the lowest rank goes first: replaced by its alt
// (a shorter form, which also drops sep) if it has one, else dropped. sep
// goes after it, before the next segment; the default is a muted " · ".
type segment struct {
	text string
	rank int
	alt  string
	sep  string
}

// Header ranks: what survives a narrow terminal longest.
const (
	rankExtra  = 1 // nice to have
	rankInfo   = 2
	rankKey    = 3 // the screen's main status
	rankUrgent = 4 // problems and live state (audio)
)

func appTitle(section string) string {
	return boldSt.Render("yadokari") + mutedSt.Render(" · "+section)
}

// headerLine is the top row of every screen, exactly w cells: left (the
// app and section) and the segments on the right. With sh set, the shared
// state's badges (audio) come first on the right.
func headerLine(left string, segs []segment, w int, sh *shared) string {
	if sh != nil {
		if b := sh.audioBadge(); b != "" {
			segs = append([]segment{{text: b, rank: rankUrgent}}, segs...)
		}
	}
	segs = slices.DeleteFunc(slices.Clone(segs), func(s segment) bool { return s.text == "" })
	left = " " + left
	join := func() string {
		var b strings.Builder
		for i, s := range segs {
			if i > 0 {
				sep := segs[i-1].sep
				if sep == "" {
					sep = mutedSt.Render(" · ")
				}
				b.WriteString(sep)
			}
			b.WriteString(s.text)
		}
		return b.String()
	}
	right := join()
	for len(segs) > 0 && lipgloss.Width(left)+lipgloss.Width(right)+2 > w {
		low := 0
		for i, s := range segs {
			if s.rank < segs[low].rank || (s.rank == segs[low].rank && i > low) {
				low = i
			}
		}
		if segs[low].alt != "" {
			segs[low].text, segs[low].alt, segs[low].sep = segs[low].alt, "", ""
		} else {
			segs = slices.Delete(segs, low, low+1)
		}
		right = join()
	}
	if right == "" {
		return padRight(left, w)
	}
	return padRight(spread(left, right+" ", w), w)
}
