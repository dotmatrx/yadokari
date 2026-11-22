// Package answer checks review answers the way WaniKani does: meanings with
// typo tolerance, user synonyms and WaniKani's allow/block lists; readings
// exactly, after kana normalization. Some mistakes are warnings (the answer
// is shaken back for another try and not counted), e.g. kana typed for a
// meaning, or the kun'yomi when WaniKani wants the on'yomi.
//
// The typo tolerance follows the community reconstruction in RadicalGuessr
// (https://github.com/romans-boi/wk-radical-guessr, MIT), whose author says it
// was taken from WaniKani's own levenshtein_helpers. It is unverified against
// WaniKani itself, so a Strictness setting is offered.
package answer

import (
	"strings"
	"unicode"

	"github.com/dotmatrx/yadokari/internal/ime"
	"github.com/dotmatrx/yadokari/internal/wk"
)

// Verdict is the outcome of a check.
type Verdict int

const (
	Incorrect Verdict = iota
	Correct
	Retry // a warning: not counted, the user tries again
)

func (v Verdict) String() string {
	return [...]string{"incorrect", "correct", "retry"}[v]
}

// Result describes a checked answer.
type Result struct {
	Verdict Verdict
	// Inexact is set when a meaning was accepted only through typo tolerance.
	Inexact bool
	// Matched is the accepted answer that matched, if any.
	Matched string
	// Message explains a warning or an inexact match; empty otherwise.
	Message string
}

// Strictness controls meaning typo tolerance.
type Strictness int

const (
	WaniKaniLike Strictness = iota // the default
	Strict                         // exact matches only
	Lenient                        // one more typo than WaniKani allows
)

// Messages shown to the user (WaniKani's wording where known).
const (
	MsgInexact      = "Your answer was a bit off. Check the meaning to make sure you are correct."
	MsgWantMeaning  = "We want the meaning, not the reading."
	MsgWantReading  = "We want the reading, not the meaning."
	MsgInvalidKana  = "That doesn't look like kana. Check your answer."
	MsgWantOnyomi   = "WaniKani is looking for the on'yomi reading."
	MsgWantKunyomi  = "WaniKani is looking for the kun'yomi reading."
	MsgWantNanori   = "WaniKani is looking for the nanori reading."
	MsgEmptyAnswer  = "Type an answer first."
	MsgReadingTypes = "WaniKani is looking for a different reading."
)

