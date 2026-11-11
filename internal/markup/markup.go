// Package markup parses WaniKani's mnemonic and hint markup into styled spans.
//
// The markup is a small HTML-like language: <radical>, <kanji>, <vocabulary>,
// <reading>, <ja>, <meaning>, <em>/<i>/<strong>/<b>, and <a href="…">. Newlines
// separate paragraphs. Real data is not always well-formed (one recorded
// mnemonic reads "<reading>car</kanji>"), so a closing tag closes the innermost
// open tag whatever its name, unknown tags are dropped with their text kept, and
// unclosed tags end with the paragraph. Parse never fails.
//
// Pure: no I/O, no UI.
package markup

import (
	"html"
	"regexp"
	"strings"
)

// Kind is what a span of text marks.
type Kind int

const (
	Plain Kind = iota
	Radical
	Kanji
	Vocabulary
	Reading
	Meaning
	Japanese
	Emphasis
	Link
	unknown // an unrecognized tag: tracked for nesting, renders as its parent
)

var kinds = map[string]Kind{
	"radical": Radical, "kanji": Kanji, "vocabulary": Vocabulary,
	"reading": Reading, "meaning": Meaning, "ja": Japanese,
	"em": Emphasis, "i": Emphasis, "strong": Emphasis, "b": Emphasis,
	"a": Link,
}

// Span is a run of text with one kind. Emph is set inside <em> (or the like)
// as well as when Kind is Emphasis, so "<kanji><em>x</em></kanji>" stays a
// kanji span that is also emphasized. URL is set inside a link.
type Span struct {
	Text string
	Kind Kind
	Emph bool
	URL  string
}

// Paragraph is one line of the source text, as spans.
type Paragraph []Span

// Text returns the paragraph's plain text.
func (p Paragraph) Text() string {
	var b strings.Builder
	for _, s := range p {
		b.WriteString(s.Text)
	}
	return b.String()
}

var (
	tagRe  = regexp.MustCompile(`<(/?)([A-Za-z][A-Za-z0-9]*)([^>]*)>`)
	hrefRe = regexp.MustCompile(`href\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

type open struct {
	kind Kind
	url  string
}

// Parse splits s into paragraphs of spans. Blank lines are dropped.
func Parse(s string) []Paragraph {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out []Paragraph
	for _, line := range strings.Split(s, "\n") {
		if p := parseLine(line); len(strings.TrimSpace(p.Text())) > 0 {
			out = append(out, p)
		}
	}
	return out
}

func parseLine(line string) Paragraph {
	var (
		p     Paragraph
		stack []open
	)
	emit := func(text string) {
		text = html.UnescapeString(text)
		if text == "" {
			return
		}
		sp := Span{Text: text}
		for _, o := range stack {
			switch o.kind {
			case Emphasis:
				sp.Emph = true
			case Link:
				sp.URL = o.url
			}
			if o.kind != unknown && (o.kind != Emphasis || sp.Kind == Plain) {
				sp.Kind = o.kind
			}
		}
		// Merge with the previous span when nothing changed (entities, dropped tags).
		if n := len(p); n > 0 && p[n-1].Kind == sp.Kind && p[n-1].Emph == sp.Emph && p[n-1].URL == sp.URL {
			p[n-1].Text += sp.Text
			return
		}
		p = append(p, sp)
	}
	pos := 0
	for _, m := range tagRe.FindAllStringSubmatchIndex(line, -1) {
		emit(line[pos:m[0]])
		pos = m[1]
		closing := line[m[2]:m[3]] == "/"
		name := strings.ToLower(line[m[4]:m[5]])
		if name == "br" || strings.HasSuffix(line[m[6]:m[7]], "/") {
			continue // self-closing
		}
		if closing {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		k, ok := kinds[name]
		if !ok {
			k = unknown
		}
		o := open{kind: k}
		if k == Link {
			if h := hrefRe.FindStringSubmatch(line[m[6]:m[7]]); h != nil {
				o.url = html.UnescapeString(h[1] + h[2])
			}
		}
		stack = append(stack, o)
	}
	emit(line[pos:])
	return p
}
