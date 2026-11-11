package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jtharris/yadokari/internal/audio"
	"github.com/jtharris/yadokari/internal/session"
	"github.com/jtharris/yadokari/internal/wk"
)

// fakeAudio is both the download cache (a URL is its own path) and the
// player (it records what it played).
type fakeAudio struct{ played []string }

func (f *fakeAudio) File(_ context.Context, url string) (string, error) { return url, nil }
func (f *fakeAudio) Play(_ context.Context, path string) error {
	f.played = append(f.played, path)
	return nil
}
func (f *fakeAudio) Formats() []string { return []string{audio.MPEG} }

// recording is the sample data's URL for a subject's default-voice mp3.
func recording(subject int) string {
	return fmt.Sprintf("https://files.example.invalid/audio/%d-1.mp3", subject)
}

func TestLessonAudio(t *testing.T) {
	h := newHarness(t)
	h.key("l")
	// Walk the pages; arriving on a word's Reading tab plays it (the sample
	// user autoplays lessons).
	var want []string
	for h.app.screen == lessonScreen {
		p := h.app.lesson.page
		if p.tabs[p.tab] == tabReading {
			want = append(want, recording(p.data.Subject.ID))
		}
		if p.tab == 0 && p.data.Subject.Object == wk.ObjectVocabulary {
			if !strings.Contains(h.screen(), "p audio") {
				t.Errorf("lesson help should offer audio:\n%s", h.screen())
			}
			before := len(h.snd.played)
			h.key("p")
			if len(h.snd.played) != before+1 || h.snd.played[before] != recording(p.data.Subject.ID) {
				t.Errorf("p should play the word: %v", h.snd.played)
			}
			want = append(want, recording(p.data.Subject.ID))
		}
		h.key("enter")
	}
	if strings.Join(h.snd.played, " ") != strings.Join(want, " ") {
		t.Errorf("played %v, want %v", h.snd.played, want)
	}

	// In the quiz, a correct reading answer plays the word.
	h.snd.played = nil
	for n := 0; h.app.screen == reviewScreen; n++ {
		if n > 20 {
			t.Fatal("quiz did not end")
		}
		q, it, _ := h.app.review.current()
		h.key(h.answerFor())
		h.key("enter")
		if q.Kind == session.Reading {
			if got := h.snd.played; len(got) == 0 || got[len(got)-1] != recording(it.Subject.ID) {
				t.Errorf("correct reading for %d should play it: %v", it.Subject.ID, got)
			}
		}
		h.key("enter")
	}
}

func TestReviewAudioRespectsHiddenReading(t *testing.T) {
	h := newHarness(t)
	h.key("r")
	// The sample's due kana word (いいえ) has only a meaning: p plays it.
	for n := 0; ; n++ {
		_, it, ok := h.app.review.current()
		if !ok || n > 20 {
			t.Fatal("no kana vocabulary question")
		}
		if it.Subject.Object == wk.ObjectKanaVocabulary {
			break
		}
		h.key(h.answerFor())
		h.key("enter")
		h.key("enter")
	}
	_, it, _ := h.app.review.current()
	h.key(h.answerFor())
	h.key("enter")
	if !strings.Contains(h.screen(), "p audio") {
		t.Errorf("help should offer audio:\n%s", h.screen())
	}
	h.key("p")
	if len(h.snd.played) != 1 || h.snd.played[0] != recording(it.Subject.ID) {
		t.Errorf("p should play the kana word: %v", h.snd.played)
	}

	// An item page with its reading hidden won't play.
	p := NewItemPage(itemFixture(wk.ObjectVocabulary), PageDeps{}, nil)
	p.data.Subject.Data.PronunciationAudios = []wk.PronunciationAudio{{URL: "x", ContentType: audio.MPEG}}
	p.HideReading = true
	if cmd := p.Key(tea.KeyPressMsg{Code: 'p', Text: "p"}); cmd != nil {
		t.Error("p must not play a hidden reading")
	}
	p.HideReading = false
	if cmd := p.Key(tea.KeyPressMsg{Code: 'p', Text: "p"}); cmd == nil {
		t.Error("p should play a visible reading")
	}
}

func TestAudioProblemsAreShown(t *testing.T) {
	h := newHarness(t)
	h.app.rdeps.Audio, h.app.rdeps.Sounds = nil, nil
	h.app.rdeps.AudioErr = errors.New("no audio player found (install pipewire-utils)")
	h.key("l")
	for h.app.lesson.page.data.Subject.Object != wk.ObjectVocabulary {
		h.key("enter")
	}
	h.key("p")
	if !strings.Contains(h.screen(), "no audio player found") {
		t.Errorf("the reason should show:\n%s", h.screen())
	}
	h.key("down")
	if strings.Contains(h.screen(), "no audio player found") {
		t.Errorf("the next key clears it:\n%s", h.screen())
	}
}
