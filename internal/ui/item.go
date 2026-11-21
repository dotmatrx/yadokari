package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jtharris/yadokari/internal/gfx"
	"github.com/jtharris/yadokari/internal/glyph"
	"github.com/jtharris/yadokari/internal/store"
	"github.com/jtharris/yadokari/internal/wk"
)

// ItemData is everything the item page shows about one subject.
type ItemData struct {
	Subject       wk.Resource[wk.Subject]
	Components    []wk.Resource[wk.Subject] // radicals of a kanji, kanji of a word
	Amalgamations []wk.Resource[wk.Subject] // kanji using a radical, words using a kanji
	Similar       []wk.Resource[wk.Subject] // visually similar kanji
	Synonyms      []string
	MeaningNote   string
	ReadingNote   string
}

// LoadItem gathers a subject and its related subjects from the store.
func LoadItem(ctx context.Context, st *store.Store, subjectID int) (ItemData, error) {
	subj, err := st.Subject(ctx, subjectID)
	if err != nil {
		return ItemData{}, err
	}
	if subj == nil {
		return ItemData{}, fmt.Errorf("subject %d is not in the store", subjectID)
	}
	d := ItemData{Subject: *subj}
	if d.Components, err = st.Subjects(ctx, subj.Data.ComponentSubjectIDs); err != nil {
		return d, err
	}
	if d.Amalgamations, err = st.Subjects(ctx, subj.Data.AmalgamationSubjectIDs); err != nil {
		return d, err
	}
	if d.Similar, err = st.Subjects(ctx, subj.Data.VisuallySimilarSubjectIDs); err != nil {
		return d, err
	}
	sm, err := st.StudyMaterial(ctx, subjectID)
	if err != nil {
		return d, err
	}
	if sm != nil {
		d.Synonyms = sm.MeaningSynonyms
		if sm.MeaningNote != nil {
			d.MeaningNote = *sm.MeaningNote
		}
		if sm.ReadingNote != nil {
			d.ReadingNote = *sm.ReadingNote
		}
	}
	return d, nil
}

type itemTab int

const (
	tabName itemTab = iota // a radical's meaning
	tabComponents
	tabMeaning
	tabReading
	tabContext
	tabFoundIn
)

// tabs are the item page's tabs for a subject type, in WaniKani's lesson
// order with what it shows elsewhere ("Found in") at the end.
func tabs(object string) []itemTab {
	switch object {
	case wk.ObjectRadical:
		return []itemTab{tabName, tabFoundIn}
	case wk.ObjectKanji:
		return []itemTab{tabComponents, tabMeaning, tabReading, tabFoundIn}
	case wk.ObjectKanaVocabulary:
		return []itemTab{tabMeaning, tabContext}
	}
	return []itemTab{tabComponents, tabMeaning, tabReading, tabContext}
}

func (t itemTab) title(object string) string {
	switch t {
	case tabName:
		return "Name"
	case tabComponents:
		if object == wk.ObjectKanji {
			return "Radicals"
		}
		return "Kanji"
	case tabMeaning:
		return "Meaning"
	case tabReading:
		return "Reading"
	case tabContext:
		return "Context"
	}
	return "Found in"
}

// ItemPage shows one subject: its glyph and summary on top, tabs of
// meaning, reading, mnemonics and related subjects below. The review screen
// opens it after an answer; lessons will step through it.
type ItemPage struct {
	data ItemData
	gfx  PageDeps
	svg  []byte // the radical image, when the subject has no characters
	tabs []itemTab
	tab  int
	top  int // first body line shown
	// HideMeaning and HideReading keep a part not yet asked in a review
	// from being given away.
	HideMeaning, HideReading bool
	// Section names the screen in the header ("item" when empty). Strip
	// (e.g. a lesson batch) goes in the header's right when it fits, else
	// Progress. Help replaces the footer's key hints.
	Section, Strip, Progress, Help string
	// Status is a one-off message for the footer (e.g. an audio problem),
	// cleared by the next key.
	Status string

	width, height int
}

