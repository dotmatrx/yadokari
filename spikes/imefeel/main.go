// Spike 3: how does internal/ime feel to type in a Bubble Tea v2 input line?
//
// Committed kana render normally, pending romaji underlined, and the real
// terminal cursor sits after them. Tab toggles reading (あ) / meaning (A)
// mode. Enter grades; Enter again moves on. Throwaway code.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jtharris/yadokari/internal/ime"
)

type prompt struct{ word, reading, hint string }

var prompts = []prompt{
	{"教育", "きょういく", "kyouiku"},
	{"学校", "がっこう", "gakkou"},
	{"こんにちは", "こんにちは", "kon'nichiha or konnnichiha (konnichiha → こんいちは, as on WK)"},
	{"新聞", "しんぶん", "shinbun"},
	{"女", "おんな", "onnna (onna → おんあ, as on WK)"},
	{"一致", "いっち", "itchi or icchi"},
	{"ラーメン", "らーめん", "ra-men, or RA-MEN for katakana"},
	{"勉強", "べんきょう", "benkyou"},
}

type state int

const (
	typing state = iota
	graded
)

type model struct {
	conv    ime.Converter
	meaning bool   // meaning mode: no conversion
	plain   []rune // meaning-mode text
	idx     int
	state   state
	result  string
	ok      bool
	warn    string
	score   int
}

var (
	titleSt   = lipgloss.NewStyle().Bold(true)
	wordSt    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff00aa"))
	pendingSt = lipgloss.NewStyle().Underline(true)
	modeRead  = lipgloss.NewStyle().Reverse(true).Padding(0, 1)
	modeMean  = lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder(), false)
	okSt      = lipgloss.NewStyle().Foreground(lipgloss.Color("#22cc55")).Bold(true)
	badSt     = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4455")).Bold(true)
	warnSt    = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffcc00")).Bold(true)
	dimSt     = lipgloss.NewStyle().Faint(true)
)

func main() {
	if _, err := tea.NewProgram(&model{}).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		m.insert(msg.Content)
	case tea.KeyPressMsg:
		switch {
		case msg.String() == "ctrl+c" || msg.Code == tea.KeyEscape:
			return m, tea.Quit
		case msg.Code == tea.KeyEnter:
			m.enter()
		case msg.Code == tea.KeyTab:
			m.meaning = !m.meaning
		case msg.Code == tea.KeyBackspace:
			if m.state == typing {
				m.warn = ""
				if m.meaning {
					if len(m.plain) > 0 {
						m.plain = m.plain[:len(m.plain)-1]
					}
				} else {
					m.conv.Backspace()
				}
			}
		case msg.String() == "ctrl+u":
			m.conv.Reset()
			m.plain = nil
		case msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0:
			m.insert(msg.Text)
		}
	}
	return m, nil
}

func (m *model) insert(s string) {
	if m.state != typing {
		return
	}
	m.warn = ""
	if m.meaning {
		m.plain = append(m.plain, []rune(s)...)
		return
	}
	m.conv.TypeString(s)
}

func (m *model) enter() {
	if m.state == graded {
		m.idx = (m.idx + 1) % len(prompts)
		m.state = typing
		m.conv.Reset()
		m.plain = nil
		m.result = ""
		return
	}
	if m.meaning {
		m.warn = "that's meaning mode (A); press Tab for あ"
		return
	}
	answer := strings.TrimSpace(m.conv.Finish())
	if answer == "" {
		return
	}
	if !ime.IsKana(answer) {
		m.warn = "contains non-kana; fix and resubmit (not counted as wrong)"
		return
	}
	m.ok = ime.ToHiragana(answer) == prompts[m.idx].reading
	if m.ok {
		m.score++
	}
	m.result = answer
	m.state = graded
}

const inputRow = 6

func (m *model) View() tea.View {
	p := prompts[m.idx]
	var b strings.Builder
	b.WriteString(titleSt.Render("Spike 3 · IME feel") + dimSt.Render(fmt.Sprintf("   %d/%d · score %d", m.idx+1, len(prompts), m.score)) + "\n\n")
	b.WriteString("Reading for  " + wordSt.Render(p.word) + "\n")
	b.WriteString(dimSt.Render("hint: "+p.hint) + "\n\n")
	b.WriteString(dimSt.Render("committed | pending (underlined)") + "\n")

	// inputRow
	mode := modeRead.Render("あ")
	if m.meaning {
		mode = modeMean.Render("A")
	}
	var committed, pending string
	if m.meaning {
		committed = string(m.plain)
	} else {
		committed, pending = m.conv.Committed(), m.conv.Pending()
	}
	prefix := mode + " "
	line := prefix + committed + pendingSt.Render(pending)
	b.WriteString(line + "\n")

	switch {
	case m.warn != "":
		b.WriteString(warnSt.Render("⚠ "+m.warn) + "\n")
	case m.state == graded && m.ok:
		b.WriteString(okSt.Render("✓ correct") + dimSt.Render("  (Enter for next)") + "\n")
	case m.state == graded:
		b.WriteString(badSt.Render("✗ "+m.result) + "  expected " + p.reading + dimSt.Render("  (Enter for next)") + "\n")
	default:
		b.WriteString("\n")
	}
	b.WriteString("\n" + dimSt.Render("Enter submit/next · Tab あ/A · Backspace · ctrl+u clear · Esc quit"))

	v := tea.NewView(b.String())
	v.AltScreen = true
	if m.state == typing {
		x := lipgloss.Width(prefix) + lipgloss.Width(committed) + lipgloss.Width(pending)
		c := tea.NewCursor(x, inputRow)
		c.Shape = tea.CursorBar
		v.Cursor = c
	}
	return v
}
