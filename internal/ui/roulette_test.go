package ui

import (
	"strings"
	"testing"

	"github.com/jtharris/yadokari/internal/glyph"
	"github.com/jtharris/yadokari/internal/session"
)

func TestRouletteFontPerQuestion(t *testing.T) {
	h := newHarness(t)
	h.app.rdeps.Roulette = glyph.PoolOf(&glyph.Font{Path: "a", Family: "Gothic A"}, &glyph.Font{Path: "b", Family: "Mincho B"})
	h.app.rdeps.Settings.Reviews.AllowUndo = true
	h.key("r")
	r := h.app.review

	_, it, _ := r.current()
	first := r.qfont
	if first == nil || !strings.Contains(r.source(it).Key, first.Path) {
		t.Fatalf("the first question gets a font in its key: %v %q", first, r.source(it).Key)
	}
	if strings.Contains(h.screen(), first.Family) {
		t.Errorf("the font's name is hidden while answering:\n%s", h.screen())
	}

	// A miss: the name shows; an undo keeps the font.
	for {
		if q, _, _ := r.current(); q.Kind == session.Meaning {
			break
		}
		h.key(h.answerFor())
		h.key("enter")
		h.key("enter")
	}
	font := r.qfont
	h.key("zzzz")
	h.key("enter")
	if !strings.Contains(h.screen(), font.Family) {
		t.Errorf("the font's name shows after answering:\n%s", h.screen())
	}
	h.key("backspace")
	if r.qfont != font {
		t.Error("an undo keeps the question's font")
	}
	for range 4 {
		h.key("backspace")
	}

	// Each next question draws again, never the same font twice in a row.
	for range 3 {
		prev := r.qfont
		h.key(h.answerFor())
		h.key("enter")
		if !strings.Contains(h.screen(), "✓") {
			t.Fatalf("expected correct:\n%s", h.screen())
		}
		h.key("enter")
		if h.app.screen != reviewScreen {
			break
		}
		h.screen()
		if r.qfont == nil || r.qfont == prev {
			t.Fatalf("next question: font %v after %v", r.qfont, prev)
		}
	}
}
