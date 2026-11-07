package wk

import (
	"encoding/json"
	"time"
)

// Revision is the API revision this client is written against.
const Revision = "20170710"

// Object type names as they appear in the "object" field.
const (
	ObjectRadical        = "radical"
	ObjectKanji          = "kanji"
	ObjectVocabulary     = "vocabulary"
	ObjectKanaVocabulary = "kana_vocabulary"
)

// Resource is the envelope around a single API object.
type Resource[T any] struct {
	ID            int       `json:"id"`
	Object        string    `json:"object"`
	URL           string    `json:"url"`
	DataUpdatedAt time.Time `json:"data_updated_at"`
	Data          T         `json:"data"`
}

// Pages holds collection pagination links.
type Pages struct {
	NextURL     *string `json:"next_url"`
	PreviousURL *string `json:"previous_url"`
	PerPage     int     `json:"per_page"`
}

// Collection is one page of a collection endpoint.
type Collection[T any] struct {
	Object        string        `json:"object"`
	URL           string        `json:"url"`
	Pages         Pages         `json:"pages"`
	TotalCount    int           `json:"total_count"`
	DataUpdatedAt *time.Time    `json:"data_updated_at"`
	Data          []Resource[T] `json:"data"`
}

// User is the data of GET /user.
type User struct {
	ID                       string       `json:"id"`
	Username                 string       `json:"username"`
	Level                    int          `json:"level"`
	ProfileURL               string       `json:"profile_url"`
	StartedAt                time.Time    `json:"started_at"`
	CurrentVacationStartedAt *time.Time   `json:"current_vacation_started_at"`
	Subscription             Subscription `json:"subscription"`
	Preferences              Preferences  `json:"preferences"`
}

// Subscription describes the user's plan.
type Subscription struct {
	Active          bool       `json:"active"`
	Type            string     `json:"type"` // free, recurring, lifetime, unknown
	MaxLevelGranted int        `json:"max_level_granted"`
	PeriodEndsAt    *time.Time `json:"period_ends_at"`
}

// Preferences are the user's WaniKani settings relevant to a client.
type Preferences struct {
	LessonsBatchSize           int    `json:"lessons_batch_size"`
	LessonsAutoplayAudio       bool   `json:"lessons_autoplay_audio"`
	LessonsPresentationOrder   string `json:"lessons_presentation_order"`
	ReviewsAutoplayAudio       bool   `json:"reviews_autoplay_audio"`
	ReviewsDisplaySRSIndicator bool   `json:"reviews_display_srs_indicator"`
	ReviewsPresentationOrder   string `json:"reviews_presentation_order"`
	ExtraStudyAutoplayAudio    bool   `json:"extra_study_autoplay_audio"`
	DefaultVoiceActorID        int    `json:"default_voice_actor_id"`
}

// Summary is the data of GET /summary.
type Summary struct {
	Lessons       []SummaryBucket `json:"lessons"`
	NextReviewsAt *time.Time      `json:"next_reviews_at"`
	Reviews       []SummaryBucket `json:"reviews"`
}

// SummaryBucket lists subjects available at a given hour.
type SummaryBucket struct {
	AvailableAt time.Time `json:"available_at"`
	SubjectIDs  []int     `json:"subject_ids"`
}

