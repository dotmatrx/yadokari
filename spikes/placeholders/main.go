// Spike 1: kitty Unicode placeholders inside a Bubble Tea v2 app.
//
// Rasterizes kanji with Noto Sans CJK JP, transmits each once as a PNG with a
// virtual placement (U=1), and renders it as a grid of U+10EEEE placeholder
// cells in the normal View. Throwaway code; see spikes/README.md.
//
// Keys: n/p next/prev glyph · s cycle size · b toggle type-colour background
//
//	m toggle fg encoding (raw SGR vs Lip Gloss) · t toggle 10 Hz re-render
//	r forget transmitted images and re-send · q quit (deletes images)
//
// --early transmits from Init, before the first frame enters the alt screen.
// In one early run the first image never appeared; it was not reproducible
// (kitty shows images sent on the main screen in the alt screen too). The
// default defers the first send until after the first frame anyway.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/sys/unix"
)

var glyphs = []string{"漢", "鬱", "一", "語", "曜", "勉強", "一つ"}

var sizes = [][2]int{{20, 10}, {40, 20}, {10, 5}, {60, 26}}

// WaniKani-ish type colours: radical, kanji, vocabulary.
var typeColours = []string{"#00aaff", "#ff00aa", "#aa00ff"}

type tickMsg time.Time

// firstFrameMsg arrives after the first frame has switched to the alt screen.
type firstFrameMsg struct{}

type model struct {
	face       *sfnt.Font
	fontPath   string
	w, h       int // terminal cells
	cellW      int // pixels
	cellH      int
	glyph      int
	size       int
	bg         bool
	lipglossFG bool
	ticking    bool
	ticks      int
	sent       map[string]bool // glyph@size@cellpx already transmitted
	sentBytes  int
	ready      bool // safe to transmit: alt screen is active
	err        error
}

// imageID is kept < 256 so the 256-colour index form of the foreground colour
// carries it exactly (no quantization risk).
func (m *model) imageID() int { return 1 + m.glyph*len(sizes) + m.size }

func (m *model) key() string {
	return fmt.Sprintf("%d@%d@%dx%d", m.glyph, m.size, m.cellW, m.cellH)
}

var early = flag.Bool("early", false, "transmit from Init, before the alt screen is active")

func main() {
	flag.Parse()
	path, f, err := loadFont()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	m := &model{face: f, fontPath: path, sent: map[string]bool{}}
	m.cellW, m.cellH = cellPixels()
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (m *model) Init() tea.Cmd {
	if *early {
		m.ready = true
		return m.transmit()
	}
	// The renderer runs at 60 fps, so the first frame (which enters the alt
	// screen) has been flushed well within 100ms.
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return firstFrameMsg{} })
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		// Font-size changes in kitty arrive as resizes; re-measure the cell
		// and re-rasterize if it changed so the glyph stays crisp.
		m.cellW, m.cellH = cellPixels()
		return m, m.transmit()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Sequence(tea.Raw(deleteAll()), tea.Quit)
		case "n":
			m.glyph = (m.glyph + 1) % len(glyphs)
		case "p":
			m.glyph = (m.glyph + len(glyphs) - 1) % len(glyphs)
		case "s":
			m.size = (m.size + 1) % len(sizes)
		case "b":
			m.bg = !m.bg
		case "m":
			m.lipglossFG = !m.lipglossFG
		case "r":
			m.sent = map[string]bool{}
		case "t":
			m.ticking = !m.ticking
			if m.ticking {
				return m, tick()
			}
		}
		return m, m.transmit()
	case firstFrameMsg:
		m.ready = true
		return m, m.transmit()
	case tickMsg:
		m.ticks++
		if m.ticking {
			return m, tick()
		}
	}
	return m, nil
}

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// transmit sends the current glyph image once per (glyph, size, cell size).
func (m *model) transmit() tea.Cmd {
	if !m.ready || m.sent[m.key()] {
		return nil
	}
	cols, rows := sizes[m.size][0], sizes[m.size][1]
	img, err := rasterize(m.face, glyphs[m.glyph], cols*m.cellW, rows*m.cellH)
	if err != nil {
		m.err = err
		return nil
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		m.err = err
		return nil
	}
	var seq strings.Builder
	// Replace any previous image with this ID, then transmit + create the
	// virtual placement in one command (a=T with U=1), chunked, quiet.
	seq.WriteString(ansi.KittyGraphics(nil, "a=d", "d=I", fmt.Sprintf("i=%d", m.imageID()), "q=2"))
	err = kitty.EncodeGraphics(&seq, img, &kitty.Options{
		Action:           kitty.TransmitAndPut,
		Format:           kitty.PNG,
		Transmission:     kitty.Direct, // zero value is NOT direct: without this no pixels are sent
		ID:               m.imageID(),
		Columns:          cols,
		Rows:             rows,
		VirtualPlacement: true,
		Quiet:            2,
		Chunk:            true,
	})
	if err != nil {
		m.err = err
		return nil
	}
	m.sent[m.key()] = true
	m.sentBytes += seq.Len()
	return tea.Raw(seq.String())
}

