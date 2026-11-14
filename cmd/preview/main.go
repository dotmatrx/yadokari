// Command preview renders yadokari's screens once, from demo data or a
// recording, for iterating on layout without driving the app:
//
//	preview [flags] [screen ...]      # default: every screen
//
// In a terminal it draws glyphs with the best tier the terminal supports
// (kitty graphics in Ghostty). With --html it writes a gallery page instead
// (plain tier, since a browser can't show kitty graphics).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jtharris/yadokari/internal/gfx"
	"github.com/jtharris/yadokari/internal/glyph"
	"github.com/jtharris/yadokari/internal/store"
	"github.com/jtharris/yadokari/internal/ui"
	"github.com/jtharris/yadokari/internal/wk"
	"github.com/jtharris/yadokari/internal/wkmock"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "preview:", err)
		os.Exit(1)
	}
}

type size struct{ w, h int }

func (s size) String() string { return fmt.Sprintf("%dx%d", s.w, s.h) }

func parseSizes(spec string) ([]size, error) {
	var out []size
	for _, part := range strings.Split(spec, ",") {
		w, h, ok := strings.Cut(strings.TrimSpace(part), "x")
		wi, err1 := strconv.Atoi(w)
		hi, err2 := strconv.Atoi(h)
		if !ok || err1 != nil || err2 != nil || wi < 20 || hi < 5 {
			return nil, fmt.Errorf("bad size %q (want e.g. 120x36)", part)
		}
		out = append(out, size{wi, hi})
	}
	return out, nil
}