// NormalizeMeaning lowercases, trims, collapses whitespace and unifies
// apostrophes and hyphens.
func NormalizeMeaning(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("’", "'", "‘", "'", "‐", "-", "‑", "-", "–", "-", "—", "-").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// Tolerance is the number of typos accepted for an answer of n characters:
// 0–3 → 0, 4–5 → 1, 6–7 → 2, then 2 + ⌊n/7⌋.
func Tolerance(n int, strictness Strictness) int {
	var t int
	switch {
	case n <= 3:
		t = 0
	case n <= 5:
		t = 1
	case n <= 7:
		t = 2
	default:
		t = 2 + n/7
	}
	switch strictness {
	case Strict:
		return 0
	case Lenient:
		if n > 2 {
			return t + 1
		}
	}
	return t
}

// Distance is the optimal-string-alignment distance: Levenshtein plus
// transposition of adjacent characters as one edit.
func Distance(a, b string) int {
	s, t := []rune(a), []rune(b)
	n, m := len(s), len(t)
	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}
	d := make([][]int, n+1)
	for i := range d {
		d[i] = make([]int, m+1)
		d[i][0] = i
	}
	for j := 0; j <= m; j++ {
		d[0][j] = j
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && s[i-1] == t[j-2] && s[i-2] == t[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[n][m]
}

func hasDigit(s string) bool { return strings.IndexFunc(s, unicode.IsDigit) >= 0 }

// CheckMeaning checks a meaning answer for subject (data from the API) with
// the user's synonyms.
func CheckMeaning(input string, subject wk.Subject, synonyms []string, strictness Strictness) Result {
	answer := NormalizeMeaning(input)
	if answer == "" {
		return Result{Verdict: Retry, Message: MsgEmptyAnswer}
	}

	// Kana typed in meaning mode: a warning.
	if ime.IsKana(answer) {
		return Result{Verdict: Retry, Message: MsgWantMeaning}
	}

	// The block list wins over everything, and is never fuzzy-matched.
	for _, aux := range subject.AuxiliaryMeanings {
		if aux.Type == wk.AuxBlacklist && NormalizeMeaning(aux.Meaning) == answer {
			return Result{Verdict: Incorrect}
		}
	}

	var accepted []string
	for _, m := range subject.Meanings {
		if m.AcceptedAnswer {
			accepted = append(accepted, m.Meaning)
		}
	}
	for _, aux := range subject.AuxiliaryMeanings {
		if aux.Type == wk.AuxWhitelist {
			accepted = append(accepted, aux.Meaning)
		}
	}
	accepted = append(accepted, synonyms...)

	for _, a := range accepted {
		if NormalizeMeaning(a) == answer {
			return Result{Verdict: Correct, Matched: a}
		}
	}

	// The reading typed in romaji ("ichi" for 一): a warning. Checked after
	// exact matches so a real meaning that happens to spell a reading wins.
	if matchesReading(answer, subject) {
		return Result{Verdict: Retry, Message: MsgWantMeaning}
	}

	// Typo tolerance, never for answers involving digits ("1" vs "7").
	tol := Tolerance(len([]rune(answer)), strictness)
	best, bestDist := "", tol+1
	for _, a := range accepted {
		na := NormalizeMeaning(a)
		if hasDigit(na) || hasDigit(answer) {
			continue
		}
		if d := Distance(answer, na); d < bestDist {
			best, bestDist = a, d
		}
	}
	if best != "" {
		return Result{Verdict: Correct, Inexact: true, Matched: best, Message: MsgInexact}
	}
	return Result{Verdict: Incorrect}
}

// matchesReading reports whether a meaning answer is actually the subject's
// reading typed in romaji (e.g. "ichi" for 一).
func matchesReading(answer string, subject wk.Subject) bool {
	if len(subject.Readings) == 0 || strings.ContainsAny(answer, " ") {
		return false
	}
	kana := ime.ToHiragana(ime.ToKana(answer))
	if !ime.IsKana(kana) {
		return false
	}
	for _, r := range subject.Readings {
		if ime.ToHiragana(r.Reading) == kana {
			return true
		}
	}
	return false
}

// CheckReading checks a reading answer. objectType is the subject's API
// object ("kanji", "vocabulary"); input is the IME output.
func CheckReading(input string, objectType string, subject wk.Subject) Result {
	answer := strings.TrimSpace(input)
	if answer == "" {
		return Result{Verdict: Retry, Message: MsgEmptyAnswer}
	}
	if !ime.IsKana(answer) {
		// Leftover romaji, or English typed while the converter was off.
		if isMeaning(answer, subject) {
			return Result{Verdict: Retry, Message: MsgWantReading}
		}
		return Result{Verdict: Retry, Message: MsgInvalidKana}
	}
	hira := ime.ToHiragana(answer)

	var other *wk.Reading
	for i, r := range subject.Readings {
		if ime.ToHiragana(r.Reading) != hira {
			continue
		}
		if r.AcceptedAnswer {
			return Result{Verdict: Correct, Matched: r.Reading}
		}
		other = &subject.Readings[i]
	}

	// A kanji's real but unaccepted reading (usually the kun'yomi when the
	// on'yomi is taught): warn instead of marking wrong.
	if other != nil && objectType == wk.ObjectKanji {
		switch wanted(subject) {
		case wk.ReadingOnyomi:
			return Result{Verdict: Retry, Message: MsgWantOnyomi}
		case wk.ReadingKunyomi:
			return Result{Verdict: Retry, Message: MsgWantKunyomi}
		case wk.ReadingNanori:
			return Result{Verdict: Retry, Message: MsgWantNanori}
		}
		return Result{Verdict: Retry, Message: MsgReadingTypes}
	}
	return Result{Verdict: Incorrect}
}

// wanted is the reading type WaniKani teaches for a kanji: the type of its
// primary accepted reading.
func wanted(subject wk.Subject) string {
	for _, r := range subject.Readings {
		if r.Primary && r.AcceptedAnswer {
			return r.Type
		}
	}
	for _, r := range subject.Readings {
		if r.AcceptedAnswer {
			return r.Type
		}
	}
	return ""
}

func isMeaning(answer string, subject wk.Subject) bool {
	a := NormalizeMeaning(answer)
	for _, m := range subject.Meanings {
		if NormalizeMeaning(m.Meaning) == a {
			return true
		}
	}
	return false
}
