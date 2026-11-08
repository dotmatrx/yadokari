// Package ui holds yadokari's Bubble Tea screens.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jtharris/yadokari/internal/store"
	"github.com/jtharris/yadokari/internal/wk"
	"github.com/jtharris/yadokari/internal/wksync"
)

// WaniKani type colours, used only as marks beside text, never as text.
var (
	lessonColour = lipgloss.Color("#ff00aa")
	reviewColour = lipgloss.Color("#00aaff")
)

var (
	boldSt   = lipgloss.NewStyle().Bold(true)
	mutedSt  = lipgloss.NewStyle().Faint(true)
	errSt    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4455"))
	barSt    = lipgloss.NewStyle().Foreground(reviewColour)
	lessonSt = lipgloss.NewStyle().Foreground(lessonColour)
	reviewSt = lipgloss.NewStyle().Foreground(reviewColour)
)

// Deps are what the dashboard needs.
type Deps struct {
	Store    *store.Store
	Syncer   *wksync.Syncer
	Now      func() time.Time
	Location *time.Location
	Endpoint string // shown in the header, e.g. "live API" or "mock"
}

// Dashboard is the home screen: counts, forecast and level progress, drawn
// from the local cache and refreshed as syncs complete.
type Dashboard struct {
	deps   Deps
	width  int
	height int

	user     *wk.Resource[wk.User]
	queue    store.Queue
	progress store.LevelProgress
	loaded   bool
	lastSync time.Time

	syncing  bool
	phase    string
	event    wksync.Event
	syncErr  error
	events   chan tea.Msg
	cancel   context.CancelFunc
	tickedAt time.Time
}

// NewDashboard builds the dashboard.
func NewDashboard(d Deps) *Dashboard {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Location == nil {
		d.Location = time.Local
	}
	return &Dashboard{deps: d}
}

type (
	loadedMsg struct {
		user     *wk.Resource[wk.User]
		queue    store.Queue
		progress store.LevelProgress
		lastSync time.Time
		err      error
	}
	syncEventMsg wksync.Event
	syncPhaseMsg struct {
		phase string
		err   error
		done  bool
	}
	tickMsg time.Time
)

func (m *Dashboard) Init() tea.Cmd {
	return tea.Batch(m.load(), m.startSync(), tick())
}

func tick() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// load reads everything the view shows from the store.
func (m *Dashboard) load() tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		ctx := context.Background()
		var msg loadedMsg
		msg.user, msg.err = deps.Store.User(ctx)
		if msg.err != nil || msg.user == nil {
			return msg
		}
		u := msg.user.Data
		msg.queue, msg.err = deps.Store.Queue(ctx, deps.Now(), u.Subscription.MaxLevelGranted)
		if msg.err != nil {
			return msg
		}
		msg.progress, msg.err = deps.Store.LevelProgress(ctx, u.Level)
		st, _ := deps.Store.SyncState(ctx, store.Assignments)
		msg.lastSync = st.SyncedAt
		return msg
	}
}

// startSync runs a foreground then background sync in a goroutine, feeding
// progress back through a channel the update loop listens on.
func (m *Dashboard) startSync() tea.Cmd {
	if m.syncing {
		return nil
	}
	m.syncing, m.syncErr, m.phase = true, nil, "connecting"
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	ch := make(chan tea.Msg, 64)
	m.events = ch
	syncer := *m.deps.Syncer
	syncer.Progress = func(e wksync.Event) {
		select {
		case ch <- syncEventMsg(e):
		default: // the UI is behind; drop intermediate progress
		}
	}
	go func() {
		defer close(ch)
		if _, err := syncer.Run(ctx, wksync.Foreground); err != nil {
			ch <- syncPhaseMsg{phase: "foreground", err: err, done: true}
			return
		}
		ch <- syncPhaseMsg{phase: "foreground"}
		_, err := syncer.Collections(ctx, wksync.Background)
		ch <- syncPhaseMsg{phase: "background", err: err, done: true}
	}()
	return listen(ch)
}

