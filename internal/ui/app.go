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
}

// NewApp builds the app, starting on the dashboard.
func NewApp(d Deps, rd ReviewDeps) *App {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Location == nil {
		d.Location = time.Local
	}
	return &App{deps: d, rdeps: rd, dash: NewDashboard(d)}
}

func (a *App) Init() tea.Cmd { return a.dash.Init() }

func (a *App) quit() tea.Cmd {
	a.dash.Stop()
	if a.rdeps.Renderer != nil {
		if s := a.rdeps.Renderer.Reset(); s != "" {
			return tea.Sequence(tea.Raw(s), tea.Quit)
		}
	}
	return tea.Quit
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.dash.Update(msg)
		if a.lesson != nil {
			a.lesson.Update(msg)
		}
		if a.review != nil {
			// The review's command carries the kitty upload for the glyph at
			// its new size; the renderer already counts it as sent.
			_, cmd := a.review.Update(msg)
			return a, cmd
		}
		return a, nil

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
		a.lesson.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		return a, a.lesson.Init()

	case lessonQuizMsg:
		a.lesson = nil
		msg.quiz.ready = true // the alt screen is already up
		return a, a.startReview(msg.quiz)

	case lessonReadyMsg, lessonImageMsg:
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
	case imageMsg, uploadMsg, reviewReadyMsg:
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
	a.review.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
	return a.review.Init()
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