func deleteAll() string {
	return ansi.KittyGraphics(nil, "a=d", "d=A", "q=2")
}

func (m *model) View() tea.View {
	cols, rows := sizes[m.size][0], sizes[m.size][1]
	id := m.imageID()

	var grid strings.Builder
	for r := 0; r < rows; r++ {
		var line strings.Builder
		for c := 0; c < cols; c++ {
			line.WriteRune(kitty.Placeholder)
			line.WriteRune(kitty.Diacritic(r))
			line.WriteRune(kitty.Diacritic(c))
		}
		row := line.String()
		bg := ""
		if m.bg {
			bg = typeColours[m.glyph%len(typeColours)]
		}
		if m.lipglossFG {
			st := lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(id))
			if bg != "" {
				st = st.Background(lipgloss.Color(bg))
			}
			row = st.Render(row)
		} else {
			sgr := fmt.Sprintf("\x1b[38;5;%dm", id)
			if bg != "" {
				var r8, g8, b8 uint8
				fmt.Sscanf(bg, "#%02x%02x%02x", &r8, &g8, &b8)
				sgr += fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r8, g8, b8)
			}
			row = sgr + row + "\x1b[m"
		}
		grid.WriteString(row)
		if r < rows-1 {
			grid.WriteByte('\n')
		}
	}

	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Render(grid.String())
	errLine := ""
	if m.err != nil {
		errLine = "\nERROR: " + m.err.Error()
	}
	fg := "raw SGR 38;5"
	if m.lipglossFG {
		fg = "lipgloss ANSIColor"
	}
	mode := "deferred"
	if *early {
		mode = "EARLY"
	}
	status := fmt.Sprintf("[%s] glyph %q  size %dx%d  cell %dx%dpx  id %d  fg %s  bg %v  ticks %d  sent %dKiB\nfont %s\n%s%s",
		mode, glyphs[m.glyph], cols, rows, m.cellW, m.cellH, id, fg, m.bg, m.ticks, m.sentBytes/1024, m.fontPath,
		"n/p glyph · s size · b background · m fg mode · t 10Hz re-render · r re-send · q quit", errLine)

	// Text immediately left/right of the image row checks that placeholder
	// widths are counted correctly (the right marker must line up).
	marked := lipgloss.JoinHorizontal(lipgloss.Center, "◀ left ", box, " right ▶")
	body := lipgloss.JoinVertical(lipgloss.Center, "──── text above ────", marked, "──── text below ────")
	content := lipgloss.Place(m.w, max(m.h-4, 0), lipgloss.Center, lipgloss.Center, body) + "\n" + status

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// cellPixels reads the cell size from TIOCGWINSZ; kitty fills in pixels.
func cellPixels() (int, int) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return 10, 20
	}
	return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row)
}

func loadFont() (string, *sfnt.Font, error) {
	candidates := []string{
		"/usr/share/fonts/google-noto-sans-cjk-fonts/NotoSansCJK-Regular.ttc",
		"/run/host/usr/share/fonts/google-noto-sans-cjk-fonts/NotoSansCJK-Regular.ttc",
	}
	for _, p := range candidates {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		coll, err := opentype.ParseCollection(raw)
		if err != nil {
			return "", nil, fmt.Errorf("%s: %w", p, err)
		}
		for i := 0; i < coll.NumFonts(); i++ {
			f, err := coll.Font(i)
			if err != nil {
				continue
			}
			name, _ := f.Name(nil, sfnt.NameIDFamily)
			if name == "Noto Sans CJK JP" {
				return fmt.Sprintf("%s [%s]", filepath.Base(p), name), f, nil
			}
		}
	}
	return "", nil, fmt.Errorf("Noto Sans CJK JP not found in %v", candidates)
}

// rasterize draws text centred on a transparent w×h canvas, as large as fits.
func rasterize(f *sfnt.Font, text string, w, h int) (*image.RGBA, error) {
	n := len([]rune(text))
	size := min(float64(h)*0.85, float64(w)*0.9/float64(n))
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer face.Close()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	d := &font.Drawer{Dst: img, Src: image.NewUniform(color.White), Face: face}
	bounds, _ := d.BoundString(text)
	gw := (bounds.Max.X - bounds.Min.X).Ceil()
	gh := (bounds.Max.Y - bounds.Min.Y).Ceil()
	d.Dot = fixed.P((w-gw)/2-bounds.Min.X.Floor(), (h-gh)/2-bounds.Min.Y.Floor())
	d.DrawString(text)
	return img, nil
}
