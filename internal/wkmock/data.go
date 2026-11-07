package wkmock

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"yadokari/internal/wk"
)

// Data is everything the mock serves.
type Data struct {
	User             wk.Resource[wk.User]
	Subjects         []wk.Resource[wk.Subject]
	Assignments      []wk.Resource[wk.Assignment]
	StudyMaterials   []wk.Resource[wk.StudyMaterial]
	SRS              []wk.Resource[wk.SpacedRepetitionSystem]
	ReviewStatistics []wk.Resource[wk.ReviewStatistic]
}

func (d Data) clone() Data {
	raw, err := json.Marshal(d)
	if err != nil {
		panic(err)
	}
	var out Data
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	out.sort()
	return out
}

func (d *Data) sort() {
	byID := func(a, b int) int { return a - b }
	slices.SortFunc(d.Subjects, func(a, b wk.Resource[wk.Subject]) int { return byID(a.ID, b.ID) })
	slices.SortFunc(d.Assignments, func(a, b wk.Resource[wk.Assignment]) int { return byID(a.ID, b.ID) })
	slices.SortFunc(d.StudyMaterials, func(a, b wk.Resource[wk.StudyMaterial]) int { return byID(a.ID, b.ID) })
	slices.SortFunc(d.SRS, func(a, b wk.Resource[wk.SpacedRepetitionSystem]) int { return byID(a.ID, b.ID) })
	slices.SortFunc(d.ReviewStatistics, func(a, b wk.Resource[wk.ReviewStatistic]) int { return byID(a.ID, b.ID) })
}

// Fixture file names inside a fixtures directory. user.json holds one
// resource; the others hold a JSON array of resources (all pages merged).
const (
	FileUser             = "user.json"
	FileSubjects         = "subjects.json"
	FileAssignments      = "assignments.json"
	FileStudyMaterials   = "study_materials.json"
	FileSRS              = "spaced_repetition_systems.json"
	FileReviewStatistics = "review_statistics.json"
)

// LoadDir reads fixtures from dir. Missing files leave that collection empty.
func LoadDir(dir string) (Data, error) {
	var d Data
	files := []struct {
		name string
		dst  any
	}{
		{FileUser, &d.User},
		{FileSubjects, &d.Subjects},
		{FileAssignments, &d.Assignments},
		{FileStudyMaterials, &d.StudyMaterials},
		{FileSRS, &d.SRS},
		{FileReviewStatistics, &d.ReviewStatistics},
	}
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(dir, f.name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return Data{}, err
		}
		if err := json.Unmarshal(raw, f.dst); err != nil {
			return Data{}, fmt.Errorf("wkmock: %s: %w", f.name, err)
		}
	}
	d.sort()
	return d, nil
}

