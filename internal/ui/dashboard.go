// Package ui holds yadokari's Bubble Tea screens.
package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/dotmatrx/yadokari/internal/store"
	"github.com/dotmatrx/yadokari/internal/wk"
	"github.com/dotmatrx/yadokari/internal/wksync"
)

// Deps are what the dashboard needs.
type Deps struct {
	Store    *store.Store
	Syncer   *wksync.Syncer
	Uploader *wksync.Uploader // nil: queued reviews are not sent from here
	Now      func() time.Time
	Location *time.Location
	Endpoint string // shown in the header, e.g. "live API" or "mock"
	Version  string // shown after the name in the header, e.g. "v0.3.0"
	// Seed, when not 0, fixes the review order and questions (previews
	// that must come out the same every time); 0 seeds from the clock.
	Seed uint64
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
	srs      [10]int
	loaded   bool
	lastSync time.Time
	pending  int        // reviews waiting to upload
	rejected []rejected // reviews the API refused

	notice      string
	syncing     bool
	phase       string
	event       wksync.Event
	syncErr     error
	syncProblem string // syncErr, for the user
	upProblem   string // the last upload attempt's failure (not "held"), for the user
	events      chan tea.Msg
	cancel      context.CancelFunc
	tickedAt    time.Time
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
		srs      [10]int
		lastSync time.Time
		pending  int
		rejected []rejected
		err      error
	}
	syncEventMsg wksync.Event
	syncPhaseMsg struct {
		phase string
		err   error
		done  bool
	}
	tickMsg time.Time
	// uploadedMsg reports the dashboard's flush of queued reviews.
	uploadedMsg struct {
		res wksync.UploadResult
		err error
	}
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
		if msg.err == nil {
			msg.srs, msg.err = deps.Store.SRSCounts(ctx, u.Subscription.MaxLevelGranted)
		}
		st, _ := deps.Store.SyncState(ctx, store.Assignments)
		msg.lastSync = st.SyncedAt
		if msg.err == nil {
			msg.pending, msg.rejected, msg.err = loadOutbox(ctx, deps.Store)
		}
		return msg
	}
}