// PageDeps draw the item's glyph (a nil Renderer draws plain text) and
// reach the App's shared state (graphics ready, the audio badge).
type PageDeps struct {
	Renderer gfx.Renderer
	Font     *glyph.Font
	shared   *shared
}

func (g PageDeps) graphicsReady() bool { return g.shared != nil && g.shared.graphicsReady }

// NewItemPage opens on the first tab.
func NewItemPage(d ItemData, g PageDeps, svg []byte) *ItemPage {
	return &ItemPage{data: d, gfx: g, svg: svg, tabs: tabs(d.Subject.Object)}
}

// ItemClosedMsg is sent when the item page is dismissed.
type ItemClosedMsg struct{}

// SetSize sets the area the page fills.
func (p *ItemPage) SetSize(w, h int) { p.width, p.height = w, h }

// Key handles a key press: tabs, scrolling, and closing.
func (p *ItemPage) Key(msg tea.KeyPressMsg) tea.Cmd {
	p.Status = ""
	switch msg.String() {
	case "p":
		if p.audible() {
			return playAudio(p.data.Subject)
		}
	case "esc", "q", "i", "enter":
		return func() tea.Msg { return ItemClosedMsg{} }
	case "right", "l", "tab":
		p.setTab(p.tab + 1)
	case "left", "h", "shift+tab":
		p.setTab(p.tab - 1)
	case "down", "j":
		p.scroll(1)
	case "up", "k":
		p.scroll(-1)
	case "pgdown", "space", "f":
		p.scroll(max(p.bodyRows()-1, 1))
	case "pgup", "b":
		p.scroll(-max(p.bodyRows()-1, 1))
	case "home", "g":
		p.top = 0
	}
	return nil
}

// audible reports whether p plays the item's pronunciation: it has one,
// and its reading isn't hidden.
func (p *ItemPage) audible() bool {
	return hasAudio(p.data.Subject) && (!p.HideReading || p.data.Subject.Object == wk.ObjectKanaVocabulary)
}

func (p *ItemPage) setTab(i int) {
	p.tab = (i + len(p.tabs)) % len(p.tabs)
	p.top = 0
}

func (p *ItemPage) scroll(n int) {
	lines := strings.Count(p.body(p.innerWidth()), "\n") + 1
	p.top = max(min(p.top+n, lines-p.bodyRows()), 0)
}

// Layout: header, the glyph beside a summary panel, the tab panel, footer.

func (p *ItemPage) size() (w, h int) { return max(p.width, 40), max(p.height, 12) }

// topRows is the height of the glyph and summary panel.
func (p *ItemPage) topRows() int {
	_, h := p.size()
	return min(max((h-2)/4, 4), 8)
}

// glyphSize is the glyph field: roughly square on typical cells.
func (p *ItemPage) glyphSize() (cols, rows int) {
	rows = p.topRows()
	return rows * 5 / 2, rows
}

// bodyRows is how many content lines the tab panel shows.
func (p *ItemPage) bodyRows() int {
	_, h := p.size()
	return max(h-2-p.topRows()-1-2, 1) // header+footer, top, gap, panel borders
}

func (p *ItemPage) innerWidth() int {
	w, _ := p.size()
	return max(w-4, 1)
}

// DrawGlyph draws the glyph so the renderer queues any upload. Only App
// calls it (through the screen showing the page), then sends the queue.
func (p *ItemPage) DrawGlyph() {
	cols, rows := p.glyphSize()
	_, _ = p.drawGlyph(cols, rows)
}

func (p *ItemPage) drawGlyph(cols, rows int) (string, error) {
	missing := p.data.Subject.Data.Characters == nil && p.svg == nil
	return drawSubjectGlyph(p.gfx.Renderer, p.gfx.graphicsReady(), p.source(), missing, cols, rows, typeColour[p.data.Subject.Object])
}

func (p *ItemPage) source() gfx.Source {
	return subjectSource(p.data.Subject, p.gfx.Font, p.svg)
}

