package ime

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// type feeds keys; '\b' is backspace.
func typeKeys(keys string) *Converter {
	var c Converter
	for _, r := range keys {
		if r == '\b' {
			c.Backspace()
		} else {
			c.Type(r)
		}
	}
	return &c
}

func TestIncremental(t *testing.T) {
	tests := []struct {
		keys            string
		committed, pend string
	}{
		{"k", "", "k"},
		{"ky", "", "ky"},
		{"kyo", "きょ", ""},
		{"kyouiku", "きょういく", ""},
		{"gakkou", "がっこう", ""},
		{"gakk", "がっ", "k"},
		{"shinbun", "しんぶ", "n"},
		{"kon'nichiha", "こんにちは", ""},
		{"konnnichiha", "こんにちは", ""},
		{"konnichiha", "こんいちは", ""}, // WaniKani/WanaKana IME behaviour: nn → ん
		{"n", "", "n"},
		{"na", "な", ""},
		{"ny", "", "ny"},
		{"nya", "にゃ", ""},
		{"nk", "ん", "k"},
		{"ts", "", "ts"},
		{"tsu", "つ", ""},
		{"matcha", "まっちゃ", ""},
		{"itchi", "いっち", ""},
		{"tc", "", "tc"},
		{"xtu", "っ", ""},
		{"lya", "ゃ", ""},
		{"ra-men", "らーめ", "n"},
		{"KATAKANA", "カタカナ", ""},
		{"Kyou", "キョう", ""},
		{"shi", "し", ""},
		{"si", "し", ""},
		{"chi", "ち", ""},
		{"ti", "ち", ""},
		{"fu", "ふ", ""},
		{"hu", "ふ", ""},
		{"ji", "じ", ""},
		{"zi", "じ", ""},
		{"ja", "じゃ", ""},
		{"jya", "じゃ", ""},
		{"zya", "じゃ", ""},
		{"wo", "を", ""},
		{"dji", "ぢ", ""},
		{"di", "ぢ", ""},
		{"du", "づ", ""},
		{"q", "q", ""},
		{"kq", "kq", ""},
		{"ka1", "か1", ""},
		{"に", "に", ""},
		{"nに", "んに", ""},
	}
	for _, tt := range tests {
		c := typeKeys(tt.keys)
		if c.Committed() != tt.committed || c.Pending() != tt.pend {
			t.Errorf("%q: got %q + pending %q, want %q + %q", tt.keys, c.Committed(), c.Pending(), tt.committed, tt.pend)
		}
	}
}

func TestRelaxedN(t *testing.T) {
	for _, tt := range []struct {
		keys            string
		committed, pend string // before Finish
		finished        string
	}{
		{"konnichiha", "こんにちは", "", "こんにちは"},
		{"konnnichiha", "こんにちは", "", "こんにちは"}, // WaniKani's spelling still works
		{"kon'nichiha", "こんにちは", "", "こんにちは"},
		{"konnyaku", "こんにゃく", "", "こんにゃく"},
		{"konn", "こ", "nn", "こん"}, // waits: a vowel would make it ん + な-row
		{"sannkaku", "さんかく", "", "さんかく"},
		{"honn", "ほ", "nn", "ほん"},
		{"kanni", "かんに", "", "かんに"}, // the trade-off: ん + vowel is…
		{"kan'i", "かんい", "", "かんい"}, // …typed with an apostrophe
		{"kanji", "かんじ", "", "かんじ"},
	} {
		c := Converter{RelaxedN: true}
		c.TypeString(tt.keys)
		if c.Committed() != tt.committed || c.Pending() != tt.pend {
			t.Errorf("%q: %q + %q, want %q + %q", tt.keys, c.Committed(), c.Pending(), tt.committed, tt.pend)
		}
		if got := c.Finish(); got != tt.finished {
			t.Errorf("%q finished: %q, want %q", tt.keys, got, tt.finished)
		}
	}
	// Backspace in the middle of "nn" leaves one n to type on.
	c := Converter{RelaxedN: true}
	c.TypeString("konn")
	c.Backspace()
	c.TypeString("i")
	if got := c.Finish(); got != "こに" {
		t.Errorf("konn⌫i: %q, want こに", got)
	}
	// Reset keeps the setting.
	c.Reset()
	c.TypeString("konnichiha")
	if got := c.Finish(); got != "こんにちは" {
		t.Errorf("after Reset: %q", got)
	}
}

func TestOnnaAndMatcha(t *testing.T) {
	if got := ToKana("onna"); got != "おんあ" {
		t.Errorf("onna → %q, want おんあ (WK behaviour; type onnna for おんな)", got)
	}
	if got := ToKana("onnna"); got != "おんな" {
		t.Errorf("onnna → %q", got)
	}
	if got := ToKana("tc"); got != "tc" {
		t.Errorf("tc → %q", got)
	}
	if got := ToKana("maccha"); got != "まっちゃ" {
		t.Errorf("maccha → %q", got)
	}
}

