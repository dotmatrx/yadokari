package ui

import (
	"fmt"
	"strings"

	"github.com/jtharris/yadokari/internal/store"
)

// Two candidate dashboard designs (see Deps.Layout). Both pin a header to
// the top row and the status/key bar to the bottom row, and fill the width.

func (m *Dashboard) headerParts() (left, right string) {
	left = boldSt.Render("yadokari")
	if m.user != nil {
		right = mutedSt.Render(fmt.Sprintf("%s · level %d · ", m.user.Data.Username, m.user.Data.Level))
	}
	right += mutedSt.Render(m.deps.Endpoint)
	return left, right
}

func (m *Dashboard) footerLine(w int) string {
	s := m.statusLine() // status left, keys right, sized to m.width
	return " " + strings.TrimRight(s, " ")
}

func (m *Dashboard) forecastNote() string {
	total := 0
	for _, n := range m.queue.Forecast {
		total += n
	}
	note := fmt.Sprintf("%d reviews", total)
	if m.queue.Next != nil {
		note += " · next " + m.queue.Next.In(m.deps.Location).Format("15:04")
	}
	return note
}

func (m *Dashboard) forecastBody(width, height int) string {
	total := 0
	for _, n := range m.queue.Forecast {
		total += n
	}
	if total == 0 {
		s := "Nothing due in the next 24 hours"
		if m.queue.Next != nil {
			s += " · next " + m.queue.Next.In(m.deps.Location).Format("Mon 2 Jan 15:04")
		}
		return mutedSt.Render(s)
	}
	return forecastChart(m.queue.Forecast, m.queue.Start, m.deps.Location, width, height)
}

func levelText(p store.LevelProgress) string {
	if p.Kanji == 0 {
		return mutedSt.Render("no kanji at this level")
	}
	s := fmt.Sprintf("%d / %d kanji passed", p.Passed, p.Kanji)
	if p.Passed < p.Needed {
		s += mutedSt.Render(fmt.Sprintf(" · %d more to level up", p.Needed-p.Passed))
	}
	return s
}

func (m *Dashboard) rejectedLines(width int) []string {
	var out []string
	for i, r := range m.rejected {
		if i == maxRejectedShown {
			out = append(out, mutedSt.Render(fmt.Sprintf("… and %d more", len(m.rejected)-i)))
			break
		}
		head := markOf(r.item.Type) + r.item.Characters
		if r.item.Meaning != "" {
			head += " " + r.item.Meaning
		}
		out = append(out, padRight(head+mutedSt.Render("  "+r.reason), width))
	}
	return out
}

func markOf(objType string) string {
	return lessonSt.Foreground(typeColour[objType]).Render("▌")
}

// renderPanels: thin rounded panels on a grid.
func (m *Dashboard) renderPanels() string {
	w, h := max(m.width, 60), max(m.height, 20)
	l, r := m.headerParts()
	header := spread(" "+l, r+" ", w)
	footer := m.footerLine(w)
	if !m.loaded {
		return frame(header, "\n "+m.firstSyncLine(), footer, h)
	}

	var body []string
	// Row 1: now and level.
	leftW := (w - 1) / 3
	rightW := w - 1 - leftW
	count := func(mark, name string, n int) string {
		return mark + padRight(name, 10) + boldSt.Render(fmt.Sprintf("%4d", n))
	}
	now := strings.Join([]string{
		"",
		count(lessonSt.Render("▌"), "Lessons", m.queue.Lessons),
		count(reviewSt.Render("▌"), "Reviews", m.queue.Reviews),
		"",
	}, "\n")
	p := m.progress
	level := strings.Join([]string{
		"",
		progressBar(progressLike{p.Passed, p.Needed, p.Kanji}, rightW-4),
		levelText(p),
		"",
	}, "\n")
	body = append(body, sideBySide(1,
		panel(titleSt.Render("Now"), now, leftW, 6),
		panel(titleSt.Render(fmt.Sprintf("Level %d", p.Level)), level, rightW, 6)))

	if len(m.rejected) > 0 {
		title := warnSt.Render(fmt.Sprintf("Rejected by WaniKani (%d)", len(m.rejected))) + mutedSt.Render(" · not recorded")
		body = append(body, panel(title, strings.Join(m.rejectedLines(w-4), "\n"), w, 0))
	}

	// The forecast takes the rows left over, within reason.
	used := 1 + 1 + strings.Count(strings.Join(body, "\n"), "\n") + 1 + 4
	if m.notice != "" {
		used++
	}
	chartH := min(max(h-used-6, 2), 16)
	title := titleSt.Render("Next 24 hours") + mutedSt.Render(" · "+m.forecastNote())
	body = append(body, panel(title, m.forecastBody(w-4, chartH), w, 0))
	body = append(body, panel(titleSt.Render("SRS stages"), stageColumns(m.srs, w-4), w, 0))
	if m.notice != "" {
		footer = " " + warnSt.Render(m.notice) + "\n" + footer
	}
	return frame(header, strings.Join(body, "\n"), footer, h)
}

// renderRules: no boxes; sections divided by titled full-width rules.
func (m *Dashboard) renderRules() string {
	w, h := max(m.width, 60), max(m.height, 20)
	l, r := m.headerParts()
	header := spread(" "+l, r+" ", w) + "\n" + lineSt.Render(strings.Repeat("─", w))
	footer := lineSt.Render(strings.Repeat("─", w)) + "\n" + m.footerLine(w)
	if !m.loaded {
		return frame(header, "\n   "+m.firstSyncLine(), footer, h)
	}
	const indent = "   "
	inner := w - 2*len(indent)
	var b []string

	// Headline: counts and level progress in three columns.
	col := inner / 4
	p := m.progress
	barW := max(inner-2*col-2, 10)
	b = append(b, "",
		indent+padRight(labelSt.Render("Lessons"), col)+padRight(labelSt.Render("Reviews"), col)+labelSt.Render(fmt.Sprintf("Level %d", p.Level)),
		indent+padRight(lessonSt.Render("▌")+boldSt.Render(fmt.Sprint(m.queue.Lessons)), col)+
			padRight(reviewSt.Render("▌")+boldSt.Render(fmt.Sprint(m.queue.Reviews)), col)+
			progressBar(progressLike{p.Passed, p.Needed, p.Kanji}, barW),
		indent+strings.Repeat(" ", 2*col)+padRight(levelText(p), inner-2*col),
		"")

	if len(m.rejected) > 0 {
		b = append(b, rule(warnSt.Render(fmt.Sprintf("Rejected by WaniKani (%d)", len(m.rejected))), "not recorded", w))
		for _, line := range m.rejectedLines(inner) {
			b = append(b, indent+line)
		}
		b = append(b, "")
	}

	used := 2 + len(b) + 1 + 1 + 2 + 4 + 2
	chartH := min(max(h-used-4, 2), 16)
	b = append(b, rule(titleSt.Render("Next 24 hours"), m.forecastNote(), w), "")
	for _, line := range strings.Split(m.forecastBody(inner, chartH), "\n") {
		b = append(b, indent+line)
	}
	b = append(b, "", rule(titleSt.Render("SRS stages"), "", w), "")
	for _, line := range strings.Split(stageColumns(m.srs, inner), "\n") {
		b = append(b, indent+line)
	}
	if m.notice != "" {
		b = append(b, "", indent+warnSt.Render(m.notice))
	}
	return frame(header, strings.Join(b, "\n"), footer, h)
}
