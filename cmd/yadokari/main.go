// Command yadokari is a terminal client for WaniKani reviews and lessons.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dotmatrx/yadokari/internal/audio"
	"github.com/dotmatrx/yadokari/internal/config"
	"github.com/dotmatrx/yadokari/internal/gfx"
	"github.com/dotmatrx/yadokari/internal/glyph"
	"github.com/dotmatrx/yadokari/internal/store"
	"github.com/dotmatrx/yadokari/internal/ui"
	"github.com/dotmatrx/yadokari/internal/wk"
	"github.com/dotmatrx/yadokari/internal/wksync"
)

// version is stamped at build time (`just install`, the release builds).
// Unstamped builds take the module version: a release tag for
// `go install …@v1.2.3`, else a development version (v1.2.3+dirty, a
// pseudo-version) or "dev".
var version = "dev"

// liveWrites says whether this build may write to the live WaniKani API
// (submit reviews, start lessons; irreversible). Builds made to be used
// may; development builds (go run, just run/dev/build, tests) never can,
// whatever their flags, so trying a change can't touch a real account.
var liveWrites bool

func init() {
	module := ""
	if bi, ok := debug.ReadBuildInfo(); ok {
		module = bi.Main.Version
	}
	version, liveWrites = buildKind(version, module)
}

// pseudoVersion matches the tail Go gives a commit that isn't a tag.
var pseudoVersion = regexp.MustCompile(`\d{14}-[0-9a-f]{12}$`)

// buildKind decides the version shown and whether the build is one made to
// be used: stamped at build time, or a module built at a clean release tag.
func buildKind(stamped, module string) (string, bool) {
	if stamped != "dev" {
		return stamped, true
	}
	switch {
	case module == "" || module == "(devel)":
		return "dev", false
	case strings.HasPrefix(module, "v") && !strings.Contains(module, "+") && !pseudoVersion.MatchString(module):
		return module, true
	}
	return module, false
}

// disclaimer goes wherever the app introduces itself.
const disclaimer = "yadokari is an unofficial WaniKani client, not affiliated with WaniKani or Tofugu LLC."

func main() {
	if err := run(); err != nil {
		if !errors.Is(err, ui.ErrAborted) {
			fmt.Fprintln(os.Stderr, "yadokari:", err)
		}
		os.Exit(1)
	}
}