// SampleData is a tiny hand-written data set: a level 1 user with a few
// reviews due now, a few lessons waiting and some reviews later today.
// IDs and content are illustrative, not copies of real subjects.
func SampleData(now time.Time) Data {
	now = now.UTC()
	t := func(d time.Duration) *time.Time { v := now.Add(d).Truncate(time.Hour); return &v }
	str := func(s string) *string { return &s }
	created := now.Add(-30 * 24 * time.Hour)
	res := func(object string, id int) (string, int, time.Time) { return object, id, created }

	subj := func(object string, id int, chars *string, level, pos int, meanings []string, readings []wk.Reading, data wk.Subject) wk.Resource[wk.Subject] {
		obj, id, updated := res(object, id)
		data.Characters = chars
		data.Level = level
		data.LessonPosition = pos
		data.SpacedRepetitionSystemID = 1
		data.CreatedAt = created
		if data.Slug == "" && chars != nil {
			data.Slug = *chars
		}
		data.DocumentURL = "https://www.wanikani.com/" + object + "/" + data.Slug
		for i, m := range meanings {
			data.Meanings = append(data.Meanings, wk.Meaning{Meaning: m, Primary: i == 0, AcceptedAnswer: true})
		}
		data.Readings = readings
		if data.AuxiliaryMeanings == nil {
			data.AuxiliaryMeanings = []wk.AuxiliaryMeaning{}
		}
		return wk.Resource[wk.Subject]{ID: id, Object: obj, DataUpdatedAt: updated, Data: data}
	}
	on := func(r string, primary bool) wk.Reading {
		return wk.Reading{Reading: r, Primary: primary, AcceptedAnswer: primary, Type: wk.ReadingOnyomi}
	}
	kun := func(r string) wk.Reading { return wk.Reading{Reading: r, Type: wk.ReadingKunyomi} }
	vr := func(r string) wk.Reading { return wk.Reading{Reading: r, Primary: true, AcceptedAnswer: true} }

	subjects := []wk.Resource[wk.Subject]{
		subj(wk.ObjectRadical, 1, str("一"), 1, 0, []string{"Ground"}, nil, wk.Subject{
			Slug:                   "ground",
			MeaningMnemonic:        "This radical is a single flat line, like the <radical>ground</radical>.",
			AmalgamationSubjectIDs: []int{101},
			AuxiliaryMeanings:      []wk.AuxiliaryMeaning{{Meaning: "one", Type: wk.AuxBlacklist}},
		}),
		subj(wk.ObjectRadical, 2, nil, 1, 1, []string{"Stick Figure"}, nil, wk.Subject{
			Slug:            "stick-figure",
			MeaningMnemonic: "An image-only radical, to exercise the no-characters path.",
			CharacterImages: []wk.CharacterImage{{
				URL:         "https://example.invalid/radicals/stick-figure.svg",
				ContentType: "image/svg+xml",
				Metadata:    json.RawMessage(`{"inline_styles":true}`),
			}},
		}),
		subj(wk.ObjectRadical, 3, str("二"), 1, 2, []string{"Two"}, nil, wk.Subject{
			MeaningMnemonic: "Two lines: <radical>two</radical>.", AmalgamationSubjectIDs: []int{102}, Slug: "two",
		}),
		subj(wk.ObjectKanji, 101, str("一"), 1, 0, []string{"One"}, []wk.Reading{on("いち", true), kun("ひと")}, wk.Subject{
			ComponentSubjectIDs:    []int{1},
			AmalgamationSubjectIDs: []int{201, 202},
			MeaningMnemonic:        "The <radical>ground</radical> is <kanji>one</kanji> thing.",
			ReadingMnemonic:        "Say <reading>いち</reading> when you count the first one.",
			AuxiliaryMeanings:      []wk.AuxiliaryMeaning{{Meaning: "1", Type: wk.AuxWhitelist}},
		}),
		subj(wk.ObjectKanji, 102, str("二"), 1, 1, []string{"Two"}, []wk.Reading{on("に", true), kun("ふた")}, wk.Subject{
			ComponentSubjectIDs: []int{3},
			MeaningMnemonic:     "The <radical>two</radical> radical is the <kanji>two</kanji> kanji.",
			ReadingMnemonic:     "<reading>に</reading> is two.",
		}),
		subj(wk.ObjectVocabulary, 201, str("一"), 1, 0, []string{"One"}, []wk.Reading{vr("いち")}, wk.Subject{
			ComponentSubjectIDs: []int{101},
			PartsOfSpeech:       []string{"numeral"},
			MeaningMnemonic:     "Same as the kanji.",
			ReadingMnemonic:     "Uses the on'yomi.",
			ContextSentences:    []wk.ContextSentence{{Ja: "一から十まで。", En: "From one to ten."}},
		}),
		subj(wk.ObjectVocabulary, 202, str("一つ"), 1, 1, []string{"One Thing"}, []wk.Reading{vr("ひとつ")}, wk.Subject{
			ComponentSubjectIDs: []int{101},
			PartsOfSpeech:       []string{"numeral"},
			MeaningMnemonic:     "One of a thing.",
			ReadingMnemonic:     "Uses the kun'yomi.",
		}),
		subj(wk.ObjectKanaVocabulary, 301, str("いいえ"), 1, 2, []string{"No"}, nil, wk.Subject{
			Slug:            "iie",
			PartsOfSpeech:   []string{"expression"},
			MeaningMnemonic: "A polite no.",
		}),
	}

	type st struct {
		subject int
		stage   int
		avail   *time.Time
		started bool
	}
	states := []st{
		{1, 4, t(-2 * time.Hour), true},   // radical due
		{3, 1, t(-1 * time.Hour), true},   // radical due
		{101, 2, t(-1 * time.Hour), true}, // kanji due
		{102, 5, t(3 * time.Hour), true},  // later today
		{201, 0, nil, false},              // lesson
		{202, 0, nil, false},              // lesson
		{301, 3, t(0), true},              // kana vocab due
		{2, 6, t(30 * time.Hour), true},   // beyond the 24h forecast
	}
	var assignments []wk.Resource[wk.Assignment]
	for i, s := range states {
		subject := subjects[slices.IndexFunc(subjects, func(r wk.Resource[wk.Subject]) bool { return r.ID == s.subject })]
		a := wk.Assignment{
			SubjectID:   s.subject,
			SubjectType: subject.Object,
			SRSStage:    s.stage,
			UnlockedAt:  &created,
			AvailableAt: s.avail,
			CreatedAt:   created,
		}
		if s.started {
			a.StartedAt = &created
		}
		if s.stage >= passingStage {
			a.PassedAt = &created
		}
		assignments = append(assignments, wk.Resource[wk.Assignment]{
			ID: 1000 + i, Object: "assignment", DataUpdatedAt: created, Data: a,
		})
	}

	sec := func(n int64) *int64 { return &n }
	unit := str("seconds")
	stages := []wk.SRSStage{{Position: 0}}
	for pos := 1; pos <= 8; pos++ {
		stages = append(stages, wk.SRSStage{Position: pos, Interval: sec(defaultIntervals[pos]), IntervalUnit: unit})
	}
	stages = append(stages, wk.SRSStage{Position: 9})

	return Data{
		User: wk.Resource[wk.User]{
			Object:        "user",
			DataUpdatedAt: created,
			Data: wk.User{
				ID:         "00000000-0000-4000-8000-000000000001",
				Username:   "mock_learner",
				Level:      1,
				ProfileURL: "https://www.wanikani.com/users/mock_learner",
				StartedAt:  created,
				Subscription: wk.Subscription{
					Active: false, Type: "free", MaxLevelGranted: 3,
				},
				Preferences: wk.Preferences{
					LessonsBatchSize:         5,
					LessonsPresentationOrder: "ascending_level_then_subject",
					ReviewsPresentationOrder: "shuffled",
					DefaultVoiceActorID:      1,
				},
			},
		},
		Subjects:    subjects,
		Assignments: assignments,
		StudyMaterials: []wk.Resource[wk.StudyMaterial]{{
			ID: 5000, Object: "study_material", DataUpdatedAt: created,
			Data: wk.StudyMaterial{SubjectID: 101, SubjectType: wk.ObjectKanji, MeaningSynonyms: []string{"single"}, CreatedAt: created},
		}},
		SRS: []wk.Resource[wk.SpacedRepetitionSystem]{{
			ID: 1, Object: "spaced_repetition_system", DataUpdatedAt: created,
			Data: wk.SpacedRepetitionSystem{
				Name: "Default system", Description: "Mock default system",
				UnlockingStagePosition: 0, StartingStagePosition: 1, PassingStagePosition: 5, BurningStagePosition: 9,
				Stages: stages, CreatedAt: created,
			},
		}},
	}
}