// View renders the page at its size.
func (p *ItemPage) View() string {
	w, h := p.size()
	subj := p.data.Subject
	section, help := p.Section, p.Help
	if section == "" {
		section = "item"
	}
	if help == "" {
		help = "←/→ tabs · ↑/↓ scroll · Esc back"
		if p.audible() {
			help = "←/→ tabs · ↑/↓ scroll · p audio · Esc back"
		}
	}
	// The batch strip shrinks to "2 / 5" before the subject's type goes.
	segs := []segment{
		{text: p.Strip, alt: mutedSt.Render(p.Progress), rank: rankInfo, sep: "   "},
		{text: mutedSt.Render(fmt.Sprintf("%s · level %d", typeName[subj.Object], subj.Data.Level)), rank: rankKey},
	}
	if p.Strip == "" {
		segs[0] = segment{text: segs[0].alt, rank: rankInfo}
	}
	header := headerLine(appTitle(section), segs, w, p.gfx.shared)
	footer := padRight(" "+mutedSt.Render(help), w)
	if p.Status != "" {
		footer = spread(" "+mutedSt.Render(help), warnSt.Render(p.Status)+" ", w)
		if lipgloss.Width(footer) > w {
			footer = padRight(" "+warnSt.Render(p.Status), w)
		}
	}

	cols, rows := p.glyphSize()
	bg := typeColour[subj.Object]
	cells, err := p.drawGlyph(cols, rows)
	if err != nil {
		cells, _ = plainGlyph(p.source(), cols, rows, bg)
	}
	sumW := max(w-cols-1, 10)
	top := sideBySide(1, cells, panel("", p.summary(sumW-4), sumW, rows))

	inner := p.innerWidth()
	lines := strings.Split(p.body(inner), "\n")
	n := p.bodyRows()
	p.top = max(min(p.top, len(lines)-n), 0)
	note := ""
	if len(lines) > n {
		note = mutedSt.Render(fmt.Sprintf("%d–%d of %d", p.top+1, min(p.top+n, len(lines)), len(lines)))
	}
	shown := lines[p.top:min(p.top+n, len(lines))]
	body := panelWith(lineSt, p.tabStrip(), note, strings.Join(shown, "\n"), w, n+2)
	return frame(header, top+"\n\n"+body, footer, h)
}

func (p *ItemPage) tabStrip() string {
	var parts []string
	for i, t := range p.tabs {
		name := t.title(p.data.Subject.Object)
		if i == p.tab {
			parts = append(parts, titleSt.Underline(true).Render(name))
		} else {
			parts = append(parts, mutedSt.Render(name))
		}
	}
	return strings.Join(parts, mutedSt.Render(" · "))
}

// summary is the panel beside the glyph: meanings, readings, and what the
// subject is.
func (p *ItemPage) summary(width int) string {
	d := p.data.Subject.Data
	var lines []string
	if p.HideMeaning {
		lines = append(lines, mutedSt.Render("Meaning hidden until it's asked"))
	} else {
		line := boldSt.Render(primaryMeaning(d))
		if alt := p.alternatives(); len(alt) > 0 {
			line += mutedSt.Render(" · " + strings.Join(alt, ", "))
		}
		lines = append(lines, line)
	}
	if len(d.Readings) > 0 {
		if p.HideReading {
			lines = append(lines, mutedSt.Render("Reading hidden until it's asked"))
		} else {
			lines = append(lines, readingLine(d.Readings, p.data.Subject.Object))
		}
	}
	if len(d.PartsOfSpeech) > 0 {
		lines = append(lines, mutedSt.Render(strings.Join(d.PartsOfSpeech, ", ")))
	}
	for i, l := range lines {
		lines[i] = lipgloss.NewStyle().Width(width).Render(l)
	}
	return strings.Join(lines, "\n")
}

// alternatives are the accepted meanings besides the primary one, then the
// user's synonyms.
func (p *ItemPage) alternatives() []string {
	var out []string
	for _, m := range p.data.Subject.Data.Meanings {
		if !m.Primary && m.AcceptedAnswer {
			out = append(out, m.Meaning)
		}
	}
	return append(out, p.data.Synonyms...)
}