func run() error {
	showVersion := flag.Bool("version", false, "print version and exit")
	// Writes to the real WaniKani API are irreversible. Nothing may submit
	// reviews or start assignments unless this flag is set.
	base := flag.String("base", "", "API base URL (default: config api_base_url, else the live API)")
	dbPath := flag.String("db", "", "database path (default: ~/.local/share/yadokari/yadokari.db)")
	syncOnly := flag.Bool("sync", false, "sync, print a summary and exit (no TUI)")
	logout := flag.Bool("logout", false, "remove the stored API token and exit")
	terminal := flag.Bool("terminal", false, "show what yadokari found out about the terminal (graphics, background, font) and exit")
	gfxFlag := flag.String("gfx", "", "graphics tier: auto, kitty or plain (default: config display.gfx, else auto)")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage: yadokari [flags]\n\n%s\n\nFlags:\n", disclaimer)
		flag.PrintDefaults()
		fmt.Fprintf(out, "\nSettings: ~/.config/yadokari/config.toml (every key is documented in\ndocs/config.example.toml, https://github.com/dotmatrx/yadokari).\n")
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("yadokari", version)
		fmt.Println(disclaimer)
		return nil
	}

	paths, err := config.DefaultPaths(config.OSEnv)
	if err != nil {
		return err
	}
	cfg, err := config.Load(paths.Config)
	if err != nil {
		return err
	}
	if *terminal {
		return terminalReport(cfg, paths.Config, cmp.Or(*gfxFlag, cfg.Display.Gfx, "auto"))
	}
	tokens := config.TokenStore{Keyring: config.OSKeyring, File: paths.TokenFile, Getenv: os.Getenv}
	if *logout {
		if err := tokens.Delete(); err != nil {
			return err
		}
		fmt.Println("Removed the stored API token.")
		return nil
	}

	baseURL := wk.DefaultBaseURL
	switch {
	case *base != "":
		baseURL = *base
	case cfg.APIBaseURL != "":
		baseURL = cfg.APIBaseURL
	}

	if *dbPath == "" {
		*dbPath = paths.Database
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
		return err
	}
	// Every write attempt and WaniKani's reply, next to the database.
	writeLog, err := os.OpenFile(filepath.Join(filepath.Dir(*dbPath), "writes.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer writeLog.Close()
	// Problems the UI reports in plain words, with the full error, here.
	appLog, err := os.OpenFile(filepath.Join(filepath.Dir(*dbPath), "yadokari.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer appLog.Close()
	slog.SetDefault(slog.New(slog.NewTextHandler(appLog, nil)).With("version", version))

	ctx := context.Background()
	client, err := clientWithToken(ctx, tokens, baseURL, wk.WithLiveWrites(liveWrites), wk.WithWriteLog(writeLog), wk.WithVersion(version))
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	syncer := &wksync.Syncer{Client: client, Store: st}
	endpoint := "live API"
	if !client.IsLive() {
		u, _ := url.Parse(baseURL)
		endpoint = "mock " + u.Host
	}
	if *syncOnly {
		return syncAndReport(ctx, syncer, &wksync.Uploader{Client: client, Store: st})
	}

	if *gfxFlag == "" {
		*gfxFlag = cmp.Or(cfg.Display.Gfx, "auto")
	}
	rdeps, warning, err := reviewDeps(client, st, cfg, *gfxFlag)
	if err != nil {
		return err
	}
	if client.IsLive() && !liveWrites {
		// Results would only pile up in the outbox and go stale.
		rdeps.Disabled = "This is a development build: it never sends answers to WaniKani. Use the mock (just dev), or install a build (just install)."
	}
	if warning != "" {
		fmt.Fprintln(os.Stderr, "yadokari:", warning)
	}
	app := ui.NewApp(ui.Deps{Store: st, Syncer: syncer, Uploader: rdeps.Uploader, Endpoint: endpoint, Version: version}, rdeps)
	_, err = tea.NewProgram(app).Run()
	return err
}

// reviewDeps finds the font and graphics tier for the review screen. A
// missing font is not fatal: glyphs fall back to the terminal's own font.
func reviewDeps(client *wk.Client, st *store.Store, cfg config.Config, gfxTier string) (ui.ReviewDeps, string, error) {
	rd := ui.ReviewDeps{
		Uploader: &wksync.Uploader{Client: client, Store: st},
		Settings: ui.Settings{Reviews: cfg.Reviews, Lessons: cfg.Lessons},
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	rd.Images = glyph.ImageCache{Dir: filepath.Join(cacheDir, "yadokari", "images")}
	rd.Sounds = glyph.ImageCache{Dir: filepath.Join(cacheDir, "yadokari", "audio")}
	if cfg.Audio.Player != "" {
		rd.Audio = audio.Command(cfg.Audio.Player)
	} else if player, err := audio.Find(exec.LookPath); err == nil {
		rd.Audio = player
	} else {
		rd.AudioErr = err // shown when p is pressed; not worth a startup warning
	}

	d, err := setupDisplay(cfg, gfxTier)
	if err != nil {
		return rd, "", err
	}
	slog.Info("terminal", "terminal", cmp.Or(d.terminal, "unknown"), "tier", d.tier, "why", d.tierWhy, "light", d.bg.Light, "background", hexOr(d.bg.Colour, "unknown"),
		"text", hexOr(d.bg.Text, "unknown"), "from", d.bg.From)
	warning := ""
	if d.fontErr != nil {
		warning = d.fontErr.Error() + "; showing kanji in the terminal's font"
	}
	rd.Font = d.font
	if d.tier == gfx.Kitty {
		if cfg.Display.Roulette {
			pool, err := roulette(cfg.Display.RouletteFonts)
			if err != nil {
				return rd, "", fmt.Errorf("config: display.roulette_fonts: %w", err)
			}
			rd.Roulette = pool
		}
	}
	rd.Renderer = gfx.New(d.tier, d.cellW, d.cellH)
	return rd, warning, nil
}

// display is what the terminal and the config say about drawing.
type display struct {
	tier         gfx.Tier
	tierWhy      string
	terminal     string // name and version, if the terminal said (XTVERSION)
	cellW, cellH int
	font         *glyph.Font
	fontErr      error
	theme        ui.Theme
	bg           ui.Background
}

// setupDisplay asks the terminal (at most once, before Bubble Tea starts),
// picks the graphics tier, font and theme, and sets the UI's theme and
// background.
func setupDisplay(cfg config.Config, gfxTier string) (display, error) {
	var d display
	var probed *gfx.Probed
	var probeErr error
	probe := func() (gfx.Probed, error) {
		if probed == nil {
			p, err := gfx.Probe(time.Second)
			probed, probeErr = &p, err
		}
		return *probed, probeErr
	}
	var err error
	if d.tier, d.tierWhy, err = gfx.ChooseWhy(gfxTier, os.Getenv, probe); err != nil {
		return d, err
	}
	if probed != nil {
		d.terminal = probed.Terminal
	}
	if d.font, d.fontErr = glyph.FindFont(cfg.Display.Font); d.fontErr != nil {
		d.tier, d.tierWhy = gfx.Plain, "no Japanese font found"
	}
	d.bg = ui.DetectBackground(cfg.Display.Background, os.Getenv, probe)
	th, _ := ui.ThemeNamed(cmp.Or(cfg.Display.Theme, ui.DefaultTheme.Name))
	th = th.ForBackground(d.bg.Light).WithColours(cfg.Theme)
	if th.UsesPalette() {
		// The glyph fields are the terminal's colours: ask for their shades
		// to pick an ink that reads on them.
		if p, err := probe(); err == nil {
			th = th.InkFor(p.Palette, cfg.Theme.Ink != "")
		}
	}
	d.theme = th
	ui.SetTheme(th)
	ui.SetBackground(d.bg)
	if d.tier == gfx.Kitty {
		d.cellW, d.cellH = gfx.CellSize(probe)
	}
	return d, nil
}

func hexOr(c color.RGBA, unknown string) string {
	if c.A == 0 {
		return unknown
	}
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// terminalReport prints what yadokari found out about the terminal and
// what it chose, for yadokari --terminal.
func terminalReport(cfg config.Config, configPath, gfxTier string) error {
	d, err := setupDisplay(cfg, gfxTier)
	if err != nil {
		return err
	}
	row := func(k, v string) { fmt.Printf("%-11s %s\n", k, v) }
	if d.terminal != "" {
		row("terminal", d.terminal)
	}
	row("graphics", fmt.Sprintf("%s (%s)", d.tier, d.tierWhy))
	if d.tier == gfx.Kitty {
		row("cell size", fmt.Sprintf("%d×%d px", d.cellW, d.cellH))
	}
	if d.fontErr != nil {
		row("font", "none found: kanji are drawn in the terminal's own font")
	} else {
		row("font", d.font.Name())
	}
	bg := "dark"
	if d.bg.Light {
		bg = "light"
	}
	if d.bg.Colour.A != 0 {
		row("background", fmt.Sprintf("%s → %s (%s)", hexOr(d.bg.Colour, ""), bg, d.bg.From))
	} else {
		row("background", fmt.Sprintf("%s (%s)", bg, d.bg.From))
	}
	ink := hexOr(ui.FrameInk(), "") + " (white on dark)"
	switch {
	case cfg.Theme.Ink != "":
		ink = hexOr(ui.FrameInk(), "") + " (set in the config file)"
	case d.bg.Text.A != 0:
		ink = hexOr(ui.FrameInk(), "") + " (the terminal's text colour)"
	case d.bg.Light:
		ink = hexOr(ui.FrameInk(), "") + " (near-black on light)"
	}
	if d.tier == gfx.Plain && cfg.Theme.Ink == "" {
		ink = "the terminal's own text colour"
	}
	row("glyph ink", ink)
	if d.theme.Light {
		row("theme", d.theme.Name+", light colours")
	} else {
		row("theme", d.theme.Name)
	}
	if d.bg.From == "not reported; assumed dark" {
		fmt.Println("\nYour terminal didn't say what its background is. If it is light, add")
		fmt.Println(`  [display]
  background = "light"`)
		fmt.Println("to " + configPath + ".")
	}
	return nil
}

// roulette is the font pool for display.roulette: the fonts named, or every
// installed Japanese font.
func roulette(names []string) (*glyph.Pool, error) {
	installed, err := glyph.ListFonts()
	if err != nil {
		slog.Warn("font roulette: can't list fonts", "err", err)
	}
	refs, err := glyph.SelectFonts(installed, names)
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("no Japanese fonts found; name font files instead")
	}
	slog.Info("font roulette", "fonts", len(refs))
	return glyph.NewPool(refs), nil
}

// clientWithToken loads the stored token, or runs the first-run form, checks
// the token against GET /user and saves it.
func clientWithToken(ctx context.Context, tokens config.TokenStore, baseURL string, opts ...wk.Option) (*wk.Client, error) {
	opts = append([]wk.Option{wk.WithBaseURL(baseURL)}, opts...)
	token, _, err := tokens.Load()
	if err == nil {
		return wk.New(token, opts...)
	}
	if !errors.Is(err, config.ErrNoToken) {
		return nil, err
	}

	token, err = ui.AskToken()
	if err != nil {
		return nil, err
	}
	client, err := wk.New(token, opts...)
	if err != nil {
		return nil, err
	}
	u, _, err := client.User(ctx, wk.Conditional{})
	if wk.IsStatus(err, 401) {
		return nil, errors.New("WaniKani rejected that token (401); check it and run yadokari again")
	}
	if err != nil {
		return nil, fmt.Errorf("checking the token: %w", err)
	}
	src, err := tokens.Save(token)
	if err != nil {
		return nil, fmt.Errorf("saving the token: %w", err)
	}
	fmt.Printf("Hello, %s. Token saved to the %s.\n", u.Data.Username, src)
	return client, nil
}

func syncAndReport(ctx context.Context, syncer *wksync.Syncer, up *wksync.Uploader) error {
	start, st := time.Now(), up.Store
	res, err := up.Flush(ctx)
	switch {
	case errors.Is(err, wksync.ErrHeld):
		fmt.Fprintf(os.Stderr, "%d queued uploads held: %v\n", res.Held, err)
	case err != nil:
		fmt.Fprintf(os.Stderr, "uploading queued reviews: %v\n", err)
	case res.Sent+res.Rejected > 0:
		fmt.Fprintf(os.Stderr, "uploaded %d queued reviews, %d rejected\n", res.Sent, res.Rejected)
	}
	syncer.Progress = func(e wksync.Event) {
		if e.Done {
			fmt.Fprintf(os.Stderr, "  %-26s %6d\n", e.Collection, e.Fetched)
		}
	}
	all := append(append([]string{}, wksync.Foreground...), wksync.Background...)
	if _, err := syncer.Run(ctx, all); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "synced in %v, %d requests\n", time.Since(start).Round(time.Millisecond), syncer.Client.Requests())

	u, err := st.User(ctx)
	if err != nil || u == nil {
		return fmt.Errorf("no user after sync: %v", err)
	}
	q, err := st.Queue(ctx, time.Now(), u.Data.Subscription.MaxLevelGranted)
	if err != nil {
		return err
	}
	p, err := st.LevelProgress(ctx, u.Data.Level)
	if err != nil {
		return err
	}
	due := 0
	for _, n := range q.Forecast {
		due += n
	}
	fmt.Printf("%s, level %d: %d lessons, %d reviews now, %d more in 24 h; level kanji %d/%d passed (%d needed)\n",
		u.Data.Username, u.Data.Level, q.Lessons, q.Reviews, due, p.Passed, p.Kanji, p.Needed)
	return nil
}
