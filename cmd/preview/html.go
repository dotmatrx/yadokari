package main

import (
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"golang.org/x/sys/unix"
)

type shot struct {
	screen string
	size   size
	ansi   string
}

func ttySize() ([2]int, error) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return [2]int{}, err
	}
	return [2]int{int(ws.Col), int(ws.Row)}, nil
}

// gallery lays the shots out as terminal panes, grouped by size. Panes are
// a dark terminal, or a light one; the page around them follows the
// viewer's theme.
func gallery(shots []shot, light bool) string {
	var b strings.Builder
	b.WriteString(galleryHead)
	if light {
		b.WriteString(`<style>.term { background: #fafafa; color: #2b2f36; }</style>`)
	}
	b.WriteString(`<main><header><h1>yadokari screens</h1><p>Rendered from the real Lip Gloss output with demo data. Glyphs are the kitty tier's images (the plain tier with --gfx plain).</p></header>`)
	var last size
	for _, s := range shots {
		if s.size != last {
			if last != (size{}) {
				b.WriteString(`</section>`)
			}
			fmt.Fprintf(&b, `<section><h2>%d × %d</h2>`, s.size.w, s.size.h)
			last = s.size
		}
		fmt.Fprintf(&b, `<figure><figcaption>%s</figcaption><div class="scroll"><pre class="term" style="width:%dch">%s</pre></div></figure>`,
			html.EscapeString(s.screen), s.size.w, ansiToHTML(s.ansi))
	}
	if last != (size{}) {
		b.WriteString(`</section>`)
	}
	b.WriteString(`</main>`)
	return b.String()
}

const galleryHead = `<title>yadokari Screens</title>
<meta name="claude:viewport-fit-sides" content="cover">
<style>
/* Layout: a quiet page of terminal panes, one section per terminal size. */
:root {
  --page: #f3f4f6; --ink: #1d2128; --muted: #5d6573; --line: #d5d9e0;
  --term: #15171c; --term-ink: #d7dae0; --term-edge: #2a2e36;
  /* Installed fonts only. JetBrains Mono (or its Nerd Font build) has box
     drawing and block characters a cell wide that join at its line height,
     1.32; the product site self-hosts it. Other fonts may leave gaps. */
  --mono: "JetBrains Mono", "JetBrainsMono Nerd Font Mono", "JetBrainsMono NFM", "DejaVu Sans Mono", "Noto Sans Mono CJK JP", monospace;
  --sans: system-ui, sans-serif;
}
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) {
  --page: #0e1014; --ink: #e3e6eb; --muted: #9098a6; --line: #262a32; --term-edge: #30353e; color-scheme: dark } }
:root[data-theme="dark"] {
  --page: #0e1014; --ink: #e3e6eb; --muted: #9098a6; --line: #262a32; --term-edge: #30353e; color-scheme: dark }
body { background: var(--page); color: var(--ink); font-family: var(--sans); }
main { padding-block: 24px 48px; padding-left: max(16px, env(safe-area-inset-left, 0px)); padding-right: max(16px, env(safe-area-inset-right, 0px)); display: grid; gap: 32px; }
header h1 { font-size: 1.4rem; margin: 0 0 4px; text-wrap: balance; }
header p { margin: 0; color: var(--muted); max-width: 65ch; line-height: 1.5; }
section { display: grid; gap: 20px; }
h2 { font-size: .8rem; letter-spacing: .08em; text-transform: uppercase; color: var(--muted); margin: 0; padding-bottom: 6px; border-bottom: 1px solid var(--line); font-variant-numeric: tabular-nums; }
figure { margin: 0; display: grid; gap: 6px; min-width: 0; }
figcaption { font-family: var(--mono); font-size: .8rem; color: var(--muted); }
.scroll { overflow-x: auto; max-width: 100%; }
.term { margin: 0; background: var(--term); color: var(--term-ink); border: 1px solid var(--term-edge); border-radius: 6px;
  padding: 10px 12px; box-sizing: content-box; font-family: var(--mono); font-size: 13px; line-height: 1.32; white-space: pre; }
.term .w { display: inline-block; width: 2ch; text-align: center; }
.term .n { display: inline-block; white-space: pre; }
.term .g { position: relative; } .term .g img { position: absolute; left: 0; top: 0; }
.term .b { font-weight: 700; } .term .f { opacity: .55; } .term .u { text-decoration: underline; }
</style>
`