// readingLine lists readings: a kanji's grouped by type (the accepted ones
// bold), a word's in a row.
func readingLine(rs []wk.Reading, object string) string {
	if object != wk.ObjectKanji {
		var out []string
		for _, r := range rs {
			if r.AcceptedAnswer {
				out = append(out, r.Reading)
			}
		}
		return strings.Join(out, ", ")
	}
	var parts []string
	for _, typ := range []string{wk.ReadingOnyomi, wk.ReadingKunyomi, wk.ReadingNanori} {
		var out []string
		for _, r := range rs {
			if r.Type != typ {
				continue
			}
			if r.AcceptedAnswer {
				out = append(out, boldSt.Render(r.Reading))
			} else {
				out = append(out, mutedSt.Render(r.Reading))
			}
		}
		if len(out) > 0 {
			parts = append(parts, mutedSt.Render(readingTypeName[typ]+" ")+strings.Join(out, mutedSt.Render(", ")))
		}
	}
	return strings.Join(parts, mutedSt.Render("  ·  "))
}

var readingTypeName = map[string]string{
	wk.ReadingOnyomi: "On'yomi", wk.ReadingKunyomi: "Kun'yomi", wk.ReadingNanori: "Nanori",
}

// body is the current tab's content wrapped to width.
func (p *ItemPage) body(width int) string {
	d := p.data.Subject.Data
	obj := p.data.Subject.Object
	var s sections
	switch p.tabs[p.tab] {
	case tabName, tabMeaning:
		if p.HideMeaning {
			return mutedSt.Render("Hidden until the meaning has been asked.")
		}
		if obj != wk.ObjectRadical {
			s.markup("Mnemonic", d.MeaningMnemonic, width)
		} else {
			s.markup("", d.MeaningMnemonic, width)
		}
		s.hint(d.MeaningHint, width)
		s.note("Your note", p.data.MeaningNote, width)
	case tabReading:
		if p.HideReading {
			return mutedSt.Render("Hidden until the reading has been asked.")
		}
		s.markup("Mnemonic", d.ReadingMnemonic, width)
		s.hint(d.ReadingHint, width)
		s.note("Your note", p.data.ReadingNote, width)
	case tabContext:
		if p.HideMeaning {
			return mutedSt.Render("Hidden until the meaning has been asked.")
		}
		if len(d.ContextSentences) == 0 {
			return mutedSt.Render("No context sentences.")
		}
		for _, cs := range d.ContextSentences {
			s.add(wrap(cs.Ja, width) + "\n" + mutedSt.Render(wrap(cs.En, width)))
		}
	case tabComponents:
		s.grid("", p.data.Components, width)
	case tabFoundIn:
		s.grid("", p.data.Amalgamations, width)
		s.grid("Visually similar", p.data.Similar, width)
		if len(s) == 0 {
			return mutedSt.Render("Not used in any other subject.")
		}
	}
	return strings.Join(s, "\n\n")
}

// sections is the tab body built up as blocks separated by blank lines.
type sections []string

func (s *sections) add(block string) { *s = append(*s, block) }

func heading(h string) string {
	if h == "" {
		return ""
	}
	return titleSt.Render(h) + "\n"
}

func (s *sections) markup(h, src string, width int) {
	if strings.TrimSpace(src) != "" {
		s.add(heading(h) + renderMarkup(src, width))
	}
}

func (s *sections) hint(src *string, width int) {
	if src != nil && strings.TrimSpace(*src) != "" {
		s.add(heading("Hint") + renderMarkup(*src, width))
	}
}

func (s *sections) note(h, text string, width int) {
	if strings.TrimSpace(text) != "" {
		s.add(heading(h) + wrap(text, width))
	}
}

func (s *sections) grid(h string, subjects []wk.Resource[wk.Subject], width int) {
	if len(subjects) == 0 {
		return
	}
	items := make([]SummaryItem, len(subjects))
	for i, r := range subjects {
		items[i] = SummaryItem{Characters: characters(r), Meaning: primaryMeaning(r.Data), Type: r.Object}
	}
	s.add(heading(h) + itemGrid(items, width))
}

func wrap(s string, width int) string {
	return lipgloss.NewStyle().Width(width).Render(s)
}
