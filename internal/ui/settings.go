package ui

import (
	"strings"

	"github.com/dotmatrx/yadokari/internal/answer"
	"github.com/dotmatrx/yadokari/internal/config"
	"github.com/dotmatrx/yadokari/internal/session"
	"github.com/dotmatrx/yadokari/internal/wk"
)

// Settings are the config file's review and lesson settings. The zero
// value follows the user's WaniKani preferences.
type Settings struct {
	Reviews config.Reviews
	Lessons config.Lessons
	Audio   config.Audio
}

// voice returns the voice actor to play from audios: the one audio.voice
// names, if the subject has a recording by them, else preferred (the
// WaniKani default voice).
func (s Settings) voice(audios []wk.PronunciationAudio, preferred int) int {
	if name := strings.TrimSpace(s.Audio.Voice); name != "" {
		for _, a := range audios {
			if strings.EqualFold(a.Metadata.VoiceActorName, name) {
				return a.Metadata.VoiceActorID
			}
		}
	}
	return preferred
}

// prefs returns the WaniKani preferences with the config file's overrides
// applied: what lessons and autoplay go by.
func (s Settings) prefs(p wk.Preferences) wk.Preferences {
	if s.Lessons.BatchSize > 0 {
		p.LessonsBatchSize = s.Lessons.BatchSize
	}
	if o := s.Lessons.Order; o != "" && o != "wanikani" {
		p.LessonsPresentationOrder = o
	}
	if a := s.Lessons.AutoplayAudio; a != nil {
		p.LessonsAutoplayAudio = *a
	}
	if a := s.Reviews.AutoplayAudio; a != nil {
		p.ReviewsAutoplayAudio = *a
	}
	return p
}

// sessionOptions are a new review session's options.
func (s Settings) sessionOptions(p wk.Preferences) session.Options {
	c := s.Reviews
	opts := session.Options{
		ActiveSize: c.ActiveSize, Limit: c.Limit, BackToBack: c.BackToBack,
		ReadingFirst: c.ReadingFirst, AllowUndo: c.AllowUndo,
	}
	switch c.Order {
	case "", "wanikani":
		if p.ReviewsPresentationOrder == "lower_levels_first" {
			opts.Order = session.LowerLevelsFirst
		}
	default:
		opts.Order = session.Order(c.Order)
	}
	return opts
}

// strictness is the meaning typo tolerance.
func (s Settings) strictness() answer.Strictness {
	switch s.Reviews.Strictness {
	case "strict":
		return answer.Strict
	case "lenient":
		return answer.Lenient
	}
	return answer.WaniKaniLike
}
