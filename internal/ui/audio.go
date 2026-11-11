package ui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jtharris/yadokari/internal/audio"
	"github.com/jtharris/yadokari/internal/wk"
)

// AudioPlayer plays a local audio file (audio.Player).
type AudioPlayer interface {
	Play(ctx context.Context, path string) error
	Formats() []string // content types, preferred first
}

// Downloads fetches a URL to a cached local file (glyph.ImageCache).
type Downloads interface {
	File(ctx context.Context, url string) (string, error)
}

type (
	// playAudioMsg asks the app to play a subject's pronunciation. Screens
	// send it; only App plays, as only App sends glyph uploads.
	playAudioMsg struct{ subject wk.Resource[wk.Subject] }
	audioDoneMsg struct{ err error }
)

func playAudio(s wk.Resource[wk.Subject]) tea.Cmd {
	return func() tea.Msg { return playAudioMsg{subject: s} }
}

// hasAudio reports whether a subject has any pronunciation recording.
func hasAudio(s wk.Resource[wk.Subject]) bool { return len(s.Data.PronunciationAudios) > 0 }

// errNoRecording is shown when p is pressed on a subject without audio.
var errNoRecording = errors.New("no pronunciation audio for this item")

// play fetches and plays the subject's recording in the background: the
// user's default voice, the primary reading, a format the player handles.
func (a *App) play(s wk.Resource[wk.Subject]) tea.Cmd {
	done := func(err error) tea.Cmd { return func() tea.Msg { return audioDoneMsg{err: err} } }
	player, files := a.rdeps.Audio, a.rdeps.Sounds
	if player == nil || files == nil {
		err := a.rdeps.AudioErr
		if err == nil {
			err = audio.ErrNoPlayer
		}
		return done(err)
	}
	voice := 0
	if a.dash.user != nil {
		voice = a.dash.user.Data.Preferences.DefaultVoiceActorID
	}
	reading := primaryReading(s.Data)
	if reading == "" && s.Data.Characters != nil {
		reading = *s.Data.Characters // kana vocabulary is its own reading
	}
	rec, ok := audio.Pick(s.Data.PronunciationAudios, reading, voice, player.Formats())
	if !ok {
		return done(errNoRecording)
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		path, err := files.File(ctx, rec.URL)
		if err == nil {
			err = player.Play(ctx, path)
		}
		return audioDoneMsg{err: err}
	}
}

func primaryReading(s wk.Subject) string {
	for _, r := range s.Readings {
		if r.Primary {
			return r.Reading
		}
	}
	return ""
}

// audioProblem shows a playback error on the visible screen.
func (a *App) audioProblem(err error) {
	switch {
	case a.screen == reviewScreen && a.review != nil:
		if a.review.item != nil {
			a.review.item.Status = err.Error()
		} else {
			a.review.warn = err.Error()
		}
	case a.screen == lessonScreen && a.lesson != nil:
		a.lesson.page.Status = err.Error()
	}
}