// Subject is the union of radical, kanji, vocabulary and kana_vocabulary data.
// The subject kind comes from the enclosing Resource's Object field.
type Subject struct {
	Characters                *string              `json:"characters"` // null for image-only radicals
	Level                     int                  `json:"level"`
	Slug                      string               `json:"slug"`
	LessonPosition            int                  `json:"lesson_position"`
	Meanings                  []Meaning            `json:"meanings"`
	AuxiliaryMeanings         []AuxiliaryMeaning   `json:"auxiliary_meanings"`
	Readings                  []Reading            `json:"readings,omitempty"`
	MeaningMnemonic           string               `json:"meaning_mnemonic"`
	MeaningHint               *string              `json:"meaning_hint,omitempty"`
	ReadingMnemonic           string               `json:"reading_mnemonic,omitempty"`
	ReadingHint               *string              `json:"reading_hint,omitempty"`
	ComponentSubjectIDs       []int                `json:"component_subject_ids,omitempty"`
	AmalgamationSubjectIDs    []int                `json:"amalgamation_subject_ids,omitempty"`
	VisuallySimilarSubjectIDs []int                `json:"visually_similar_subject_ids,omitempty"`
	CharacterImages           []CharacterImage     `json:"character_images,omitempty"`
	PartsOfSpeech             []string             `json:"parts_of_speech,omitempty"`
	ContextSentences          []ContextSentence    `json:"context_sentences,omitempty"`
	PronunciationAudios       []PronunciationAudio `json:"pronunciation_audios,omitempty"`
	SpacedRepetitionSystemID  int                  `json:"spaced_repetition_system_id"`
	DocumentURL               string               `json:"document_url"`
	HiddenAt                  *time.Time           `json:"hidden_at"`
	CreatedAt                 time.Time            `json:"created_at"`
}

// Meaning is an accepted (or listed) English meaning.
type Meaning struct {
	Meaning        string `json:"meaning"`
	Primary        bool   `json:"primary"`
	AcceptedAnswer bool   `json:"accepted_answer"`
}

// Auxiliary meaning types.
const (
	AuxWhitelist = "whitelist"
	AuxBlacklist = "blacklist"
)

// AuxiliaryMeaning is an extra allowed (whitelist) or rejected (blacklist) answer.
type AuxiliaryMeaning struct {
	Meaning string `json:"meaning"`
	Type    string `json:"type"`
}

// Reading types (kanji only; vocabulary readings have no type).
const (
	ReadingOnyomi  = "onyomi"
	ReadingKunyomi = "kunyomi"
	ReadingNanori  = "nanori"
)

// Reading is a kana reading.
type Reading struct {
	Reading        string `json:"reading"`
	Primary        bool   `json:"primary"`
	AcceptedAnswer bool   `json:"accepted_answer"`
	Type           string `json:"type,omitempty"`
}

// CharacterImage is a radical image. Despite the docs ("only image/svg+xml"),
// real data has several PNG sizes plus one SVG per radical.
type CharacterImage struct {
	URL         string          `json:"url"`
	ContentType string          `json:"content_type"`
	Metadata    json.RawMessage `json:"metadata"`
}

// ContextSentence pairs a Japanese example sentence with its translation.
type ContextSentence struct {
	En string `json:"en"`
	Ja string `json:"ja"`
}

// PronunciationAudio is one recording of a vocabulary reading.
type PronunciationAudio struct {
	URL         string        `json:"url"`
	ContentType string        `json:"content_type"`
	Metadata    AudioMetadata `json:"metadata"`
}

// AudioMetadata describes a pronunciation recording.
type AudioMetadata struct {
	Gender           string `json:"gender"`
	SourceID         int    `json:"source_id"`
	Pronunciation    string `json:"pronunciation"`
	VoiceActorID     int    `json:"voice_actor_id"`
	VoiceActorName   string `json:"voice_actor_name"`
	VoiceDescription string `json:"voice_description"`
}

// Assignment is the user's SRS state for one subject.
type Assignment struct {
	SubjectID   int    `json:"subject_id"`
	SubjectType string `json:"subject_type"`
	// Level appears in some doc examples but is absent from real collection
	// responses; look the level up from the subject instead.
	Level         int        `json:"level,omitempty"`
	SRSStage      int        `json:"srs_stage"`
	UnlockedAt    *time.Time `json:"unlocked_at"`
	StartedAt     *time.Time `json:"started_at"`
	AvailableAt   *time.Time `json:"available_at"`
	PassedAt      *time.Time `json:"passed_at"`
	BurnedAt      *time.Time `json:"burned_at"`
	ResurrectedAt *time.Time `json:"resurrected_at"`
	Hidden        bool       `json:"hidden"`
	CreatedAt     time.Time  `json:"created_at"`
}

