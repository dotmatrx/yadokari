package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jtharris/yadokari/internal/session"
	"github.com/jtharris/yadokari/internal/store"
)

// PreviewScreens are the screens and states Preview can render.
var PreviewScreens = []string{"dashboard", "review-empty", "review", "review-reading", "review-wrong", "summary", "item", "item-vocab", "lesson"}

// Preview renders one screen once at w×h from the store, without a running
// program: for iterating on layout (cmd/preview) and golden tests. Review
// states are reached by typing into a real session, so the store is
// changed (reviews get queued); use a throwaway store. The result is padded
// to h lines. With a kitty renderer, send rd.Renderer.Flush() before it.
func Preview(ctx context.Context, d Deps, rd ReviewDeps, screen string, w, h int) (string, error) {
	size := tea.WindowSizeMsg{Width: w, Height: h}
	switch screen {
	case "dashboard":
		m := NewDashboard(d)
		m.Update(size)
		m.Update(m.load()())
		if m.syncErr != nil {
			return "", m.syncErr
		}
		return fit(m.render(), h), nil

	case "review-empty", "review", "review-reading", "review-wrong":
		u, err := d.Store.User(ctx)
		if err != nil || u == nil {
			return "", fmt.Errorf("preview: no user in the store: %v", err)
		}
		r, err := LoadReview(ctx, d, rd, u.Data)
		if err != nil {
			return "", err
		}
		r.Update(size)
		// Not ready while typing: each Update would otherwise queue a kitty
		// upload into a command this function discards, and the final frame
		// would point at an image the terminal never received.
		want := session.Meaning
		if screen == "review-reading" {
			want = session.Reading
		}
		// Answer correctly until a question of the wanted kind comes up.
		for range 100 {
			q, it, ok := r.current()
			if !ok {
				return "", errors.New("preview: ran out of questions")
			}
			if q.Kind == want {
				break
			}
			typeKeys(r, correctAnswer(q, it))
			enterKey(r)
			enterKey(r)
		}
		q, it, _ := r.current()
		answer := []rune(correctAnswer(q, it))
		switch screen {
		case "review-empty": // nothing typed: the placeholder shows
		case "review": // half-typed meaning
			typeKeys(r, string(answer[:max(len(answer)/2, 1)]))
		case "review-reading": // kana so far plus a pending consonant
			typeKeys(r, string(answer[:max(len(answer)-1, 1)])+"s")
		case "review-wrong":
			wrong := "Mountain"
			if strings.EqualFold(string(answer), wrong) {
				wrong = "River"
			}
			typeKeys(r, wrong)
			enterKey(r)
		}
		r.ready = true // the caller sends the View's uploads (Flush)
		return fit(r.View(), h), nil

	case "item", "item-vocab":
		p, err := previewItem(ctx, d, rd, screen == "item-vocab")
		if err != nil {
			return "", err
		}
		p.SetSize(w, h)
		return fit(p.View(), h), nil

	case "lesson":
		u, err := d.Store.User(ctx)
		if err != nil || u == nil {
			return "", fmt.Errorf("preview: no user in the store: %v", err)
		}
		l, _, err := LoadLessons(ctx, d, rd, u.Data)
		if err != nil {
			return "", err
		}
		l.Update(size)
		l.page.gfx.Ready = true // the caller sends the View's uploads (Flush)
		return fit(l.View(), h), nil

	case "summary":
		s, err := previewSummary(ctx, d.Store, d)
		if err != nil {
			return "", err
		}
		return fit(s.View(w, h), h), nil
	}
	return "", fmt.Errorf("preview: unknown screen %q (%s)", screen, strings.Join(PreviewScreens, ", "))
}

// previewItem opens the item page on the due kanji used in the most words
// (its Meaning tab), or the due word written with the most kanji (its Kanji tab).
func previewItem(ctx context.Context, d Deps, rd ReviewDeps, vocab bool) (*ItemPage, error) {
	u, err := d.Store.User(ctx)
	if err != nil || u == nil {
		return nil, fmt.Errorf("preview: no user in the store: %v", err)
	}
	items, err := d.Store.ReviewItems(ctx, d.Now(), u.Data.Subscription.MaxLevelGranted)
	if err != nil || len(items) == 0 {
		return nil, fmt.Errorf("preview: no due items: %v", err)
	}
	pick, best := items[0], 0
	for _, it := range items {
		s := it.Subject
		n := len(s.Data.AmalgamationSubjectIDs)
		if vocab {
			n = len(s.Data.ComponentSubjectIDs)
		}
		if want := map[bool]string{false: "kanji", true: "vocabulary"}[vocab]; s.Object == want && n > best {
			pick, best = it, n
		}
	}
	data, err := LoadItem(ctx, d.Store, pick.Subject.ID)
	if err != nil {
		return nil, err
	}
	p := NewItemPage(data, GlyphDeps{Renderer: rd.Renderer, Font: rd.Font, Ready: true}, nil)
	if !vocab && len(p.tabs) > 1 {
		p.setTab(1)
	}
	return p, nil
}

// previewSummary makes a plausible finished session from the due items:
// every third one missed, stages moved the way WaniKani would.
func previewSummary(ctx context.Context, st *store.Store, d Deps) (Summary, error) {
	u, err := st.User(ctx)
	if err != nil || u == nil {
		return Summary{}, fmt.Errorf("preview: no user in the store: %v", err)
	}
	items, err := st.ReviewItems(ctx, d.Now(), u.Data.Subscription.MaxLevelGranted)
	if err != nil {
		return Summary{}, err
	}
	var s Summary
	for i, it := range items[:min(len(items), 18)] {
		from := it.Assignment.Data.SRSStage
		line := SummaryItem{
			Characters: characters(it.Subject), Meaning: primaryMeaning(it.Subject.Data), Type: it.Subject.Object,
			AssignmentID: it.Assignment.ID, From: from, To: min(from+1, 9),
		}
		if i%3 == 1 {
			line.To = max(from-1, 1)
			if from >= 5 {
				line.To = from - 2
			}
			s.Incorrect = append(s.Incorrect, line)
		} else {
			s.Correct = append(s.Correct, line)
		}
	}
	return s, nil
}

func correctAnswer(q session.Question, it store.ReviewItem) string {
	if q.Kind == session.Reading {
		for _, r := range it.Subject.Data.Readings {
			if r.Primary {
				return r.Reading
			}
		}
	}
	return primaryMeaning(it.Subject.Data)
}

func typeKeys(r *Review, s string) {
	for _, c := range s {
		r.Update(tea.KeyPressMsg{Code: c, Text: string(c)})
	}
}

func enterKey(r *Review) { r.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) }

// fit pads s to h lines, so a preview shows the whole terminal.
func fit(s string, h int) string {
	n := strings.Count(s, "\n") + 1
	if n < h {
		s += strings.Repeat("\n", h-n)
	}
	return s
}
