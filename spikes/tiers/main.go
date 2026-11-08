// Throwaway demo: the same glyph drawn by every gfx tier, side by side, to
// judge them by eye in different terminals. The kitty column only works in
// kitty/Ghostty. Keys: n/p glyph · s size · q quit.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jtharris/yadokari/internal/gfx"
	"github.com/jtharris/yadokari/internal/glyph"
	"github.com/jtharris/yadokari/internal/wk"
)

type item struct {
	label string
	src   gfx.Source
	bg    color.Color
}

var (
	radicalBlue = color.RGBA{0x00, 0xaa, 0xff, 255}
	kanjiPink   = color.RGBA{0xff, 0x00, 0xaa, 255}
	vocabPurple = color.RGBA{0xaa, 0x00, 0xff, 255}
	white       = color.RGBA{255, 255, 255, 255}
)

type model struct {
	items     []item
	idx, size int
	renderers []gfx.Renderer
	ready     bool
	err       error
}

var sizes = [][2]int{{16, 8}, {24, 12}, {10, 5}}

type readyMsg struct{}

func main() {
	f, err := glyph.FindFont("")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	text := func(key, s string) gfx.Source {
		return gfx.Source{Key: key, Text: s, Mask: func(w, h, ss int) (*image.Alpha, error) { return f.Text(s, w, h, ss) }}
	}
	items := []item{
		{"kanji 鬱 (dense)", text("k:鬱", "鬱"), kanjiPink},
		{"kanji 漢", text("k:漢", "漢"), kanjiPink},
		{"vocab 勉強", text("v:勉強", "勉強"), vocabPurple},
		{"radical 一", text("r:一", "一"), radicalBlue},
	}
	if it, ok := imageRadical(); ok {
		items = append(items, it)
	}
	cw, ch := gfx.CellSize()
	m := &model{items: items, renderers: []gfx.Renderer{gfx.New(gfx.Kitty, cw, ch), gfx.New(gfx.HalfBlock, 0, 0), gfx.New(gfx.Plain, 0, 0)}}
	fmt.Fprintf(os.Stderr, "font %s, cell %dx%d px, detected tier %s\n", f.Name(), cw, ch, gfx.Detect(os.Getenv))
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// imageRadical loads the first image-only radical from a local recording.
func imageRadical() (item, bool) {
	files, _ := filepath.Glob("testdata/recordings/*/subjects.json")
	if len(files) == 0 {
		return item{}, false
	}
	raw, err := os.ReadFile(files[len(files)-1])
	if err != nil {
		return item{}, false
	}
	var subjects []wk.Resource[wk.Subject]
	if json.Unmarshal(raw, &subjects) != nil {
		return item{}, false
	}
	cache := glyph.ImageCache{Dir: ".dev/images"}
	for _, s := range subjects {
		if s.Object != wk.ObjectRadical || s.Data.Characters != nil {
			continue
		}
		url, ct := glyph.PickImage(s.Data.CharacterImages)
		if url == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		raw, err := cache.Get(ctx, url)
		cancel()
		if err != nil {
			fmt.Fprintln(os.Stderr, "radical image:", err)
			return item{}, false
		}
		src := gfx.Source{Key: "img:" + s.Data.Slug, Mask: func(w, h, ss int) (*image.Alpha, error) {
			return glyph.ImageMask(raw, ct, w, h) // already anti-aliased; ss not needed
		}}
		return item{"radical image: " + s.Data.Slug, src, radicalBlue}, true
	}
	return item{}, false
}

func (m *model) Init() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return readyMsg{} })
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case readyMsg:
		m.ready = true
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Sequence(tea.Raw(m.renderers[0].Reset()), tea.Quit)
		case "n":
			m.idx = (m.idx + 1) % len(m.items)
		case "p":
			m.idx = (m.idx + len(m.items) - 1) % len(m.items)
		case "s":
			m.size = (m.size + 1) % len(sizes)
		}
	}
	return m, m.setup()
}

// setup uploads the current kitty image, once the alt screen is active.
func (m *model) setup() tea.Cmd {
	if !m.ready {
		return nil
	}
	it := m.items[m.idx]
	cols, rows := sizes[m.size][0], sizes[m.size][1]
	f, err := m.renderers[0].Draw(it.src, cols, rows, white, it.bg)
	if err != nil || f.Setup == "" {
		return nil
	}
	return tea.Raw(f.Setup)
}

func (m *model) View() tea.View {
	it := m.items[m.idx]
	cols, rows := sizes[m.size][0], sizes[m.size][1]
	var cols3 []string
	for _, r := range m.renderers {
		f, err := r.Draw(it.src, cols, rows, white, it.bg)
		body := f.Cells
		if err != nil {
			body = "error: " + err.Error()
		}
		if r.Tier() == gfx.Kitty && !m.ready {
			body = strings.Repeat(strings.Repeat(" ", cols)+"\n", rows)
		}
		cols3 = append(cols3, lipgloss.JoinVertical(lipgloss.Left, string(r.Tier()), body))
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, cols3[0], "   ", cols3[1], "   ", cols3[2])
	v := tea.NewView(fmt.Sprintf("%s  (%dx%d cells)\n\n%s\n\nn/p glyph · s size · q quit", it.label, cols, rows, row))
	v.AltScreen = true
	return v
}