func listen(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "r":
			return m, m.startSync()
		}
	case loadedMsg:
		if msg.err != nil {
			m.syncErr = msg.err
			return m, nil
		}
		if msg.user != nil {
			m.user, m.queue, m.progress, m.lastSync, m.loaded = msg.user, msg.queue, msg.progress, msg.lastSync, true
		}
	case syncEventMsg:
		m.event = wksync.Event(msg)
		m.phase = msg.Collection
		return m, listen(m.events)
	case syncPhaseMsg:
		if msg.err != nil {
			m.syncErr = msg.err
		}
		if msg.done {
			m.syncing = false
		}
		// Redraw from the store after each phase; keep listening until closed.
		return m, tea.Batch(m.load(), listen(m.events))
	case tickMsg:
		// Reviews become due on the hour; recompute from the store.
		return m, tea.Batch(m.load(), tick())
	}
	return m, nil
}

func (m *Dashboard) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "yadokari"
	return v
}

func (m *Dashboard) render() string {
	width := max(m.width, 40)
	var b strings.Builder

	// Header.
	left := boldSt.Render("yadokari")
	if m.user != nil {
		left += mutedSt.Render(fmt.Sprintf(" · %s · level %d", m.user.Data.Username, m.user.Data.Level))
	}
	right := mutedSt.Render(m.deps.Endpoint)
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right)-2, 1)
	b.WriteString(" " + left + strings.Repeat(" ", gap) + right + "\n\n")

	if !m.loaded {
		b.WriteString(" " + m.firstSyncLine() + "\n")
		b.WriteString("\n " + m.statusLine())
		return b.String()
	}

	// Headline counts: neutral ink with a coloured mark beside each.
	b.WriteString(" " + lessonSt.Render("▌") + "Lessons  " + boldSt.Render(fmt.Sprint(m.queue.Lessons)) +
		"      " + reviewSt.Render("▌") + "Reviews  " + boldSt.Render(fmt.Sprint(m.queue.Reviews)) + "\n\n")

	forecast := renderForecast(m.queue.Forecast, m.queue.Start, m.queue.Next, width, m.deps.Location, barSt, mutedSt)
	for _, line := range strings.Split(forecast, "\n") {
		b.WriteString(" " + line + "\n")
	}
	b.WriteString("\n " + m.levelLine(width) + "\n\n")
	b.WriteString(" " + m.statusLine())
	return b.String()
}

func (m *Dashboard) firstSyncLine() string {
	if m.syncErr != nil {
		return errSt.Render("First sync failed: " + m.syncErr.Error())
	}
	e := m.event
	if e.Total > 0 {
		return fmt.Sprintf("First sync: %s %s / %s", e.Collection, comma(e.Fetched), comma(e.Total))
	}
	return "First sync: " + m.phase + "…"
}

func (m *Dashboard) levelLine(width int) string {
	p := m.progress
	if p.Kanji == 0 {
		return mutedSt.Render(fmt.Sprintf("Level %d: no kanji", p.Level))
	}
	barW := min(30, max(width-50, 10))
	filled := p.Passed * barW / p.Kanji
	mark := min(p.Needed*barW/p.Kanji, barW-1) // where level-up happens
	var bar strings.Builder
	for i := range barW {
		switch {
		case i < filled:
			bar.WriteString(lessonSt.Render("━"))
		case i == mark:
			bar.WriteString("┃")
		default:
			bar.WriteString(mutedSt.Render("━"))
		}
	}
	text := fmt.Sprintf("%d / %d kanji passed", p.Passed, p.Kanji)
	if p.Passed < p.Needed {
		text += fmt.Sprintf(" · %d more to level up", p.Needed-p.Passed)
	}
	return fmt.Sprintf("Level %d  %s  %s", p.Level, bar.String(), text)
}

func (m *Dashboard) statusLine() string {
	var s string
	switch {
	case m.syncErr != nil && m.loaded:
		s = errSt.Render("sync failed: "+shortErr(m.syncErr)) + mutedSt.Render(" · showing data from "+ago(m.deps.Now(), m.lastSync))
	case m.syncing && m.loaded:
		s = mutedSt.Render("syncing " + m.phase + "…")
	case m.loaded:
		s = mutedSt.Render("synced " + ago(m.deps.Now(), m.lastSync))
	}
	help := mutedSt.Render("r sync · q quit")
	gap := max(max(m.width, 40)-lipgloss.Width(s)-lipgloss.Width(help)-2, 1)
	return s + strings.Repeat(" ", gap) + help
}

func shortErr(err error) string {
	s := err.Error()
	if len(s) > 60 {
		s = s[:57] + "…"
	}
	return s
}

func ago(now, t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}

func comma(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
