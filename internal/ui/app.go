package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

type screen int

const (
	dashScreen screen = iota
	reviewScreen
	summaryScreen
	lessonScreen
)

type reviewLoadedMsg struct {
	review *Review
	err    error
}

// App is the root model: it owns the screens and switches between them.
type App struct {
	deps    Deps
	rdeps   ReviewDeps
	screen  screen
	dash    *Dashboard
	review  *Review
	lesson  *Lesson
	summary Summary
	width   int
	height  int
	shared  shared // graphics ready, audio state; screens read it via rdeps
}

// NewApp builds the app, starting on the dashboard.
func NewApp(d Deps, rd ReviewDeps) *App {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Location == nil {
		d.Location = time.Local
	}
	a := &App{deps: d, rdeps: rd, dash: NewDashboard(d)}
	a.rdeps.shared = &a.shared
	return a
}

// graphicsReadyMsg says the alt screen is up: kitty uploads sent before it
// could be lost, so glyphs draw as blank blocks until then.
type graphicsReadyMsg struct{}

func (a *App) Init() tea.Cmd {
	return tea.Batch(a.dash.Init(),
		tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return graphicsReadyMsg{} }))
}

func (a *App) quit() tea.Cmd {
	a.dash.Stop()
	if a.rdeps.Renderer != nil {
		if s := a.rdeps.Renderer.Reset(); s != "" {
			return tea.Sequence(tea.Raw(s), tea.Quit)
		}
	}
	return tea.Quit
}

// Update handles a message, then draws the visible screen's glyph and
// sends any kitty upload that queued. This is the only place uploads are
// sent: a screen just describes its glyph (DrawGlyph), so no command a
// screen returns, or a caller drops, can strand an upload and leave the
// glyph blank.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := a.update(msg)
	return m, tea.Batch(cmd, a.graphics())
}

// graphics draws the visible screen's glyph (queuing any upload) and,
// once the alt screen is up, sends the queue.
func (a *App) graphics() tea.Cmd {
	rd := a.rdeps.Renderer
	if !a.shared.graphicsReady || rd == nil {
		return nil
	}
	switch {
	case a.screen == reviewScreen && a.review != nil:
		a.review.DrawGlyph()
	case a.screen == lessonScreen && a.lesson != nil:
		a.lesson.DrawGlyph()
	}
	if s := rd.Flush(); s != "" {
		return tea.Raw(s)
	}
	return nil
}

func (a *App) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case graphicsReadyMsg:
		a.shared.graphicsReady = true
		return a, nil

	case playAudioMsg:
		return a, a.play(msg.subject)
	case audioStartedMsg:
		return a, a.audioStarted(msg)
	case audioDoneMsg:
		a.audioDone(msg)
		return a, nil

	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.dash.Update(msg)
		var cmds []tea.Cmd
		if a.lesson != nil {
			_, cmd := a.lesson.Update(msg)
			cmds = append(cmds, cmd)
		}
		if a.review != nil {
			_, cmd := a.review.Update(msg)
			cmds = append(cmds, cmd)
		}
		return a, tea.Batch(cmds...)

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" || (a.screen == dashScreen && (msg.String() == "q" || msg.String() == "esc")) {
			return a, a.quit()
		}

	case StartReviewsMsg:
		if a.rdeps.Disabled != "" {
			a.dash.Notice(a.rdeps.Disabled)
			return a, nil
		}
		deps, rdeps, user := a.deps, a.rdeps, msg.User
		return a, func() tea.Msg {
			r, err := LoadReview(context.Background(), deps, rdeps, user)
			return reviewLoadedMsg{review: r, err: err}
		}

	case StartLessonsMsg:
		if a.rdeps.Disabled != "" {
			a.dash.Notice(a.rdeps.Disabled)
			return a, nil
		}
		deps, rdeps, user := a.deps, a.rdeps, msg.User
		return a, func() tea.Msg {
			l, q, err := LoadLessons(context.Background(), deps, rdeps, user)
			return lessonLoadedMsg{lesson: l, quiz: q, err: err}
		}

	case lessonLoadedMsg:
		switch {
		case msg.err != nil:
			a.dash.Notice(msg.err.Error())
			a.screen, a.lesson = dashScreen, nil
			return a, a.dash.Reload()
		case msg.quiz != nil: // resuming a quiz
			return a, a.startReview(msg.quiz)
		}
		a.lesson, a.screen = msg.lesson, lessonScreen
		_, cmd := a.lesson.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		return a, tea.Batch(cmd, a.lesson.Init())

	case lessonQuizMsg:
		a.lesson = nil
		return a, a.startReview(msg.quiz)

	case lessonImageMsg:
		var cmd tea.Cmd
		if a.lesson != nil {
			_, cmd = a.lesson.Update(msg)
		}
		return a, cmd

	case reviewLoadedMsg:
		if msg.err != nil {
			a.dash.Notice(msg.err.Error())
			return a, nil
		}
		return a, a.startReview(msg.review)

	case BackMsg:
		a.screen, a.lesson = dashScreen, nil
		return a, a.dash.Reload()

	case ReviewDoneMsg:
		a.summary, a.screen = msg.Summary, summaryScreen
		return a, nil

	// Background work keeps flowing to its owner whatever is on screen.
	case loadedMsg, syncEventMsg, syncPhaseMsg, tickMsg, uploadedMsg:
		_, cmd := a.dash.Update(msg)
		return a, cmd
	case imageMsg, uploadMsg:
		var cmd tea.Cmd
		if a.review != nil {
			_, cmd = a.review.Update(msg)
		}
		if _, ok := msg.(uploadMsg); ok && a.screen == summaryScreen {
			// Uploads finishing after the session fill in SRS movement.
			_ = a.summary.Refresh(context.Background(), a.deps.Store)
			a.summary.Held = a.review != nil && a.review.held
		}
		return a, cmd
	}

	switch a.screen {
	case lessonScreen:
		_, cmd := a.lesson.Update(msg)
		return a, cmd
	case reviewScreen:
		_, cmd := a.review.Update(msg)
		return a, cmd
	case summaryScreen:
		if k, ok := msg.(tea.KeyPressMsg); ok && isEnter(k) {
			a.screen = dashScreen
			return a, a.dash.Reload()
		}
		return a, nil
	}
	_, cmd := a.dash.Update(msg)
	return a, cmd
}

// startReview shows a review session (or lesson quiz) at the current size.
func (a *App) startReview(r *Review) tea.Cmd {
	a.review, a.screen = r, reviewScreen
	_, cmd := a.review.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
	return tea.Batch(cmd, a.review.Init())
}

func (a *App) View() tea.View {
	var v tea.View
	switch a.screen {
	case lessonScreen:
		v = tea.NewView(a.lesson.View())
	case reviewScreen:
		v = tea.NewView(a.review.View())
		v.Cursor = a.review.Cursor()
	case summaryScreen:
		v = tea.NewView(a.summary.View(a.width, a.height))
	default:
		return a.dash.View()
	}
	v.AltScreen = true
	v.WindowTitle = "yadokari"
	return v
}
