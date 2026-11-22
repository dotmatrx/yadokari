package answer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dotmatrx/yadokari/internal/wk"
)

func TestTolerance(t *testing.T) {
	// From the reconstruction of WaniKani's table: 0–3 → 0, 4–5 → 1, 6–7 → 2, then 2 + ⌊n/7⌋.
	want := map[int]int{0: 0, 3: 0, 4: 1, 5: 1, 6: 2, 7: 2, 8: 3, 13: 3, 14: 4, 20: 4, 21: 5}
	for n, w := range want {
		if got := Tolerance(n, WaniKaniLike); got != w {
			t.Errorf("Tolerance(%d) = %d, want %d", n, got, w)
		}
	}
	if Tolerance(10, Strict) != 0 || Tolerance(10, Lenient) != 4 || Tolerance(2, Lenient) != 0 {
		t.Error("strictness adjustments")
	}
}

func TestDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"abc", "", 3}, {"", "ab", 2},
		{"crab", "carb", 1},      // adjacent transposition = one edit
		{"kitten", "sitting", 3}, // classic Levenshtein
		{"ground", "grond", 1},
		{"ground", "gruond", 1},
		{"ca", "abc", 3}, // OSA (not full Damerau): no editing a transposed pair
		{"日本", "日木", 1},  // runes, not bytes
	}
	for _, tt := range tests {
		if got := Distance(tt.a, tt.b); got != tt.want {
			t.Errorf("Distance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func str(s string) *string { return &s }

var (
	ichi = wk.Subject{ // the kanji 一
		Characters: str("一"),
		Meanings:   []wk.Meaning{{Meaning: "One", Primary: true, AcceptedAnswer: true}},
		AuxiliaryMeanings: []wk.AuxiliaryMeaning{
			{Meaning: "1", Type: wk.AuxWhitelist},
			{Meaning: "ground", Type: wk.AuxBlacklist},
		},
		Readings: []wk.Reading{
			{Reading: "いち", Primary: true, AcceptedAnswer: true, Type: wk.ReadingOnyomi},
			{Reading: "いつ", AcceptedAnswer: true, Type: wk.ReadingOnyomi},
			{Reading: "ひと", Type: wk.ReadingKunyomi},
		},
	}
	daigaku = wk.Subject{ // the vocabulary 大学
		Characters: str("大学"),
		Meanings: []wk.Meaning{
			{Meaning: "University", Primary: true, AcceptedAnswer: true},
			{Meaning: "College", AcceptedAnswer: true},
		},
		AuxiliaryMeanings: []wk.AuxiliaryMeaning{{Meaning: "uni", Type: wk.AuxWhitelist}, {Meaning: "school", Type: wk.AuxBlacklist}},
		Readings:          []wk.Reading{{Reading: "だいがく", Primary: true, AcceptedAnswer: true}},
	}
)

func TestCheckMeaning(t *testing.T) {
	tests := []struct {
		name     string
		subject  wk.Subject
		input    string
		synonyms []string
		verdict  Verdict
		inexact  bool
		msg      string
	}{
		{"exact, any case and spacing", daigaku, "  uNiversity ", nil, Correct, false, ""},
		{"secondary meaning", daigaku, "college", nil, Correct, false, ""},
		{"whitelist", daigaku, "uni", nil, Correct, false, ""},
		{"blacklist", daigaku, "school", nil, Incorrect, false, ""},
		{"user synonym", daigaku, "academy", []string{"Academy"}, Correct, false, ""},
		{"typo within tolerance", daigaku, "univeristy", nil, Correct, true, MsgInexact},
		{"two typos on a 10-letter word", daigaku, "unversty", nil, Correct, true, MsgInexact},
		{"too far", daigaku, "unicorn", nil, Incorrect, false, ""},
		{"typo on a synonym", daigaku, "acadmy", []string{"academy"}, Correct, true, MsgInexact},
		{"short words need exact", ichi, "onw", nil, Incorrect, false, ""},
		{"digit whitelist exact", ichi, "1", nil, Correct, false, ""},
		{"digits never fuzzy", ichi, "7", nil, Incorrect, false, ""},
		{"blacklist beats closeness", ichi, "ground", nil, Incorrect, false, ""},
		{"kana in meaning mode", daigaku, "だいがく", nil, Retry, false, MsgWantMeaning},
		{"reading typed in romaji", daigaku, "daigaku", nil, Retry, false, MsgWantMeaning},
		{"kunyomi in romaji", ichi, "hito", nil, Retry, false, MsgWantMeaning},
		{"empty", daigaku, "   ", nil, Retry, false, MsgEmptyAnswer},
		{"curly apostrophe", wk.Subject{Meanings: []wk.Meaning{{Meaning: "One's Own", AcceptedAnswer: true}}}, "one’s own", nil, Correct, false, ""},
		{"unaccepted meaning", wk.Subject{Meanings: []wk.Meaning{{Meaning: "Old", AcceptedAnswer: false}, {Meaning: "New", AcceptedAnswer: true}}}, "old", nil, Incorrect, false, ""},
	}
	for _, tt := range tests {
		r := CheckMeaning(tt.input, tt.subject, tt.synonyms, WaniKaniLike)
		if r.Verdict != tt.verdict || r.Inexact != tt.inexact || r.Message != tt.msg {
			t.Errorf("%s: %q → %+v, want %v inexact=%v msg=%q", tt.name, tt.input, r, tt.verdict, tt.inexact, tt.msg)
		}
	}

	if r := CheckMeaning("univeristy", daigaku, nil, Strict); r.Verdict != Incorrect {
		t.Errorf("strict should reject typos: %+v", r)
	}
}

func TestCheckReading(t *testing.T) {
	tests := []struct {
		name    string
		object  string
		subject wk.Subject
		input   string
		verdict Verdict
		msg     string
	}{
		{"primary on'yomi", wk.ObjectKanji, ichi, "いち", Correct, ""},
		{"other accepted", wk.ObjectKanji, ichi, "いつ", Correct, ""},
		{"katakana is normalized", wk.ObjectKanji, ichi, "イチ", Correct, ""},
		{"kun'yomi when on'yomi wanted", wk.ObjectKanji, ichi, "ひと", Retry, MsgWantOnyomi},
		{"wrong", wk.ObjectKanji, ichi, "に", Incorrect, ""},
		{"leftover romaji", wk.ObjectKanji, ichi, "いk", Retry, MsgInvalidKana},
		{"meaning in reading mode", wk.ObjectVocabulary, daigaku, "university", Retry, MsgWantReading},
		{"vocab exact", wk.ObjectVocabulary, daigaku, "だいがく", Correct, ""},
		{"vocab wrong", wk.ObjectVocabulary, daigaku, "だいかく", Incorrect, ""},
		{"empty", wk.ObjectVocabulary, daigaku, "", Retry, MsgEmptyAnswer},
	}
	for _, tt := range tests {
		r := CheckReading(tt.input, tt.object, tt.subject)
		if r.Verdict != tt.verdict || r.Message != tt.msg {
			t.Errorf("%s: %q → %+v, want %v %q", tt.name, tt.input, r, tt.verdict, tt.msg)
		}
	}

	kun := wk.Subject{Readings: []wk.Reading{
		{Reading: "き", Primary: true, AcceptedAnswer: true, Type: wk.ReadingKunyomi},
		{Reading: "もく", Type: wk.ReadingOnyomi},
	}}
	if r := CheckReading("もく", wk.ObjectKanji, kun); r.Message != MsgWantKunyomi {
		t.Errorf("on'yomi when kun'yomi wanted: %+v", r)
	}
}

// TestRecordedSubjects checks every subject in a local recording: each
// accepted meaning and reading passes exactly, and each blacklisted meaning
// is rejected. Skipped without a recording.
func TestRecordedSubjects(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/recordings/*/subjects.json")
	if len(files) == 0 {
		t.Skip("no recorded subjects (run `just probe-live`)")
	}
	raw, err := os.ReadFile(files[len(files)-1])
	if err != nil {
		t.Fatal(err)
	}
	var subjects []wk.Resource[wk.Subject]
	if err := json.Unmarshal(raw, &subjects); err != nil {
		t.Fatal(err)
	}
	var meanings, readings, blocked, failures int
	fail := func(format string, args ...any) {
		failures++
		if failures <= 15 {
			t.Errorf(format, args...)
		}
	}
	for _, s := range subjects {
		for _, m := range s.Data.Meanings {
			if !m.AcceptedAnswer {
				continue
			}
			meanings++
			if r := CheckMeaning(m.Meaning, s.Data, nil, WaniKaniLike); r.Verdict != Correct || r.Inexact {
				fail("%s %d %q: meaning %q → %+v", s.Object, s.ID, deref(s.Data.Characters), m.Meaning, r)
			}
		}
		for _, aux := range s.Data.AuxiliaryMeanings {
			if aux.Type != wk.AuxBlacklist {
				continue
			}
			blocked++
			if r := CheckMeaning(aux.Meaning, s.Data, nil, WaniKaniLike); r.Verdict == Correct {
				fail("%s %d %q: blacklisted %q accepted", s.Object, s.ID, deref(s.Data.Characters), aux.Meaning)
			}
		}
		for _, rd := range s.Data.Readings {
			if !rd.AcceptedAnswer {
				continue
			}
			readings++
			if r := CheckReading(rd.Reading, s.Object, s.Data); r.Verdict != Correct {
				fail("%s %d %q: reading %q → %+v", s.Object, s.ID, deref(s.Data.Characters), rd.Reading, r)
			}
		}
	}
	t.Logf("checked %d meanings, %d readings, %d blacklisted answers: %d failures", meanings, readings, blocked, failures)
}

func deref(s *string) string {
	if s == nil {
		return "(image)"
	}
	return *s
}
