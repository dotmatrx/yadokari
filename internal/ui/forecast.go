package ui

import (
	"fmt"
	"strings"
	"time"
)

// eighths are the partial-block characters for sub-row bar heights.
var eighths = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

// forecastChart draws the 24-hour forecast within width: bars as wide as
// fit (1–4 cells, one-cell gaps), centred, height rows tall, the peak
// labelled, a baseline and hour labels every six hours.
func forecastChart(counts [24]int, start time.Time, loc *time.Location, width, height int) string {
	peak, peakAt := 0, 0
	for i, n := range counts {
		if n > peak {
			peak, peakAt = n, i
		}
	}
	barW := min(max((width+1)/24-1, 1), 4)
	step := barW + 1
	axisW := 24*step - 1
	pad := strings.Repeat(" ", max((width-axisW)/2, 0))
	var b strings.Builder
	label := fmt.Sprint(peak)
	pos := peakAt*step + barW/2 - len(label)/2
	b.WriteString(pad + strings.Repeat(" ", max(pos, 0)) + label + "\n")
	for row := height - 1; row >= 0; row-- {
		b.WriteString(pad)
		for i, n := range counts {
			units := 0
			if n > 0 && peak > 0 {
				units = max(1, (n*height*8+peak-1)/peak)
			}
			fill := min(max(units-row*8, 0), 8)
			b.WriteString(barSt.Render(strings.Repeat(eighths[fill], barW)))
			if i < 23 {
				b.WriteByte(' ')
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString(pad + lineSt.Render(strings.Repeat("─", axisW)) + "\n")
	labels := []rune(strings.Repeat(" ", axisW+6))
	for h := 0; h < 24; h += 6 {
		copy(labels[h*step:], []rune(start.Add(time.Duration(h)*time.Hour).In(loc).Format("15:04")))
	}
	b.WriteString(pad + labelSt.Render(strings.TrimRight(string(labels), " ")))
	return b.String()
}