// ansiToHTML converts the SGR styling Lip Gloss emits into spans; other
// escape sequences are dropped. Wide characters get a 2ch box so columns
// line up in the browser as they do in the terminal. Kitty graphics become
// images: uploads (APC) are collected by ID, and each placeholder grid
// shows its image over its cells, from the top-left one.
func ansiToHTML(s string) string {
	var out strings.Builder
	var st sgr
	images := map[int]*kittyImage{}
	var cur *kittyImage // the upload whose chunks are arriving
	// run holds narrow non-ASCII characters (box drawing, blocks): written
	// as one box exactly len(run) ch wide, so a fallback font whose glyphs
	// aren't 1ch can't push the rest of the row out of line.
	var run []rune
	endRun := func() {
		if len(run) > 0 {
			fmt.Fprintf(&out, `<span class="n" style="width:%dch">%s</span>`, len(run), html.EscapeString(string(run)))
			run = run[:0]
		}
	}
	open := false
	flush := func() {
		endRun()
		if open {
			out.WriteString("</span>")
			open = false
		}
		if cls, style := st.css(); cls != "" || style != "" {
			fmt.Fprintf(&out, `<span class="%s" style="%s">`, cls, style)
			open = true
		}
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		narrow := r > 0x7f && r != kitty.Placeholder && ansi.StringWidth(string(r)) == 1
		if narrow && r != 0x1b {
			run = append(run, r)
			continue
		}
		if r == 0x1b && i+1 < len(rs) {
			switch rs[i+1] {
			case '[': // CSI: params then a final byte in @–~
				j := i + 2
				for j < len(rs) && (rs[j] < 0x40 || rs[j] > 0x7e) {
					j++
				}
				if j < len(rs) && rs[j] == 'm' {
					st.apply(string(rs[i+2 : j]))
					flush()
				}
				i = j
			case ']', '_', 'P': // OSC, APC (kitty), DCS: skip to ST or BEL
				j := i + 2
				for j < len(rs) && rs[j] != 0x07 && !(rs[j] == 0x1b && j+1 < len(rs) && rs[j+1] == '\\') {
					j++
				}
				if rs[i+1] == '_' && i+2 < len(rs) && rs[i+2] == 'G' {
					cur = kittyChunk(string(rs[i+3:j]), images, cur)
				}
				if j < len(rs) && rs[j] == 0x1b {
					j++
				}
				i = j
			default:
				i++
			}
			continue
		}
		endRun()
		if r == kitty.Placeholder {
			// The cell's row and column ride on two diacritics.
			row, col := -1, -1
			if i+2 < len(rs) {
				row, col = diacritic[rs[i+1]], diacritic[rs[i+2]]
				i += 2
			}
			img := images[st.index]
			if row == 0 && col == 0 && img != nil {
				fmt.Fprintf(&out, `<span class="g"> <img alt="" src="data:image/png;base64,%s" style="width:%dch;height:%dlh"></span>`,
					img.data.String(), img.cols, img.rows)
			} else {
				out.WriteByte(' ')
			}
			continue
		}
		text := html.EscapeString(string(r))
		if r != '\n' && ansi.StringWidth(string(r)) == 2 {
			text = `<span class="w">` + text + `</span>`
		}
		out.WriteString(text)
	}
	endRun()
	if open {
		out.WriteString("</span>")
	}
	return out.String()
}

type sgr struct {
	bold, faint, under bool
	fg, bg             string
	index              int // the last 38;5 colour: a kitty placeholder's image ID
}

// kittyImage is one kitty upload: a PNG in base64, for cols×rows cells.
type kittyImage struct {
	cols, rows int
	data       strings.Builder
	done       bool
}

