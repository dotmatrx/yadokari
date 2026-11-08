package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// eighths are the partial-block characters for sub-row bar heights.
var eighths = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

// renderForecast draws the 24-hour review forecast as one series of vertical
// bars: one per hour, a one-cell gap between bars, muted hour labels every
// six hours, and only the peak value labelled. When nothing is due it
// returns a sentence instead of an empty chart.
func renderForecast(counts [24]int, start time.Time, next *time.Time, width int, loc *time.Location, bar lipgloss.Style, muted lipgloss.Style) string {
	total, peak, peakAt := 0, 0, 0
	for i, n := range counts {
		total += n
		if n > peak {
			peak, peakAt = n, i
		}
	}
	title := "Reviews in the next 24 hours"
	if total == 0 {
		s := title + ": none"
		if next != nil {
			s += " · next " + next.In(loc).Format("Mon 2 Jan 15:04")
		}
		return s
	}

	barW := 2
	if width < 24*3+6 {
		barW = 1
	}
	step := barW + 1
	const height = 5 // rows; each row is 8 eighths

	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d", title, total)
	if next != nil {
		b.WriteString(muted.Render(" · next " + next.In(loc).Format("15:04")))
	}
	b.WriteByte('\n')

	// Peak label, centred over its bar.
	label := fmt.Sprint(peak)
	pos := peakAt*step + barW/2 - len(label)/2
	b.WriteString(strings.Repeat(" ", max(pos, 0)) + label + "\n")

	for row := height - 1; row >= 0; row-- {
		var line strings.Builder
		for i, n := range counts {
			units := 0
			if n > 0 {
				// Scale to eighths; any non-zero hour shows at least a sliver.
				units = max(1, (n*height*8+peak-1)/peak)
			}
			fill := min(max(units-row*8, 0), 8)
			line.WriteString(bar.Render(strings.Repeat(eighths[fill], barW)))
			if i < len(counts)-1 {
				line.WriteByte(' ')
			}
		}
		b.WriteString(line.String() + "\n")
	}

	// Baseline and hour labels at 0, 6, 12, 18 hours.
	axisW := 24*step - 1
	b.WriteString(muted.Render(strings.Repeat("─", axisW)) + "\n")
	labels := []rune(strings.Repeat(" ", axisW+6))
	for _, h := range []int{0, 6, 12, 18} {
		text := []rune(start.Add(time.Duration(h) * time.Hour).In(loc).Format("15:04"))
		copy(labels[h*step:], text)
	}
	b.WriteString(muted.Render(strings.TrimRight(string(labels), " ")))
	return b.String()
}