func run() error {
	sizesFlag := flag.String("size", "", "comma-separated WxH sizes (default: the terminal's size; 120x36,80x24 for --html)")
	fixtures := flag.String("fixtures", "", "recording directory to use instead of demo data (personal data: don't publish)")
	htmlOut := flag.String("html", "", "write an HTML gallery to this file instead of drawing in the terminal")
	gfxFlag := flag.String("gfx", "auto", "graphics tier in the terminal: auto, kitty or plain")
	themeFlag := flag.String("theme", "", `colour theme: a name (wanikani, yadokari), or "all" for each in turn (default: wanikani)`)
	fontFlag := flag.String("font", "", `glyph font as the roulette would show it: a font file or installed family name, or "all" for each installed Japanese font in turn`)
	screens, err := parseArgs(flag.CommandLine, os.Args[1:])
	if err != nil {
		return err
	}
	if len(screens) == 0 {
		screens = ui.PreviewScreens
	}
	spec := *sizesFlag
	if spec == "" {
		if *htmlOut != "" {
			spec = "120x36,80x24"
		} else {
			spec = terminalSize()
		}
	}
	sizes, err := parseSizes(spec)
	if err != nil {
		return err
	}

	now := time.Now()
	data := wkmock.DemoData(now)
	if *fixtures != "" {
		if data, err = wkmock.LoadDir(*fixtures); err != nil {
			return err
		}
	}

	font, err := glyph.FindFont("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "preview:", err, "(glyphs in the terminal's font)")
	}
	fonts := []*glyph.Font{font}
	if *fontFlag != "" {
		if fonts, err = previewFonts(*fontFlag); err != nil {
			return err
		}
		font = fonts[0]
	}
	renderer := gfx.New(gfx.Plain, 0, 0)
	if *htmlOut == "" {
		probe := func() (gfx.Probed, error) { return gfx.Probe(time.Second) }
		tier, err := gfx.Choose(*gfxFlag, os.Getenv, probe)
		if err != nil {
			return err
		}
		if tier == gfx.Kitty && font != nil {
			cw, ch := gfx.CellSize(probe)
			renderer = gfx.New(gfx.Kitty, cw, ch)
		}
	}

	themes := []ui.Theme{ui.DefaultTheme}
	switch *themeFlag {
	case "":
	case "all":
		themes = ui.Themes
	default:
		t, ok := ui.ThemeNamed(*themeFlag)
		if !ok {
			return fmt.Errorf("no theme %q", *themeFlag)
		}
		themes = []ui.Theme{t}
	}

	ctx := context.Background()
	var shots []shot
	for _, sz := range sizes {
		for _, screen := range screens {
			for _, th := range themes {
				if th.UsesPalette() {
					th = th.InkFor(previewPalette(*htmlOut != ""), false)
				}
				ui.SetTheme(th)
				for _, f := range fonts {
					// A fresh store each time: review previews queue reviews.
					st, err := loadStore(ctx, data, now)
					if err != nil {
						return err
					}
					deps := ui.Deps{Store: st, Now: func() time.Time { return now }, Location: time.Local, Endpoint: "mock demo"}
					rd := ui.ReviewDeps{Renderer: renderer, Font: f}
					if *fontFlag != "" {
						rd.Roulette = glyph.PoolOf(f)
					}
					out, err := ui.Preview(ctx, deps, rd, screen, sz.w, sz.h)
					st.Close()
					if err != nil {
						return fmt.Errorf("%s at %s: %w", screen, sz, err)
					}
					label := screen
					if len(themes) > 1 || *themeFlag != "" {
						label += " · " + th.Name
					}
					shots = append(shots, shot{screen: label, size: sz, ansi: out})
					if *htmlOut == "" {
						fmt.Print(renderer.Flush())
						fmt.Printf("\x1b[2m── %s %s %s\x1b[m\n%s\n", label, sz, strings.Repeat("─", max(sz.w-len(label)-len(sz.String())-5, 0)), out)
					}
				}
			}
		}
	}
	if *htmlOut != "" {
		if err := os.WriteFile(*htmlOut, []byte(gallery(shots)), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %d screens to %s\n", len(shots), *htmlOut)
	}
	return nil
}

func terminalSize() string {
	cw, ch := 0, 0
	if ws, err := ttySize(); err == nil {
		cw, ch = ws[0], ws[1]
	}
	if cw < 20 || ch < 5 {
		return "120x36"
	}
	return fmt.Sprintf("%dx%d", cw, ch-2) // room for the label line and prompt
}

// loadStore fills an in-memory store with data, as a finished sync would.
func loadStore(ctx context.Context, d wkmock.Data, now time.Time) (*store.Store, error) {
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*store.Store, error) { st.Close(); return nil, err }
	for _, c := range []struct {
		name string
		rs   []store.RawResource
	}{
		{store.Subjects, raw(d.Subjects)},
		{store.Assignments, raw(d.Assignments)},
		{store.StudyMaterials, raw(d.StudyMaterials)},
		{store.SpacedRepetitionSystems, raw(d.SRS)},
		{store.ReviewStatistics, raw(d.ReviewStatistics)},
	} {
		if err := st.Upsert(ctx, c.name, c.rs); err != nil {
			return fail(err)
		}
	}
	userRaw, err := json.Marshal(d.User)
	if err != nil {
		return fail(err)
	}
	if err := st.PutSingleton(ctx, "user", store.Singleton{Raw: userRaw, FetchedAt: now}); err != nil {
		return fail(err)
	}
	if err := st.SetSyncState(ctx, store.Assignments, store.SyncState{SyncedAt: now.Add(-3 * time.Minute)}); err != nil {
		return fail(err)
	}
	return st, nil
}

func raw[T any](rs []wk.Resource[T]) []store.RawResource {
	out := make([]store.RawResource, len(rs))
	for i, r := range rs {
		data, _ := json.Marshal(r.Data)
		out[i] = store.RawResource{ID: r.ID, Object: r.Object, URL: r.URL, DataUpdatedAt: r.DataUpdatedAt, Data: data}
	}
	return out
}

// parseArgs parses flags wherever they appear (Go's flag package stops at
// the first non-flag), so `preview dashboard --size=80x24` works. A flag
// without "=" takes the next argument as its value unless it is boolean.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			rest = append(rest, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			continue
		}
		if f := fs.Lookup(name); f != nil && i+1 < len(args) {
			if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); !ok || !bf.IsBoolFlag() {
				i++
				flags = append(flags, args[i])
			}
		}
	}
	if err := fs.Parse(flags); err != nil {
		return nil, err
	}
	return append(rest, fs.Args()...), nil
}

// previewFonts loads the fonts --font names: one font, or "all" installed.
func previewFonts(name string) ([]*glyph.Font, error) {
	installed, err := glyph.ListFonts()
	if err != nil && name == "all" {
		return nil, err
	}
	refs := installed
	if name != "all" {
		if refs, err = glyph.SelectFonts(installed, []string{name}); err != nil {
			return nil, err
		}
	}
	var fonts []*glyph.Font
	for _, r := range refs {
		f, err := glyph.LoadFontAt(r)
		if err != nil {
			return nil, err
		}
		fonts = append(fonts, f)
	}
	if len(fonts) == 0 {
		return nil, fmt.Errorf("no Japanese fonts installed")
	}
	return fonts, nil
}

// previewPalette is the palette the "terminal" theme is drawn with: the
// gallery's (html.go's basic colours), or the terminal's own.
func previewPalette(html bool) [16]color.RGBA {
	var pal [16]color.RGBA
	if html {
		for i, hex := range basic {
			var r, g, b uint8
			fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
			pal[i] = color.RGBA{r, g, b, 0xff}
		}
		return pal
	}
	if p, err := gfx.Probe(time.Second); err == nil {
		pal = p.Palette
	}
	return pal
}
