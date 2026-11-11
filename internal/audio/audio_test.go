package audio

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jtharris/yadokari/internal/wk"
)

func rec(typ, reading string, voice int) wk.PronunciationAudio {
	return wk.PronunciationAudio{URL: typ + "/" + reading + "/" + string(rune('0'+voice)), ContentType: typ,
		Metadata: wk.AudioMetadata{Pronunciation: reading, VoiceActorID: voice}}
}

func TestPick(t *testing.T) {
	nana := []wk.PronunciationAudio{
		rec(WebM, "なな", 2), rec(MPEG, "なな", 2), rec(WebM, "なな", 1), rec(WebM, "しち", 2),
		rec(MPEG, "なな", 1), rec(WebM, "しち", 1), rec(MPEG, "しち", 2), rec(MPEG, "しち", 1),
	}
	tests := []struct {
		name    string
		audios  []wk.PronunciationAudio
		reading string
		voice   int
		types   []string
		want    string
	}{
		{"reading and voice", nana, "しち", 1, []string{MPEG}, "audio/mpeg/しち/1"},
		{"type preference", nana, "なな", 2, []string{WebM, MPEG}, "audio/webm/なな/2"},
		{"any voice", nana, "なな", 0, []string{MPEG}, "audio/mpeg/なな/2"},
		{"unknown voice falls back", nana, "しち", 9, []string{MPEG}, "audio/mpeg/しち/2"},
		{"unknown reading falls back", nana, "いち", 1, []string{MPEG}, "audio/mpeg/なな/2"},
		{"no playable type", []wk.PronunciationAudio{rec(WebM, "なな", 1)}, "なな", 1, []string{MPEG}, ""},
		{"none", nil, "なな", 1, []string{MPEG}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Pick(tt.audios, tt.reading, tt.voice, tt.types)
			if got.URL != tt.want || ok != (tt.want != "") {
				t.Errorf("Pick = %q %v, want %q", got.URL, ok, tt.want)
			}
		})
	}
}

func TestFind(t *testing.T) {
	have := func(names ...string) func(string) (string, error) {
		return func(n string) (string, error) {
			for _, h := range names {
				if h == n {
					return "/usr/bin/" + n, nil
				}
			}
			return "", exec.ErrNotFound
		}
	}
	p, err := Find(have("mpv", "ffplay"))
	if err != nil || p.Name != "ffplay" || p.argv[0] != "/usr/bin/ffplay" || len(p.Types) != 2 {
		t.Errorf("Find = %+v %v, want ffplay first", p, err)
	}
	if p, _ := Find(have("paplay", "pw-play")); p.Name != "pw-play" {
		t.Errorf("pw-play is preferred, got %s", p.Name)
	}
	if _, err := Find(have()); !errors.Is(err, ErrNoPlayer) {
		t.Errorf("no player: %v", err)
	}
}

func TestPlay(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	ctx := context.Background()
	ok := New([]string{MPEG}, "sh", "-c", "exit 0", "sh")
	if err := ok.Play(ctx, "x.mp3"); err != nil {
		t.Errorf("ok player: %v", err)
	}
	bad := New([]string{MPEG}, "sh", "-c", "echo noise >&2; echo cannot decode $0 >&2; exit 1")
	if err := bad.Play(ctx, "x.mp3"); err == nil || !strings.Contains(err.Error(), "cannot decode x.mp3") {
		t.Errorf("failing player: %v", err)
	}

	// A second Play stops the first, which then reports no error.
	slow := New([]string{MPEG}, "sh", "-c", "sleep 5", "sh")
	first := make(chan error, 1)
	start := time.Now()
	go func() { first <- slow.Play(ctx, "a.mp3") }()
	for range 100 {
		slow.mu.Lock()
		running := slow.cur != nil
		slow.mu.Unlock()
		if running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	slow.argv = []string{"sh", "-c", "exit 0", "sh"}
	if err := slow.Play(ctx, "b.mp3"); err != nil {
		t.Errorf("second play: %v", err)
	}
	if err := <-first; err != nil {
		t.Errorf("stopped play should not report an error: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("the first play was not stopped")
	}
}
