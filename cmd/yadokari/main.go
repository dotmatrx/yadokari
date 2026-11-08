// Command yadokari is a terminal client for WaniKani reviews and lessons.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jtharris/yadokari/internal/config"
	"github.com/jtharris/yadokari/internal/gfx"
	"github.com/jtharris/yadokari/internal/glyph"
	"github.com/jtharris/yadokari/internal/store"
	"github.com/jtharris/yadokari/internal/ui"
	"github.com/jtharris/yadokari/internal/wk"
	"github.com/jtharris/yadokari/internal/wksync"
)

var version = "dev"

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
	live := flag.Bool("live", false, "allow writes to the live WaniKani API (default: dry-run)")
	base := flag.String("base", "", "API base URL (default: config api_base_url, else the live API)")
	dbPath := flag.String("db", "", "database path (default: ~/.local/share/yadokari/yadokari.db)")
	syncOnly := flag.Bool("sync", false, "sync, print a summary and exit (no TUI)")
	logout := flag.Bool("logout", false, "remove the stored API token and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("yadokari", version)
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

	ctx := context.Background()
	client, err := clientWithToken(ctx, tokens, baseURL, *live)
	if err != nil {
		return err
	}

	if *dbPath == "" {
		*dbPath = paths.Database
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
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
		return syncAndReport(ctx, syncer, st)
	}

	rdeps, warning := reviewDeps(client, st)
	if client.IsLive() && !*live {
		// Results would only pile up in the outbox and go stale.
		rdeps.Disabled = "Reviews against the live API need --live. Try them on the mock: just mock-recorded, then just dev."
	}
	if warning != "" {
		fmt.Fprintln(os.Stderr, "yadokari:", warning)
	}
	app := ui.NewApp(ui.Deps{Store: st, Syncer: syncer, Endpoint: endpoint}, rdeps)
	_, err = tea.NewProgram(app).Run()
	return err
}

// reviewDeps finds the font and graphics tier for the review screen. A
// missing font is not fatal: glyphs fall back to the terminal's own font.
func reviewDeps(client *wk.Client, st *store.Store) (ui.ReviewDeps, string) {
	rd := ui.ReviewDeps{Uploader: &wksync.Uploader{Client: client, Store: st}}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	rd.Images = glyph.ImageCache{Dir: filepath.Join(cacheDir, "yadokari", "images")}

	tier := gfx.Detect(os.Getenv)
	font, err := glyph.FindFont("")
	warning := ""
	if err != nil {
		warning = err.Error() + "; showing kanji in the terminal's font"
		tier = gfx.Plain
	}
	rd.Font = font
	cw, ch := gfx.CellSize()
	rd.Renderer = gfx.New(tier, cw, ch)
	return rd, warning
}

// clientWithToken loads the stored token, or runs the first-run form, checks
// the token against GET /user and saves it.
func clientWithToken(ctx context.Context, tokens config.TokenStore, baseURL string, live bool) (*wk.Client, error) {
	token, _, err := tokens.Load()
	if err == nil {
		return wk.New(token, wk.WithBaseURL(baseURL), wk.WithLiveWrites(live))
	}
	if !errors.Is(err, config.ErrNoToken) {
		return nil, err
	}

	token, err = ui.AskToken()
	if err != nil {
		return nil, err
	}
	client, err := wk.New(token, wk.WithBaseURL(baseURL), wk.WithLiveWrites(live))
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

func syncAndReport(ctx context.Context, syncer *wksync.Syncer, st *store.Store) error {
	start := time.Now()
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
