package markup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Paragraph
	}{
		{"empty", "", nil},
		{"plain", "Just words.", []Paragraph{{{Text: "Just words."}}}},
		{"types", "The <radical>ground</radical> and <kanji>one</kanji> make <vocabulary>two</vocabulary>.",
			[]Paragraph{{
				{Text: "The "}, {Text: "ground", Kind: Radical}, {Text: " and "},
				{Text: "one", Kind: Kanji}, {Text: " make "}, {Text: "two", Kind: Vocabulary}, {Text: "."},
			}}},
		{"reading and ja", "Say <reading>いち</reading> like <ja>一つ</ja>, <meaning>one</meaning>.",
			[]Paragraph{{
				{Text: "Say "}, {Text: "いち", Kind: Reading}, {Text: " like "},
				{Text: "一つ", Kind: Japanese}, {Text: ", "}, {Text: "one", Kind: Meaning}, {Text: "."},
			}}},
		{"emphasis", "<em>Really</em> <i>so</i>", []Paragraph{{
			{Text: "Really", Kind: Emphasis, Emph: true}, {Text: " "}, {Text: "so", Kind: Emphasis, Emph: true},
		}}},
		{"emphasis inside a type keeps the type", "<kanji>a <em>b</em></kanji>", []Paragraph{{
			{Text: "a ", Kind: Kanji}, {Text: "b", Kind: Kanji, Emph: true},
		}}},
		{"type inside emphasis", "<em>a <kanji>b</kanji></em>", []Paragraph{{
			{Text: "a ", Kind: Emphasis, Emph: true}, {Text: "b", Kind: Kanji, Emph: true},
		}}},
		{"link", `See <a href="http://example.com/x?a=1&amp;b=2" target="_blank">this</a>.`, []Paragraph{{
			{Text: "See "}, {Text: "this", Kind: Link, URL: "http://example.com/x?a=1&b=2"}, {Text: "."},
		}}},
		{"mismatched close closes the innermost tag", "a <reading>car</kanji> (か).", []Paragraph{{
			{Text: "a "}, {Text: "car", Kind: Reading}, {Text: " (か)."},
		}}},
		{"stray close is ignored", "a</kanji> b", []Paragraph{{{Text: "a b"}}}},
		{"unclosed tag ends with the paragraph", "<kanji>a\nb", []Paragraph{
			{{Text: "a", Kind: Kanji}}, {{Text: "b"}},
		}},
		{"unknown tags dropped, text kept", "<span class=x>a</span> <b>b</b>", []Paragraph{{
			{Text: "a "}, {Text: "b", Kind: Emphasis, Emph: true},
		}}},
		{"unknown tag inside a type", "<kanji><span>a</span></kanji>", []Paragraph{{{Text: "a", Kind: Kanji}}}},
		{"self-closing", "a<br/>b<hr />c", []Paragraph{{{Text: "abc"}}}},
		{"entities", "Tom &amp; Jerry &quot;x&quot; 1 &lt; 2", []Paragraph{{{Text: `Tom & Jerry "x" 1 < 2`}}}},
		{"paragraphs, blank lines dropped", "one\r\n\r\ntwo\n  \nthree", []Paragraph{
			{{Text: "one"}}, {{Text: "two"}}, {{Text: "three"}},
		}},
		{"a lone < is text", "1 < 2 and <3", []Paragraph{{{Text: "1 < 2 and <3"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Parse(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q)\n got %#v\nwant %#v", tt.in, got, tt.want)
			}
		})
	}
}

// TestRealMnemonics parses every mnemonic and hint in a recorded fixture, if one
// exists locally (recordings are gitignored), and checks no tag leaks into the text.
func TestRealMnemonics(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/recordings/*/subjects.json")
	if len(files) == 0 {
		t.Skip("no recorded subjects (run `just probe-live`)")
	}
	raw, err := os.ReadFile(files[len(files)-1])
	if err != nil {
		t.Fatal(err)
	}
	var subjects []struct {
		ID   int `json:"id"`
		Data struct {
			MeaningMnemonic string `json:"meaning_mnemonic"`
			ReadingMnemonic string `json:"reading_mnemonic"`
			MeaningHint     string `json:"meaning_hint"`
			ReadingHint     string `json:"reading_hint"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &subjects); err != nil {
		t.Fatal(err)
	}
	n, bad := 0, 0
	for _, s := range subjects {
		for _, src := range []string{s.Data.MeaningMnemonic, s.Data.ReadingMnemonic, s.Data.MeaningHint, s.Data.ReadingHint} {
			if src == "" {
				continue
			}
			n++
			for _, p := range Parse(src) {
				if txt := p.Text(); tagRe.MatchString(txt) || strings.Contains(txt, "&amp;") {
					bad++
					if bad <= 10 {
						t.Errorf("subject %d: markup left in %q", s.ID, txt)
					}
				}
			}
		}
	}
	t.Logf("parsed %d texts", n)
}