func TestBackspace(t *testing.T) {
	tests := []struct{ keys, want string }{
		{"ky\b", "k"},         // pending letter
		{"ky\b\b", ""},        //
		{"kyo\b", "き"},        // whole kana: きょ is two runes, ょ removed
		{"kyo\b\b", ""},       //
		{"gakkou\b\b", "がっ"},  //
		{"shinbun\b", "しんぶ"},  // pending n
		{"shinbun\b\b", "しん"}, //
		{"ka\bi", "い"},        //
		{"n\bka", "か"},        //
		{"\b\bka", "か"},       // backspace on empty is a no-op
		{"kos\bnn", "こん"},     //
		// A letter left behind by a typo still combines with what follows.
		{"ks\ba", "か"},    //
		{"kos\bka", "こか"}, //
		{"kyz\bo", "きょ"},  //
		{"sx\bhi", "し"},   //
		{"Ks\ba", "カ"},    //
	}
	for _, tt := range tests {
		if got := typeKeys(tt.keys).String(); got != tt.want {
			t.Errorf("%q: got %q, want %q", strings.ReplaceAll(tt.keys, "\b", "⌫"), got, tt.want)
		}
	}
}

func TestFinish(t *testing.T) {
	tests := []struct{ in, want string }{
		{"shinbun", "しんぶん"},
		{"hon", "ほん"},
		{"ky", "ky"},
		{"kak", "かk"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := ToKana(tt.in); got != tt.want {
			t.Errorf("ToKana(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestKanaHelpers(t *testing.T) {
	if got := ToHiragana("カタカナー・ヴ"); got != "かたかなー・ゔ" {
		t.Errorf("ToHiragana: %q", got)
	}
	if got := ToKatakana("ひらがなゔ"); got != "ヒラガナヴ" {
		t.Errorf("ToKatakana: %q", got)
	}
	for s, want := range map[string]bool{"きょう": true, "ラーメン": true, "": false, "kyo": false, "今日": false, "きょうk": false} {
		if IsKana(s) != want {
			t.Errorf("IsKana(%q) = %v", s, !want)
		}
	}
}

// toRomaji produces a keystroke spelling that the converter should map back
// to exactly kana: canonical spellings, "nn" for ん, doubled consonant or
// "xtu" for っ. Non-kana runes pass through.
func toRomaji(kana string) string {
	rev := map[string]string{}
	for _, k := range order {
		if _, ok := rev[table[k]]; !ok {
			rev[table[k]] = k
		}
	}
	r := []rune(kana)
	var parts []string
	for i := 0; i < len(r); {
		if r[i] == 'ん' {
			parts = append(parts, "nn")
			i++
			continue
		}
		if r[i] == 'っ' {
			parts = append(parts, "\x00") // fixed up below
			i++
			continue
		}
		if i+1 < len(r) {
			if s, ok := rev[string(r[i:i+2])]; ok {
				parts = append(parts, s)
				i += 2
				continue
			}
		}
		if s, ok := rev[string(r[i])]; ok {
			parts = append(parts, s)
		} else {
			parts = append(parts, string(r[i]))
		}
		i++
	}
	for i, p := range parts {
		if p != "\x00" {
			continue
		}
		parts[i] = "xtu"
		if i+1 < len(parts) && len(parts[i+1]) > 0 {
			if b := parts[i+1][0]; isConsonant(b) && b != 'n' {
				parts[i] = string(b)
			}
		}
	}
	return strings.Join(parts, "")
}

func TestRoundTripTable(t *testing.T) {
	for _, k := range order {
		kana := table[k]
		if got := ToKana(k); got != kana {
			t.Errorf("ToKana(%q) = %q, want %q", k, got, kana)
		}
	}
}

func TestRoundTripRandom(t *testing.T) {
	var kana []string
	seen := map[string]bool{}
	for _, k := range order {
		if !seen[table[k]] {
			seen[table[k]] = true
			kana = append(kana, table[k])
		}
	}
	kana = append(kana, "ん", "っ")
	rng := rand.New(rand.NewPCG(1, 2))
	for range 5000 {
		var b strings.Builder
		for range 1 + rng.IntN(8) {
			b.WriteString(kana[rng.IntN(len(kana))])
		}
		want := b.String()
		romaji := toRomaji(want)
		if got := ToKana(romaji); got != want {
			t.Fatalf("%q → %q → %q", want, romaji, got)
		}
	}
}

// TestRealReadings round-trips every reading in a recorded fixture, if one
// exists locally (recordings are gitignored).
func TestRealReadings(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/recordings/*/subjects.json")
	if len(files) == 0 {
		t.Skip("no recorded subjects (run `just probe-live`)")
	}
	raw, err := os.ReadFile(files[len(files)-1])
	if err != nil {
		t.Fatal(err)
	}
	var subjects []struct {
		Data struct {
			Readings []struct {
				Reading string `json:"reading"`
			} `json:"readings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &subjects); err != nil {
		t.Fatal(err)
	}
	n, failed := 0, 0
	for _, s := range subjects {
		for _, r := range s.Data.Readings {
			n++
			romaji := toRomaji(r.Reading)
			if got := ToKana(romaji); got != r.Reading {
				failed++
				if failed <= 20 {
					t.Errorf("%q → %q → %q", r.Reading, romaji, got)
				}
			}
		}
	}
	t.Logf("round-tripped %d readings, %d failed", n, failed)
}
