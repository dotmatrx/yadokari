package ui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jtharris/yadokari/internal/audio"
	"github.com/jtharris/yadokari/internal/wk"
)

// AudioPlayer plays a local audio file (audio.Player): Start returns once
// the player is running, wait when it has finished.
type AudioPlayer interface {
	Start(ctx context.Context, path string) (wait func() error, err error)
	Formats() []string // content types, preferred first
}

// Downloads fetches a URL to a cached local file (glyph.ImageCache).
type Downloads interface {
	File(ctx context.Context, url string) (string, error)
}

type (
	// playAudioMsg asks the app to play a subject's pronunciation. Screens
	// send it; only App plays, as only App sends glyph uploads.
	playAudioMsg    struct{ subject wk.Resource[wk.Subject] }
	audioStartedMsg struct {
		gen  int
		wait func() error
	}
	audioDoneMsg struct {
		gen int
		err error
	}
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
// The header badge shows "loading" until the player runs, then "playing"
// until it ends (audioStartedMsg, audioDoneMsg).
func (a *App) play(s wk.Resource[wk.Subject]) tea.Cmd {
	a.shared.audioGen++
	gen := a.shared.audioGen
	a.shared.audio = audioLoading
	done := func(err error) tea.Cmd { return func() tea.Msg { return audioDoneMsg{gen: gen, err: err} } }
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		path, err := files.File(ctx, rec.URL)
		var wait func() error
		if err == nil {
			wait, err = player.Start(ctx, path)
		}
		if err != nil {
			cancel()
			return audioDoneMsg{gen: gen, err: err}
		}
		return audioStartedMsg{gen: gen, wait: func() error { defer cancel(); return wait() }}
	}
}

// audioStarted shows "playing" (unless a newer play has begun) and waits
// for the player to finish.
func (a *App) audioStarted(msg audioStartedMsg) tea.Cmd {
	if msg.gen == a.shared.audioGen {
		a.shared.audio = audioPlaying
	}
	return func() tea.Msg { return audioDoneMsg{gen: msg.gen, err: msg.wait()} }
}

// audioDone clears the badge and shows any problem, unless a newer play
// has begun since.
func (a *App) audioDone(msg audioDoneMsg) {
	if msg.gen != a.shared.audioGen {
		return
	}
	a.shared.audio = audioIdle
	if msg.err != nil {
		a.audioProblem(msg.err)
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
