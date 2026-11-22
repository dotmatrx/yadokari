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

	"github.com/dotmatrx/yadokari/internal/gfx"
	"github.com/dotmatrx/yadokari/internal/store"
	"github.com/dotmatrx/yadokari/internal/wk"
	"github.com/dotmatrx/yadokari/internal/wkmock"
)

var now = time.Date(2026, 11, 7, 12, 30, 0, 0, time.UTC)

func plain() lipgloss.Style { return lipgloss.NewStyle() }

func TestForecastChart(t *testing.T) {
	var counts [24]int
	counts[0], counts[3], counts[23] = 1, 40, 8
	got := forecastChart(counts, now.Truncate(time.Hour), time.UTC, 100, 5)
	lines := strings.Split(ansi.Strip(got), "\n")
	if len(lines) != 1+5+2 {
		t.Fatalf("%d lines:\n%s", len(lines), got)
	}
	// 100 cells: 3-wide bars, 95 cells in all, centred with 2 cells of padding.
	pad := 2
	if strings.TrimSpace(lines[0]) != "40" || strings.Index(lines[0], "40") != pad+3*4+1-1 {
		t.Errorf("peak label should sit over hour 3: %q", lines[0])
	}
	bars := lines[1:6]
	if !strings.HasPrefix(bars[0][pad+12:], "███") {
		t.Errorf("peak top row %q", bars[0])
	}
	if !strings.HasPrefix(bars[4][pad:], "▁▁▁") {
		t.Errorf("small non-zero hour should show a sliver: %q", bars[4])
	}
	for _, l := range bars {
		if w := ansi.StringWidth(l); w != pad+24*4-1 {
			t.Errorf("bar row width %d: %q", w, l)
		}
	}
	if !strings.HasPrefix(lines[7][pad:], "12:00") || !strings.Contains(lines[7], "18:00") {
		t.Errorf("axis labels %q", lines[7])
	}
	// Narrow: 1-wide bars.
	narrow := strings.Split(ansi.Strip(forecastChart(counts, now, time.UTC, 50, 3)), "\n")
	if w := ansi.StringWidth(narrow[1]); w > 50 || !strings.Contains(narrow[1], "█ ") {
		t.Errorf("narrow bars %q", narrow[1])
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

	m := NewDashboard(Deps{Store: st, Now: func() time.Time { return now }, Location: time.UTC, Endpoint: "mock", Version: "v0.3.0"})
	m.Update(windowSize(100, 30))

	before := ansi.Strip(m.render())
	if !strings.Contains(before, "First sync") {
		t.Errorf("before loading:\n%s", before)
	}

	m.Update(m.load()())
	out := ansi.Strip(m.render())
	for _, want := range []string{
		"yadokari v0.3.0", "mock_learner · level 1 · mock",
		"Lessons      2", "Reviews      4",
		"Next 24 hours · 1 reviews · next 15:00",
		"Level 1", "1 / 2 kanji passed · 1 more to level up",
		"Apprentice", "Guru",
		"synced 5 min ago", "r reviews · l lessons · s sync · q quit",
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

func TestScreensFillTheTerminal(t *testing.T) {
	ctx := context.Background()
	d := wkmock.DemoData(now)
	for _, screen := range PreviewScreens {
		for _, sz := range [][2]int{{120, 36}, {80, 24}, {60, 20}} {
			// A fresh store each time: review previews queue reviews.
			st, err := store.Open(ctx, ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			st.Upsert(ctx, store.Subjects, toRaw(d.Subjects))
			st.Upsert(ctx, store.Assignments, toRaw(d.Assignments))
			userRaw, _ := json.Marshal(d.User)
			st.PutSingleton(ctx, "user", store.Singleton{Raw: userRaw, FetchedAt: now})
			out, err := Preview(ctx, Deps{Store: st, Now: func() time.Time { return now }, Location: time.UTC},
				ReviewDeps{Renderer: gfx.New(gfx.Plain, 0, 0)}, screen, sz[0], sz[1])
			st.Close()
			if err != nil {
				t.Fatalf("%s %v: %v", screen, sz, err)
			}
			lines := strings.Split(ansi.Strip(out), "\n")
			if len(lines) != sz[1] {
				t.Errorf("%s %v: %d lines", screen, sz, len(lines))
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w > sz[0] {
					t.Errorf("%s %v: line %d is %d wide: %q", screen, sz, i, w, l)
				}
			}
			if strings.TrimSpace(lines[0]) == "" || strings.TrimSpace(lines[len(lines)-1]) == "" {
				t.Errorf("%s %v: header and footer should be on the first and last rows", screen, sz)
			}
		}
	}
}
