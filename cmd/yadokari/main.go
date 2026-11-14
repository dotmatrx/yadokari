// Command yadokari is a terminal client for WaniKani reviews and lessons.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jtharris/yadokari/internal/audio"
	"github.com/jtharris/yadokari/internal/config"
	"github.com/jtharris/yadokari/internal/gfx"
	"github.com/jtharris/yadokari/internal/glyph"
	"github.com/jtharris/yadokari/internal/store"
	"github.com/jtharris/yadokari/internal/ui"
	"github.com/jtharris/yadokari/internal/wk"
	"github.com/jtharris/yadokari/internal/wksync"
)

// version is set at build time from `git describe` (see the justfile).
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
	gfxFlag := flag.String("gfx", "auto", "graphics tier: auto, kitty or plain")
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
	client, err := clientWithToken(ctx, tokens, baseURL, wk.WithLiveWrites(*live), wk.WithWriteLog(writeLog), wk.WithVersion(version))
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

	rdeps, warning, err := reviewDeps(client, st, *gfxFlag)
	if err != nil {
		return err
	}
	if client.IsLive() && !*live {
		// Results would only pile up in the outbox and go stale.
		rdeps.Disabled = "Reviews and lessons against the live API need --live. Try them on the mock: just mock-recorded, then just dev."
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
func reviewDeps(client *wk.Client, st *store.Store, gfxTier string) (ui.ReviewDeps, string, error) {
	rd := ui.ReviewDeps{Uploader: &wksync.Uploader{Client: client, Store: st}}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	rd.Images = glyph.ImageCache{Dir: filepath.Join(cacheDir, "yadokari", "images")}
	rd.Sounds = glyph.ImageCache{Dir: filepath.Join(cacheDir, "yadokari", "audio")}
	if player, err := audio.Find(exec.LookPath); err == nil {
		rd.Audio = player
	} else {
		rd.AudioErr = err // shown when p is pressed; not worth a startup warning
	}

	// Ask the terminal at most once, and only if needed.
	var probed *gfx.Probed
	var probeErr error
	probe := func() (gfx.Probed, error) {
		if probed == nil {
			p, err := gfx.Probe(time.Second)
			probed, probeErr = &p, err
		}
		return *probed, probeErr
	}
	tier, err := gfx.Choose(gfxTier, os.Getenv, probe)
	if err != nil {
		return rd, "", err
	}
	font, err := glyph.FindFont("")
	warning := ""
	if err != nil {
		warning = err.Error() + "; showing kanji in the terminal's font"
		tier = gfx.Plain
	}
	rd.Font = font
	cw, ch := 0, 0
	if tier == gfx.Kitty {
		cw, ch = gfx.CellSize(probe)
	}
	rd.Renderer = gfx.New(tier, cw, ch)
	return rd, warning, nil
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
