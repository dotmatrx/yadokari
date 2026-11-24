// Package ime is an incremental romaji → kana converter that behaves like
// WanaKana's IME mode (which WaniKani's site uses): kana appear as you type,
// unresolved romaji stays visible as "pending", and backspace removes the last
// pending letter or, if none, the last whole kana.
//
// Rules worth knowing:
//   - "nn" and "n'" produce ん immediately, so こんにちは is typed
//     "kon'nichiha" or "konnnichiha" (as on WaniKani). n before a consonant
//     other than y also becomes ん; a trailing n becomes ん on Finish.
//     With RelaxedN, "nn" before a vowel or y is ん + a na-row syllable, so
//     "konnichiha" is こんにちは too; ん + vowel is then typed "n'" (kan'i).
//   - A doubled consonant (kk, tt, pp…, not nn) or Hepburn "tch" produces っ.
//   - x/l prefixes give small kana (xtu, lya); "-" gives ー.
//   - Romaji typed with a leading uppercase letter produces katakana.
//   - Any non-ASCII rune (e.g. from a system IME or paste) is inserted as is.
//
// The package does no I/O and has no UI dependencies.
package ime

import (
	"strings"
	"unicode"
)

// Converter holds the state of one input line.
type Converter struct {
	// RelaxedN reads "nn" before a vowel or y as ん + na-row: konnichiha →
	// こんにちは, not こんいちは. Off is WaniKani's behaviour.
	RelaxedN bool

	committed []rune
	pending   []rune // ASCII, original case
}

// Type feeds one keystroke.
func (c *Converter) Type(r rune) {
	if r > unicode.MaxASCII {
		c.resolve(true)
		c.committed = append(c.committed, r)
		return
	}
	c.pending = append(c.pending, r)
	c.resolve(false)
}

// TypeString feeds each rune of s.
func (c *Converter) TypeString(s string) {
	for _, r := range s {
		c.Type(r)
	}
}

// Backspace deletes the last pending letter, or else the last kana. Letters
// that were kept as typed because they couldn't start any kana (the k of a
// mistyped "ks") go back to pending, so after "ks⌫" an "a" still gives か.
func (c *Converter) Backspace() {
	switch {
	case len(c.pending) > 0:
		c.pending = c.pending[:len(c.pending)-1]
	case len(c.committed) > 0:
		c.committed = c.committed[:len(c.committed)-1]
	default:
		return
	}
	i := len(c.committed)
	for i > 0 && isASCIILetter(c.committed[i-1]) {
		i--
	}
	if i == len(c.committed) {
		return
	}
	c.pending = append(append([]rune{}, c.committed[i:]...), c.pending...)
	c.committed = c.committed[:i]
	c.resolve(false)
}

func isASCIILetter(r rune) bool { return r < unicode.MaxASCII && unicode.IsLetter(r) }

// Committed is the converted text.
func (c *Converter) Committed() string { return string(c.committed) }

// Pending is the unresolved romaji, shown after the committed text.
func (c *Converter) Pending() string { return string(c.pending) }

// String is what the input line displays: committed + pending.
func (c *Converter) String() string { return string(c.committed) + string(c.pending) }

// Finish resolves everything that can be (a trailing n becomes ん), leaves
// unresolvable letters as romaji, and returns the full text.
func (c *Converter) Finish() string {
	c.resolve(true)
	return c.String()
}

// Reset clears the line.
func (c *Converter) Reset() { c.committed, c.pending = nil, nil }

// ToKana converts a whole romaji string, as if typed and then finished.
func ToKana(s string) string {
	var c Converter
	c.TypeString(s)
	return c.Finish()
}

func isVowel(b byte) bool { return strings.IndexByte("aiueo", b) >= 0 }

func isConsonant(b byte) bool { return b >= 'a' && b <= 'z' && !isVowel(b) }

// resolve converts as much of pending as is unambiguous. When final is true,
// nothing may wait for more input.
func (c *Converter) resolve(final bool) {
	for len(c.pending) > 0 {
		p := c.pending
		lower := strings.ToLower(string(p))
		katakana := unicode.IsUpper(p[0])

		if lower[0] == 'n' {
			if len(lower) == 1 {
				if !final {
					return
				}
				c.emit("ん", katakana, 1)
				continue
			}
			switch n1 := lower[1]; {
			case n1 == 'n' && c.RelaxedN:
				// ん, and the second n starts the next syllable if a vowel
				// or y follows: wait to see which.
				if len(lower) == 2 {
					if !final {
						return
					}
					c.emit("ん", katakana, 2)
					continue
				}
				if n2 := lower[2]; isVowel(n2) || n2 == 'y' {
					c.emit("ん", katakana, 1)
				} else {
					c.emit("ん", katakana, 2)
				}
				continue
			case n1 == 'n' || n1 == '\'':
				c.emit("ん", katakana, 2)
				continue
			case !isVowel(n1) && n1 != 'y':
				c.emit("ん", katakana, 1)
				continue
			}
		}

		if len(lower) >= 2 && lower[0] == lower[1] && isConsonant(lower[0]) {
			c.emit("っ", katakana, 1)
			continue
		}

		// Hepburn "tch" (matcha, itchi) is also っ + ch.
		if strings.HasPrefix(lower, "tc") {
			if len(lower) == 2 && !final {
				return
			}
			if strings.HasPrefix(lower, "tch") {
				c.emit("っ", katakana, 1)
				continue
			}
		}

		if !final && prefixes[lower] {
			return // could still become a longer key
		}

		matched := false
		for n := min(maxKeyLen, len(lower)); n > 0; n-- {
			if kana, ok := table[lower[:n]]; ok {
				c.emit(kana, katakana, n)
				matched = true
				break
			}
		}
		if !matched {
			// No key can ever start here: keep the letter as typed.
			c.committed = append(c.committed, p[0])
			c.pending = p[1:]
		}
	}
}

func (c *Converter) emit(kana string, katakana bool, consumed int) {
	if katakana {
		kana = ToKatakana(kana)
	}
	c.committed = append(c.committed, []rune(kana)...)
	c.pending = c.pending[consumed:]
}
