// Package audio picks a subject's pronunciation recording and plays it with
// an external player. The binary is static (no cgo), so it can't open a
// sound device itself; it hands a file to the first player it finds.
package audio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/jtharris/yadokari/internal/wk"
)

// Content types WaniKani serves (no ogg).
const (
	MPEG = "audio/mpeg"
	WebM = "audio/webm"
)

// Pick chooses the recording to play: one in a format the player handles
// (in its order of preference) for reading, by voice. When nothing matches,
// it relaxes the voice, then the reading. ok is false when the subject has
// no playable recording.
func Pick(audios []wk.PronunciationAudio, reading string, voice int, types []string) (wk.PronunciationAudio, bool) {
	for _, pass := range []struct{ voice, reading bool }{{true, true}, {false, true}, {false, false}} {
		for _, typ := range types {
			for _, a := range audios {
				if a.ContentType != typ ||
					(pass.reading && reading != "" && a.Metadata.Pronunciation != reading) ||
					(pass.voice && voice != 0 && a.Metadata.VoiceActorID != voice) {
					continue
				}
				return a, true
			}
		}
	}
	return wk.PronunciationAudio{}, false
}

// Player plays audio files with an external program.
type Player struct {
	Name  string   // the program, e.g. pw-play
	Types []string // content types it plays, preferred first
	argv  []string // the command; the file path is appended

	mu  sync.Mutex
	cur *exec.Cmd
}

// candidates are tried in order. pw-play and paplay go through libsndfile,
// which reads mp3 (since 1.1) but not webm.
var candidates = []struct {
	argv  []string
	types []string
}{
	{[]string{"pw-play"}, []string{MPEG}},
	{[]string{"paplay"}, []string{MPEG}},
	{[]string{"ffplay", "-nodisp", "-autoexit", "-loglevel", "error"}, []string{MPEG, WebM}},
	{[]string{"mpv", "--no-terminal", "--no-video"}, []string{MPEG, WebM}},
}

// ErrNoPlayer means none of the supported players is installed.
var ErrNoPlayer = errors.New("no audio player found (install pipewire-utils for pw-play, or ffmpeg for ffplay)")

// Find returns the first installed player; lookPath is exec.LookPath, or a
// fake in tests.
func Find(lookPath func(string) (string, error)) (*Player, error) {
	for _, c := range candidates {
		if path, err := lookPath(c.argv[0]); err == nil {
			argv := append([]string{path}, c.argv[1:]...)
			return &Player{Name: c.argv[0], Types: c.types, argv: argv}, nil
		}
	}
	return nil, ErrNoPlayer
}

// New makes a player from a command line (the file path is appended).
func New(types []string, argv ...string) *Player {
	return &Player{Name: argv[0], Types: types, argv: argv}
}

// Play plays the file and waits until it ends; see Start.
func (p *Player) Play(ctx context.Context, path string) error {
	wait, err := p.Start(ctx, path)
	if err != nil {
		return err
	}
	return wait()
}

// Start starts playing the file, stopping whatever this player was playing,
// and returns once the player is running; wait blocks until it ends. The
// player gets no terminal: no input or output, and its own process group,
// so it can't disturb the TUI. A play stopped by a newer one, or by ctx,
// reports no error.
func (p *Player) Start(ctx context.Context, path string) (wait func() error, err error) {
	cmd := exec.CommandContext(ctx, p.argv[0], append(slices.Clone(p.argv[1:]), path)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	p.mu.Lock()
	if p.cur != nil && p.cur.Process != nil {
		_ = p.cur.Process.Kill()
	}
	if err := cmd.Start(); err != nil {
		p.mu.Unlock()
		return nil, fmt.Errorf("audio: %s: %w", p.Name, err)
	}
	p.cur = cmd
	p.mu.Unlock()

	return func() error {
		err := cmd.Wait()
		p.mu.Lock()
		stopped := p.cur != cmd // a newer Start replaced this one
		if !stopped {
			p.cur = nil
		}
		p.mu.Unlock()
		if err == nil || stopped || ctx.Err() != nil {
			return nil
		}
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
			msg = msg[i+1:]
		}
		if msg != "" {
			return fmt.Errorf("audio: %s: %s", p.Name, msg)
		}
		return fmt.Errorf("audio: %s: %w", p.Name, err)
	}, nil
}

// Formats returns the content types the player plays, preferred first.
func (p *Player) Formats() []string { return p.Types }
