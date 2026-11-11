package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jtharris/yadokari/internal/session"
	"github.com/jtharris/yadokari/internal/wk"
)

func TestItemPageFromReview(t *testing.T) {
	h := newHarness(t)
	h.key("r")
	// Answer until the first question of an item with both parts.
	for n := 0; ; n++ {
		q, it, ok := h.app.review.current()
		if !ok || n > 20 {
			t.Fatal("no item with a meaning and a reading")
		}
		st := h.app.review.sess.Item(q.Index)
		if len(it.Subject.Data.Readings) > 0 && !st.ReadingDone && !st.MeaningDone {
			break
		}
		h.key(h.answerFor())
		h.key("enter")
		h.key("enter")
	}
	q, it, _ := h.app.review.current()
	h.key(h.answerFor())
	h.key("enter")
	h.key("i")
	out := h.screen()
	if !strings.Contains(out, "yadokari · item") {
		t.Fatalf("item page should open:\n%s", out)
	}
	// The part not yet asked stays hidden, in the summary and in its tab.
	hiddenNote, hiddenTab, tabText := "Reading hidden until it's asked", tabReading, "Hidden until the reading has been asked."
	var secrets []string
	for _, r := range it.Subject.Data.Readings {
		secrets = append(secrets, r.Reading)
	}
	if q.Kind == session.Reading {
		hiddenNote, hiddenTab, tabText = "Meaning hidden until it's asked", tabMeaning, "Hidden until the meaning has been asked."
		secrets = []string{primaryMeaning(it.Subject.Data)}
	}
	if !strings.Contains(out, hiddenNote) {
		t.Errorf("want %q:\n%s", hiddenNote, out)
	}
	p := h.app.review.item
	for range len(p.tabs) {
		if p.tabs[p.tab] == hiddenTab {
			break
		}
		h.send(tea.KeyPressMsg{Code: tea.KeyRight})
	}
	out = h.screen()
	if !strings.Contains(out, tabText) {
		t.Errorf("want %q:\n%s", tabText, out)
	}
	for _, sec := range secrets {
		if strings.Contains(out, sec) {
			t.Errorf("%q given away:\n%s", sec, out)
		}
	}
	h.key("esc")
	if h.app.review.item != nil || !strings.Contains(h.screen(), "✓") {
		t.Fatalf("Esc should return to the graded answer:\n%s", h.screen())
	}
	if h.app.screen != reviewScreen {
		t.Fatal("Esc on the item page must not leave the review")
	}
}

func itemFixture(object string) ItemData {
	chars := "大人"
	d := ItemData{Subject: wk.Resource[wk.Subject]{ID: 1, Object: object, Data: wk.Subject{
		Characters: &chars, Level: 3,
		Meanings: []wk.Meaning{{Meaning: "Adult", Primary: true, AcceptedAnswer: true},
			{Meaning: "Grown-up", AcceptedAnswer: true}},
		Readings:         []wk.Reading{{Reading: "おとな", Primary: true, AcceptedAnswer: true}},
		MeaningMnemonic:  strings.Repeat("A <kanji>big</kanji> <kanji>person</kanji> is an <vocabulary>adult</vocabulary>. ", 30),
		ReadingMnemonic:  "Say <reading>おとな</reading>.",
		ContextSentences: []wk.ContextSentence{{Ja: "大人です。", En: "I'm an adult."}},
		PartsOfSpeech:    []string{"noun"},
	}}}
	return d
}

func TestItemPageTabsAndScroll(t *testing.T) {
	for obj, want := range map[string]string{
		wk.ObjectRadical:        "Name · Found in",
		wk.ObjectKanji:          "Radicals · Meaning · Reading · Found in",
		wk.ObjectVocabulary:     "Kanji · Meaning · Reading · Context",
		wk.ObjectKanaVocabulary: "Meaning · Context",
	} {
		p := NewItemPage(itemFixture(obj), PageDeps{}, nil)
		p.SetSize(80, 24)
		if out := ansi.Strip(p.View()); !strings.Contains(out, want) {
			t.Errorf("%s: tabs %q missing:\n%s", obj, want, out)
		}
	}

	p := NewItemPage(itemFixture(wk.ObjectVocabulary), PageDeps{}, nil)
	p.SetSize(60, 20)
	p.Key(tea.KeyPressMsg{Code: tea.KeyRight}) // Meaning
	out := ansi.Strip(p.View())
	if !strings.Contains(out, "Mnemonic") || !strings.Contains(out, "1–") || strings.Contains(out, "<kanji>") {
		t.Fatalf("long mnemonic should scroll, without markup:\n%s", out)
	}
	for range 100 {
		p.Key(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	lines := strings.Count(p.body(p.innerWidth()), "\n") + 1
	if p.top != lines-p.bodyRows() {
		t.Errorf("scrolled to %d, want the last page at %d", p.top, lines-p.bodyRows())
	}
	p.Key(tea.KeyPressMsg{Code: tea.KeyLeft})
	if p.top != 0 || p.tabs[p.tab] != tabComponents {
		t.Errorf("changing tab should reset the scroll: tab %d top %d", p.tab, p.top)
	}
	for _, line := range strings.Split(ansi.Strip(p.View()), "\n") {
		if w := ansi.StringWidth(line); w > 60 {
			t.Errorf("line %d wide: %q", w, line)
		}
	}
}

func TestItemPageHidesMeaning(t *testing.T) {
	p := NewItemPage(itemFixture(wk.ObjectVocabulary), PageDeps{}, nil)
	p.HideMeaning = true
	p.SetSize(80, 24)
	for _, tab := range []int{1, 3} { // Meaning, Context
		p.setTab(tab)
		out := ansi.Strip(p.View())
		for _, secret := range []string{"Adult", "Grown-up", "I'm an adult"} {
			if strings.Contains(out, secret) {
				t.Errorf("tab %d gives away %q:\n%s", tab, secret, out)
			}
		}
	}
}