// startSync sends queued reviews, then runs a foreground then background
// sync, in a goroutine feeding progress back through a channel the update
// loop listens on. Uploading first means the sync sees their effect, and
// reviews done offline go out as soon as the app starts with a network.
func (m *Dashboard) startSync() tea.Cmd {
	if m.syncing {
		return nil
	}
	m.syncing, m.syncErr, m.phase = true, nil, "connecting"
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	ch := make(chan tea.Msg, 64)
	m.events = ch
	syncer, up := *m.deps.Syncer, m.deps.Uploader
	syncer.Progress = func(e wksync.Event) {
		select {
		case ch <- syncEventMsg(e):
		default: // the UI is behind; drop intermediate progress
		}
	}
	go func() {
		defer close(ch)
		if up != nil {
			res, err := up.Flush(ctx)
			ch <- uploadedMsg{res: res, err: err}
		}
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
		m.notice = ""
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.Stop()
			return m, tea.Quit
		case "s":
			return m, m.startSync()
		case "x":
			if len(m.rejected) > 0 {
				return m, m.dismissRejected()
			}
		case "r":
			if m.loaded {
				return m, func() tea.Msg { return StartReviewsMsg{User: m.user.Data} }
			}
		case "l":
			if m.loaded {
				return m, func() tea.Msg { return StartLessonsMsg{User: m.user.Data} }
			}
		}
	case loadedMsg:
		if msg.err != nil {
			m.syncErr, m.syncProblem = msg.err, problem("loading the dashboard", msg.err)
			return m, nil
		}
		if msg.user != nil {
			m.user, m.queue, m.progress, m.lastSync, m.loaded = msg.user, msg.queue, msg.progress, msg.lastSync, true
			m.pending, m.rejected, m.srs = msg.pending, msg.rejected, msg.srs
		}
	case syncEventMsg:
		m.event = wksync.Event(msg)
		m.phase = msg.Collection
		return m, listen(m.events)
	case uploadedMsg:
		m.upProblem = ""
		if msg.err != nil && !errors.Is(msg.err, wksync.ErrHeld) {
			m.upProblem = problem("upload", msg.err)
		}
		return m, tea.Batch(m.load(), listen(m.events))
	case syncPhaseMsg:
		if msg.err != nil {
			m.syncErr, m.syncProblem = msg.err, problem("sync", msg.err)
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

// StartReviewsMsg asks the app to open the review screen.
type StartReviewsMsg struct{ User wk.User }

// Stop cancels a running sync.
func (m *Dashboard) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// Reload re-reads the counts from the store (e.g. after a review session).
func (m *Dashboard) Reload() tea.Cmd { return m.load() }

// Notice shows a one-line message until the next key press.
func (m *Dashboard) Notice(s string) { m.notice = s }

func (m *Dashboard) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "yadokari"
	return v
}

func (m *Dashboard) firstSyncLine() string {
	if m.syncErr != nil {
		return errSt.Render("First sync failed: " + m.syncProblem)
	}
	e := m.event
	if e.Total > 0 {
		return fmt.Sprintf("First sync: %s %s / %s", e.Collection, comma(e.Fetched), comma(e.Total))
	}
	return "First sync: " + m.phase + "…"
}

func (m *Dashboard) statusLine() string {
	var s string
	switch {
	case m.syncErr != nil && m.loaded:
		s = errSt.Render("sync failed: "+m.syncProblem) + mutedSt.Render(" · showing data from "+ago(m.deps.Now(), m.lastSync))
	case m.syncing && m.loaded:
		s = mutedSt.Render("syncing " + m.phase + "…")
	case m.loaded:
		s = mutedSt.Render("synced " + ago(m.deps.Now(), m.lastSync))
	}
	if m.pending > 0 {
		s += mutedSt.Render(fmt.Sprintf(" · %d to upload", m.pending))
		if m.upProblem != "" && m.upProblem != m.syncProblem { // say it once
			s += errSt.Render(" (" + m.upProblem + ")")
		}
	}
	keys := "r reviews · l lessons · s sync · q quit"
	if len(m.rejected) > 0 {
		keys = "r reviews · l lessons · s sync · x dismiss rejected · q quit"
	}
	help := mutedSt.Render(keys)
	gap := max(max(m.width, 40)-lipgloss.Width(s)-lipgloss.Width(help)-2, 1)
	return s + strings.Repeat(" ", gap) + help
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

// rejected is a review the API refused, for the dashboard.
type rejected struct {
	item   SummaryItem
	reason string
}

func loadOutbox(ctx context.Context, st *store.Store) (int, []rejected, error) {
	pending, _, err := st.OutboxCounts(ctx)
	if err != nil {
		return 0, nil, err
	}
	rows, err := st.Rejected(ctx)
	if err != nil {
		return 0, nil, err
	}
	var out []rejected
	for _, r := range rows {
		line := SummaryItem{Characters: fmt.Sprintf("assignment %d", r.AssignmentID)}
		if subj, err := st.Subject(ctx, r.SubjectID); err == nil && subj != nil {
			line = SummaryItem{Characters: characters(*subj), Meaning: primaryMeaning(subj.Data), Type: subj.Object}
		}
		out = append(out, rejected{item: line, reason: r.Failed})
	}
	return pending, out, nil
}

func (m *Dashboard) dismissRejected() tea.Cmd {
	st, load := m.deps.Store, m.load()
	return func() tea.Msg {
		if _, err := st.DismissRejected(context.Background()); err != nil {
			return loadedMsg{err: err}
		}
		return load()
	}
}

// maxRejectedShown caps the rejected list; the rest are summarized.
const maxRejectedShown = 5
