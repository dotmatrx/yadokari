package gfx

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// --- Plain ----------------------------------------------------------------

type plain struct{}

func (plain) Tier() Tier    { return Plain }
func (plain) Flush() string { return "" }
func (plain) Reset() string { return "" }
func (plain) Draw(src Source, cols, rows int, fg, bg color.Color) (string, error) {
	text := src.Text
	if text == "" {
		text = "（画像）" // "(image)": an image-only radical, never its name
	}
	st := lipgloss.NewStyle().Bold(true).Foreground(fg).Background(bg)
	return lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center, st.Render(text),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(bg))), nil
}

// --- Kitty ------------------------------------------------------------------

// kittyRenderer uploads each image once and shows it through Unicode
// placeholder cells whose foreground colour (256-colour index) is the image
// ID. IDs stay below 256 so that encoding is exact.
type kittyRenderer struct {
	cellW, cellH int
	ids          map[string]int // cache key → image ID
	order        []string       // keys, oldest first, for reuse
	pending      strings.Builder
}

const maxKittyImages = 200

func newKitty(cellW, cellH int) *kittyRenderer {
	return &kittyRenderer{cellW: max(cellW, 1), cellH: max(cellH, 1), ids: map[string]int{}}
}

func (k *kittyRenderer) Tier() Tier { return Kitty }

func (k *kittyRenderer) Flush() string {
	s := k.pending.String()
	k.pending.Reset()
	return s
}

func (k *kittyRenderer) Reset() string {
	var b strings.Builder
	for _, id := range k.ids {
		b.WriteString(ansi.KittyGraphics(nil, "a=d", "d=I", fmt.Sprintf("i=%d", id), "q=2"))
	}
	k.ids, k.order = map[string]int{}, nil
	k.pending.Reset()
	return b.String()
}

func (k *kittyRenderer) Draw(src Source, cols, rows int, fg, bg color.Color) (string, error) {
	fr, fgg, fb := rgb(fg)
	key := fmt.Sprintf("%s@%dx%d@%dx%d#%02x%02x%02x", src.Key, cols, rows, k.cellW, k.cellH, fr, fgg, fb)
	id, ok := k.ids[key]
	if !ok {
		var err error
		if id, err = k.upload(key, src, cols, rows, fg); err != nil {
			return "", err
		}
	}
	return placeholders(id, cols, rows, bg), nil
}

// upload rasterizes the source, assigns an image ID and queues the upload.
func (k *kittyRenderer) upload(key string, src Source, cols, rows int, fg color.Color) (int, error) {
	mask, err := maskOf(src, cols*k.cellW, rows*k.cellH)
	if err != nil {
		return 0, err
	}
	img := image.NewNRGBA(mask.Bounds())
	r, g, b := rgb(fg)
	for i, a := range mask.Pix {
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = r, g, b, a
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return 0, err
	}

	// Pick an ID: the next unused one, or recycle the oldest.
	id := 0
	if len(k.order) >= maxKittyImages {
		oldest := k.order[0]
		id = k.ids[oldest]
		delete(k.ids, oldest)
		k.order = k.order[1:]
	} else {
		used := map[int]bool{}
		for _, v := range k.ids {
			used[v] = true
		}
		for id = 1; used[id]; id++ {
		}
	}

	var seq strings.Builder
	seq.WriteString(ansi.KittyGraphics(nil, "a=d", "d=I", fmt.Sprintf("i=%d", id), "q=2"))
	err = kitty.EncodeGraphics(&seq, img, &kitty.Options{
		Action:           kitty.TransmitAndPut,
		Transmission:     kitty.Direct, // zero value sends no pixels (Spike 1)
		Format:           kitty.PNG,
		ID:               id,
		Columns:          cols,
		Rows:             rows,
		VirtualPlacement: true,
		Quiet:            2,
		Chunk:            true,
	})
	if err != nil {
		return 0, err
	}
	k.ids[key] = id
	k.order = append(k.order, key)
	k.pending.WriteString(seq.String())
	return id, nil
}

func placeholders(id, cols, rows int, bg color.Color) string {
	sgr := fmt.Sprintf("\x1b[38;5;%dm", id) + bgSGR(bg)
	var b strings.Builder
	for r := range rows {
		b.WriteString(sgr)
		for c := range cols {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(r))
			b.WriteRune(kitty.Diacritic(c))
		}
		b.WriteString("\x1b[m")
		if r < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// maskOf calls src.Mask and checks it honoured the requested size.
func maskOf(src Source, w, h int) (*image.Alpha, error) {
	m, err := src.Mask(w, h)
	if err != nil {
		return nil, err
	}
	if b := m.Bounds(); b.Dx() != w || b.Dy() != h || b.Min != (image.Point{}) {
		return nil, fmt.Errorf("gfx: %s: mask is %v, want %dx%d", src.Key, b, w, h)
	}
	return m, nil
}
