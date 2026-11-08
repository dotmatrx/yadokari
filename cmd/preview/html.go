package main

import (
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
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
// always a dark terminal; the page around them follows the viewer's theme.
func gallery(shots []shot) string {
	var b strings.Builder
	b.WriteString(galleryHead)
	b.WriteString(`<main><header><h1>yadokari screens</h1><p>Rendered from the real Lip Gloss output with demo data. Glyphs use the plain tier here; in Ghostty they are kitty graphics.</p></header>`)
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
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;700&family=Noto+Sans+JP:wght@400;700&family=IBM+Plex+Sans:wght@400;600&display=swap">
<style>
/* Layout: a quiet page of terminal panes, one section per terminal size. */
:root {
  --page: #f3f4f6; --ink: #1d2128; --muted: #5d6573; --line: #d5d9e0;
  --term: #15171c; --term-ink: #d7dae0; --term-edge: #2a2e36;
  --mono: "JetBrains Mono", "DejaVu Sans Mono", "Noto Sans JP", monospace;
  --sans: "IBM Plex Sans", system-ui, sans-serif;
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
  padding: 10px 12px; box-sizing: content-box; font-family: var(--mono); font-size: 13px; line-height: 1.25; white-space: pre; }
.term .w { display: inline-block; width: 2ch; text-align: center; }
.term .b { font-weight: 700; } .term .f { opacity: .55; } .term .u { text-decoration: underline; }
</style>
`

// ansiToHTML converts the SGR styling Lip Gloss emits into spans; other
// escape sequences are dropped. Wide characters get a 2ch box so columns
// line up in the browser as they do in the terminal.
func ansiToHTML(s string) string {
	var out strings.Builder
	var st sgr
	open := false
	flush := func() {
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
				if j < len(rs) && rs[j] == 0x1b {
					j++
				}
				i = j
			default:
				i++
			}
			continue
		}
		text := html.EscapeString(string(r))
		if r != '\n' && ansi.StringWidth(string(r)) == 2 {
			text = `<span class="w">` + text + `</span>`
		}
		out.WriteString(text)
	}
	if open {
		out.WriteString("</span>")
	}
	return out.String()
}

type sgr struct {
	bold, faint, under bool
	fg, bg             string
}

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
			switch num(i + 1) {
			case 2:
				col = fmt.Sprintf("rgb(%d,%d,%d)", num(i+2), num(i+3), num(i+4))
				i += 4
			case 5:
				if c := num(i + 2); c < 16 {
					col = basic[c]
				} else {
					col = xterm256(c)
				}
				i += 2
			}
			if n == 38 {
				s.fg = col
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