// kittyChunk reads one kitty graphics command ("keys;payload"): an upload's
// first chunk (with i=, c=, r=) starts an image, later chunks (keys m= only)
// add to cur. Deletes and other commands are ignored. It returns the image
// still expecting chunks, if any.
func kittyChunk(cmd string, images map[int]*kittyImage, cur *kittyImage) *kittyImage {
	keys, payload, _ := strings.Cut(cmd, ";")
	kv := map[string]string{}
	for _, f := range strings.Split(keys, ",") {
		if k, v, ok := strings.Cut(f, "="); ok {
			kv[k] = v
		}
	}
	if kv["a"] == "d" {
		return cur
	}
	if id, err := strconv.Atoi(kv["i"]); err == nil {
		cols, _ := strconv.Atoi(kv["c"])
		rows, _ := strconv.Atoi(kv["r"])
		cur = &kittyImage{cols: cols, rows: rows}
		images[id] = cur
	}
	if cur == nil || cur.done {
		return nil
	}
	cur.data.WriteString(payload)
	if kv["m"] != "1" {
		cur.done = true
		return nil
	}
	return cur
}

// diacritic maps kitty's row/column diacritics back to their numbers.
var diacritic = func() map[rune]int {
	m := map[rune]int{}
	for i := 0; i < 297; i++ {
		m[kitty.Diacritic(i)] = i
	}
	return m
}()

func (s sgr) css() (cls, style string) {
	var c []string
	if s.bold {
		c = append(c, "b")
	}
	if s.faint {
		c = append(c, "f")
	}
	if s.under {
		c = append(c, "u")
	}
	if s.fg != "" {
		style += "color:" + s.fg + ";"
	}
	if s.bg != "" {
		style += "background:" + s.bg + ";"
	}
	return strings.Join(c, " "), style
}

var basic = []string{"#1d1f21", "#cc6666", "#b5bd68", "#f0c674", "#81a2be", "#b294bb", "#8abeb7", "#c5c8c6",
	"#666666", "#d54e53", "#b9ca4a", "#e7c547", "#7aa6da", "#c397d8", "#70c0b1", "#eaeaea"}

func (s *sgr) apply(params string) {
	if params == "" {
		*s = sgr{}
		return
	}
	ps := strings.FieldsFunc(params, func(r rune) bool { return r == ';' || r == ':' })
	num := func(i int) int {
		if i >= len(ps) {
			return 0
		}
		n, _ := strconv.Atoi(ps[i])
		return n
	}
	for i := 0; i < len(ps); i++ {
		switch n := num(i); {
		case n == 0:
			*s = sgr{}
		case n == 1:
			s.bold = true
		case n == 2:
			s.faint = true
		case n == 4:
			s.under = true
		case n == 22:
			s.bold, s.faint = false, false
		case n == 24:
			s.under = false
		case n == 39:
			s.fg = ""
		case n == 49:
			s.bg = ""
		case n >= 30 && n <= 37:
			s.fg = basic[n-30]
		case n >= 90 && n <= 97:
			s.fg = basic[n-90+8]
		case n >= 40 && n <= 47:
			s.bg = basic[n-40]
		case n >= 100 && n <= 107:
			s.bg = basic[n-100+8]
		case n == 38 || n == 48:
			var col string
			index := -1
			switch num(i + 1) {
			case 2:
				col = fmt.Sprintf("rgb(%d,%d,%d)", num(i+2), num(i+3), num(i+4))
				i += 4
			case 5:
				index = num(i + 2)
				if c := num(i + 2); c < 16 {
					col = basic[c]
				} else {
					col = xterm256(c)
				}
				i += 2
			}
			if n == 38 {
				s.fg, s.index = col, index
			} else {
				s.bg = col
			}
		}
	}
}

func xterm256(c int) string {
	if c >= 232 {
		v := 8 + (c-232)*10
		return fmt.Sprintf("rgb(%d,%d,%d)", v, v, v)
	}
	c -= 16
	lv := []int{0, 95, 135, 175, 215, 255}
	return fmt.Sprintf("rgb(%d,%d,%d)", lv[c/36], lv[c/6%6], lv[c%6])
}
