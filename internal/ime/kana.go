package ime

// Hiragana ぁ (U+3041) … ゖ (U+3096) map to katakana by adding 0x60.
const (
	hiraFirst = 'ぁ'
	hiraLast  = 'ゖ'
	kataFirst = 'ァ'
	kataLast  = 'ヶ'
	kataShift = 'ァ' - 'ぁ'
)

// ToKatakana converts hiragana to katakana, leaving everything else alone.
func ToKatakana(s string) string {
	r := []rune(s)
	for i, c := range r {
		if c >= hiraFirst && c <= hiraLast {
			r[i] = c + kataShift
		}
	}
	return string(r)
}

// ToHiragana converts katakana to hiragana, leaving everything else (including
// ー) alone. Answer checking normalizes readings with this.
func ToHiragana(s string) string {
	r := []rune(s)
	for i, c := range r {
		if c >= kataFirst && c <= kataLast {
			r[i] = c - kataShift
		}
	}
	return string(r)
}

// IsKana reports whether s is non-empty and only hiragana, katakana and ー.
func IsKana(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= hiraFirst && c <= hiraLast) && !(c >= kataFirst && c <= kataLast) && c != 'ー' {
			return false
		}
	}
	return true
}