// StudyMaterial holds the user's synonyms and notes for a subject.
type StudyMaterial struct {
	SubjectID       int       `json:"subject_id"`
	SubjectType     string    `json:"subject_type"`
	MeaningSynonyms []string  `json:"meaning_synonyms"`
	MeaningNote     *string   `json:"meaning_note"`
	ReadingNote     *string   `json:"reading_note"`
	Hidden          bool      `json:"hidden"`
	CreatedAt       time.Time `json:"created_at"`
}

// SpacedRepetitionSystem describes SRS stage intervals.
type SpacedRepetitionSystem struct {
	Name                   string     `json:"name"`
	Description            string     `json:"description"`
	UnlockingStagePosition int        `json:"unlocking_stage_position"`
	StartingStagePosition  int        `json:"starting_stage_position"`
	PassingStagePosition   int        `json:"passing_stage_position"`
	BurningStagePosition   int        `json:"burning_stage_position"`
	Stages                 []SRSStage `json:"stages"`
	CreatedAt              time.Time  `json:"created_at"`
}

// SRSStage is one stage of an SRS. Interval is nil for the unlocking and burning stages.
type SRSStage struct {
	Position     int     `json:"position"`
	Interval     *int64  `json:"interval"`
	IntervalUnit *string `json:"interval_unit"`
}

// ReviewStatistic holds per-subject accuracy.
type ReviewStatistic struct {
	SubjectID            int       `json:"subject_id"`
	SubjectType          string    `json:"subject_type"`
	MeaningCorrect       int       `json:"meaning_correct"`
	MeaningIncorrect     int       `json:"meaning_incorrect"`
	MeaningCurrentStreak int       `json:"meaning_current_streak"`
	MeaningMaxStreak     int       `json:"meaning_max_streak"`
	ReadingCorrect       int       `json:"reading_correct"`
	ReadingIncorrect     int       `json:"reading_incorrect"`
	ReadingCurrentStreak int       `json:"reading_current_streak"`
	ReadingMaxStreak     int       `json:"reading_max_streak"`
	PercentageCorrect    int       `json:"percentage_correct"`
	Hidden               bool      `json:"hidden"`
	CreatedAt            time.Time `json:"created_at"`
}

// Review is the data of a created review.
type Review struct {
	AssignmentID             int       `json:"assignment_id"`
	SubjectID                int       `json:"subject_id"`
	SpacedRepetitionSystemID int       `json:"spaced_repetition_system_id"`
	StartingSRSStage         int       `json:"starting_srs_stage"`
	EndingSRSStage           int       `json:"ending_srs_stage"`
	IncorrectMeaningAnswers  int       `json:"incorrect_meaning_answers"`
	IncorrectReadingAnswers  int       `json:"incorrect_reading_answers"`
	CreatedAt                time.Time `json:"created_at"`
}

// ReviewInput is the body of POST /reviews (wrapped in {"review": ...}).
type ReviewInput struct {
	AssignmentID            int        `json:"assignment_id,omitempty"`
	SubjectID               int        `json:"subject_id,omitempty"`
	IncorrectMeaningAnswers int        `json:"incorrect_meaning_answers"`
	IncorrectReadingAnswers int        `json:"incorrect_reading_answers"`
	CreatedAt               *time.Time `json:"created_at,omitempty"`
}

// ReviewCreated is the response of POST /reviews.
type ReviewCreated struct {
	Resource[Review]
	ResourcesUpdated struct {
		Assignment      Resource[Assignment]      `json:"assignment"`
		ReviewStatistic Resource[ReviewStatistic] `json:"review_statistic"`
	} `json:"resources_updated"`
}
