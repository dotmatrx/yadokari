package ui

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jtharris/yadokari/internal/store"
	"github.com/jtharris/yadokari/internal/wk"
	"github.com/jtharris/yadokari/internal/wkmock"
)

var now = time.Date(2026, 11, 7, 12, 30, 0, 0, time.UTC)

func plain() lipgloss.Style { return lipgloss.NewStyle() }

func TestForecastEmpty(t *testing.T) {
	next := time.Date(2026, 11, 9, 13, 0, 0, 0, time.UTC)
	got := renderForecast([24]int{}, now.Truncate(time.Hour), &next, 100, time.UTC, plain(), plain())
	if got != "Reviews in the next 24 hours: none · next Mon 9 Nov 13:00" {
		t.Errorf("got %q", got)
	}
}

func TestForecastBars(t *testing.T) {
	var counts [24]int
	counts[0], counts[3], counts[23] = 1, 40, 8
	got := renderForecast(counts, now.Truncate(time.Hour), nil, 100, time.UTC, plain(), plain())
	lines := strings.Split(got, "\n")
	if lines[0] != "Reviews in the next 24 hours: 49" { // next is nil here
		t.Errorf("title %q", lines[0])
	}
	if strings.TrimSpace(lines[1]) != "40" || strings.Index(lines[1], "40") != 3*3 {
		t.Errorf("peak label should sit over hour 3: %q", lines[1])
	}
	// Five bar rows: the peak is full height, hour 0 is a sliver, hour 23 ~1/5.
	bars := lines[2:7]
	if !strings.HasPrefix(bars[0][9:], "██") {
		t.Errorf("peak top row %q", bars[0])
	}
	if !strings.HasPrefix(bars[4], "▁▁") {
		t.Errorf("small non-zero hour should show a sliver: %q", bars[4])
	}
	for _, l := range bars {
		if w := ansi.StringWidth(l); w != 24*3-1 {
			t.Errorf("bar row width %d: %q", w, l)
		}
	}
	if !strings.HasPrefix(lines[8], "12:00") || !strings.Contains(lines[8], "18:00") {
		t.Errorf("axis labels %q", lines[8])
	}

	narrow := renderForecast(counts, now.Truncate(time.Hour), nil, 60, time.UTC, plain(), plain())
	if w := ansi.StringWidth(strings.Split(narrow, "\n")[2]); w != 24*2-1 {
		t.Errorf("narrow bars width %d", w)
	}
}

func toRaw[T any](rs []wk.Resource[T]) []store.RawResource {
	out := make([]store.RawResource, len(rs))
	for i, r := range rs {
		data, _ := json.Marshal(r.Data)
		out[i] = store.RawResource{ID: r.ID, Object: r.Object, DataUpdatedAt: r.DataUpdatedAt, Data: data}
	}
	return out
}

func TestDashboardRender(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	d := wkmock.SampleData(now)
	st.Upsert(ctx, store.Subjects, toRaw(d.Subjects))
	st.Upsert(ctx, store.Assignments, toRaw(d.Assignments))
	userRaw, _ := json.Marshal(d.User)
	st.PutSingleton(ctx, "user", store.Singleton{Raw: userRaw, FetchedAt: now})
	st.SetSyncState(ctx, store.Assignments, store.SyncState{SyncedAt: now.Add(-5 * time.Minute)})

	m := NewDashboard(Deps{Store: st, Now: func() time.Time { return now }, Location: time.UTC, Endpoint: "mock"})
	m.Update(windowSize(100, 30))

	before := ansi.Strip(m.render())
	if !strings.Contains(before, "First sync") {
		t.Errorf("before loading:\n%s", before)
	}

	m.Update(m.load()())
	out := ansi.Strip(m.render())
	for _, want := range []string{
		"yadokari · mock_learner · level 1", "mock",
		"Lessons  2", "Reviews  4",
		"Reviews in the next 24 hours: 1 · next 15:00",
		"Level 1", "1 / 2 kanji passed · 1 more to level up",
		"synced 5 min ago", "r sync · q quit",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for i, line := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(line); w > 100 {
			t.Errorf("line %d is %d wide (> 100): %q", i, w, line)
		}
	}
}

func TestComma(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 9452: "9,452", 1234567: "1,234,567"} {
		if got := comma(n); got != want {
			t.Errorf("comma(%d) = %q", n, got)
		}
	}
}

func windowSize(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }
