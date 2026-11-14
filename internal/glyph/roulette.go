package glyph

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font/opentype"
)

// FontRef is an installed font: a file and, in a collection, the member.
type FontRef struct {
	Path   string
	Index  int // collection member; -1: let LoadFont pick the Japanese one
	Family string
}

// fcFormat is what ListFonts asks fc-list for: one font per line.
const fcFormat = `%{file}\t%{family[0]}\t%{style[0]}\t%{index}\t%{variable}\n`

// ListFonts returns the installed Japanese fonts, one per family: from
// fontconfig where there is one, else by scanning the usual font folders
// (macOS has no fontconfig unless installed).
func ListFonts() ([]FontRef, error) {
	out, err := exec.Command("fc-list", "-f", fcFormat, ":lang=ja").Output()
	if err == nil {
		if refs := parseFCList(string(out)); len(refs) > 0 {
			return refs, nil
		}
	}
	refs := scanFonts(fontDirs())
	if len(refs) == 0 && err != nil {
		return nil, fmt.Errorf("glyph: fc-list: %w; no Japanese fonts in the font folders", err)
	}
	return refs, nil
}

// otherScripts are family-name markers of CJK fonts for other languages:
// their kanji take Chinese or Korean forms.
var otherScripts = []string{" SC", " TC", " HK", " KR", " CN", " GB", " CNS", " Mono"}

func otherScript(family string) bool {
	return slices.ContainsFunc(otherScripts, func(m string) bool {
		return strings.HasSuffix(family, m) || strings.Contains(family, m+" ")
	})
}

// candidate is one face of an installed font, before one per family is kept.
type candidate struct {
	ref   FontRef
	style string
}

// styleRank orders a family's faces: the text weight first.
func styleRank(style string) int {
	for i, s := range []string{"regular", "w3", "book", "normal", "w4", "medium", "r"} {
		if strings.EqualFold(style, s) {
			return i
		}
	}
	return 99
}

// perFamily keeps one face per family (styleRank, then the first listed),
// in the order families first appear.
func perFamily(cands []candidate) []FontRef {
	var order []string
	best := map[string]candidate{}
	for _, c := range cands {
		old, seen := best[c.ref.Family]
		if !seen {
			order = append(order, c.ref.Family)
		}
		if !seen || styleRank(c.style) < styleRank(old.style) {
			best[c.ref.Family] = c
		}
	}
	refs := make([]FontRef, 0, len(order))
	for _, fam := range order {
		refs = append(refs, best[fam].ref)
	}
	return refs
}

// parseFCList reads fc-list output (fcFormat) into one font per family.
// Variable fonts (which sfnt can't draw), monospaced and non-Japanese CJK
// families are left out.
func parseFCList(out string) []FontRef {
	var cands []candidate
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 5 || f[0] == "" || f[1] == "" {
			continue
		}
		path, family, style, variable := f[0], f[1], f[2], f[4]
		if strings.EqualFold(variable, "true") || strings.Contains(path, "-VF.") || otherScript(family) {
			continue
		}
		index, err := strconv.Atoi(f[3])
		if err != nil {
			continue
		}
		cands = append(cands, candidate{FontRef{Path: path, Index: index, Family: family}, style})
	}
	return perFamily(cands)
}

// SelectFonts narrows the installed fonts to the names given: a font file
// (used as is) or a family name, matched without case. An unknown name is
// an error that lists the installed families. No names: all of them.
func SelectFonts(installed []FontRef, names []string) ([]FontRef, error) {
	if len(names) == 0 {
		return installed, nil
	}
	var out []FontRef
	for _, n := range names {
		path := n
		if rest, ok := strings.CutPrefix(n, "~/"); ok {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, rest)
			}
		}
		if _, err := os.Stat(path); err == nil {
			out = append(out, FontRef{Path: path, Index: -1})
			continue
		}
		i := slices.IndexFunc(installed, func(r FontRef) bool { return strings.EqualFold(r.Family, n) })
		if i < 0 {
			fams := make([]string, len(installed))
			for j, r := range installed {
				fams[j] = r.Family
			}
			return nil, fmt.Errorf("no font file or installed Japanese font named %q (installed: %s)", n, strings.Join(fams, ", "))
		}
		out = append(out, installed[i])
	}
	return out, nil
}

// LoadFontAt loads one member of a font file, reading it from disk as
// needed rather than holding it in memory (CJK collections are ~20 MB).
func LoadFontAt(ref FontRef) (*Font, error) {
	if ref.Index < 0 {
		return LoadFont(ref.Path)
	}
	file, err := os.Open(ref.Path)
	if err != nil {
		return nil, err
	}
	coll, err := opentype.ParseCollectionReaderAt(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("glyph: %s: %w", ref.Path, err)
	}
	f, err := coll.Font(ref.Index)
	if err == nil && !covers(f) {
		err = fmt.Errorf("glyph: %s #%d: doesn't cover kanji and kana", ref.Path, ref.Index)
	}
	if err != nil {
		file.Close()
		return nil, err
	}
	family := ref.Family
	if family == "" {
		family, _ = f.Name(nil, 1)
	}
	return &Font{Path: ref.Path, Family: family, sfnt: f}, nil // the file stays open for the font's life
}

// Pool is the font roulette: a random font for each question, never the
// same one twice in a row when there's a choice. Fonts load on first use;
// one that fails to load is dropped (and logged).
type Pool struct {
	mu     sync.Mutex
	refs   []FontRef
	loaded map[FontRef]*Font
	load   func(FontRef) (*Font, error)
	last   int
}

// NewPool makes a roulette over refs.
func NewPool(refs []FontRef) *Pool {
	return &Pool{refs: slices.Clone(refs), loaded: map[FontRef]*Font{}, load: LoadFontAt, last: -1}
}

// Len is the number of fonts still in the pool.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.refs)
}

// Pick returns a random font that has every character of text; nil when
// none does (or none loads), and the caller uses its default font.
func (p *Pool) Pick(rng *rand.Rand, text string) *Font {
	p.mu.Lock()
	defer p.mu.Unlock()
	lastRef := FontRef{Index: -2}
	if p.last >= 0 && p.last < len(p.refs) {
		lastRef = p.refs[p.last]
	}
	// Candidates in random order, the last font used at the end.
	order := rng.Perm(len(p.refs))
	cands := make([]FontRef, 0, len(order))
	for _, i := range order {
		if p.refs[i] != lastRef {
			cands = append(cands, p.refs[i])
		}
	}
	if len(cands) < len(p.refs) {
		cands = append(cands, lastRef)
	}
	for _, ref := range cands {
		f := p.loaded[ref]
		if f == nil {
			var err error
			if f, err = p.load(ref); err != nil {
				slog.Warn("font roulette: dropping a font", "font", ref.Path, "index", ref.Index, "err", err)
				p.refs = slices.DeleteFunc(p.refs, func(r FontRef) bool { return r == ref })
				continue
			}
			p.loaded[ref] = f
		}
		if f.Covers(text) {
			p.last = slices.Index(p.refs, ref)
			return f
		}
	}
	return nil
}

// PoolOf makes a roulette over fonts already loaded (for tests and previews).
func PoolOf(fonts ...*Font) *Pool {
	p := NewPool(nil)
	for i, f := range fonts {
		ref := FontRef{Path: f.Path, Index: i, Family: f.Family}
		p.refs = append(p.refs, ref)
		p.loaded[ref] = f
	}
	return p
}
