// Package config locates yadokari's files, loads settings and stores the
// WaniKani API token.
//
// Locations follow the XDG base directories:
//
//	config  $XDG_CONFIG_HOME/yadokari/config.toml   (~/.config)
//	token   OS keyring, else $XDG_CONFIG_HOME/yadokari/token (mode 0600)
//	data    $XDG_DATA_HOME/yadokari/yadokari.db     (~/.local/share)
//
// The database lives under data rather than cache because it will also hold
// the outbox of unsent reviews, which must never be discarded.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

const appName = "yadokari"

// Paths are the files yadokari uses.
type Paths struct {
	ConfigDir string
	Config    string // config.toml
	TokenFile string // fallback when the keyring is unavailable
	DataDir   string
	Database  string
}

// Env abstracts the environment for tests.
type Env struct {
	Getenv  func(string) string
	HomeDir func() (string, error)
}

// OSEnv is the real environment.
var OSEnv = Env{Getenv: os.Getenv, HomeDir: os.UserHomeDir}

// DefaultPaths resolves paths from XDG variables, falling back to ~/.config
// and ~/.local/share.
func DefaultPaths(env Env) (Paths, error) {
	home, err := env.HomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("config: home directory: %w", err)
	}
	xdg := func(name, fallback string) string {
		if v := env.Getenv(name); v != "" && filepath.IsAbs(v) {
			return v
		}
		return filepath.Join(home, fallback)
	}
	cfg := filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), appName)
	data := filepath.Join(xdg("XDG_DATA_HOME", filepath.Join(".local", "share")), appName)
	return Paths{
		ConfigDir: cfg,
		Config:    filepath.Join(cfg, "config.toml"),
		TokenFile: filepath.Join(cfg, "token"),
		DataDir:   data,
		Database:  filepath.Join(data, appName+".db"),
	}, nil
}

// Config is the contents of config.toml. Every field is optional, and the
// zero Config behaves as if there were no file: settings that mirror a
// WaniKani preference follow it unless set. docs/config.example.toml lists
// every key.
type Config struct {
	// APIBaseURL overrides the API endpoint, e.g. the local mock
	// "http://127.0.0.1:8642/v2/". Empty means the live API.
	APIBaseURL string  `toml:"api_base_url"`
	Reviews    Reviews `toml:"reviews"`
	Lessons    Lessons `toml:"lessons"`
	Audio      Audio   `toml:"audio"`
	Display    Display `toml:"display"`
	Theme      Colours `toml:"theme"`
}

// Colours override single colours of the chosen theme: "#rrggbb", or a
// terminal palette number "0"–"255" (the terminal's own shade). Ink is the
// glyph itself, drawn into an image, so it takes "#rrggbb" only.
type Colours struct {
	Radical    string `toml:"radical"`
	Kanji      string `toml:"kanji"`
	Vocabulary string `toml:"vocabulary"`
	Ink        string `toml:"ink"`
	Lessons    string `toml:"lessons"`
	Reviews    string `toml:"reviews"`
	Bar        string `toml:"bar"`
	Correct    string `toml:"correct"`
	Wrong      string `toml:"wrong"`
	Warn       string `toml:"warn"`
	Error      string `toml:"error"`
	Line       string `toml:"line"`
}

var hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validColour(s string, paletteOK bool) bool {
	if hexColour.MatchString(s) {
		return true
	}
	n, err := strconv.Atoi(s)
	return paletteOK && err == nil && n >= 0 && n <= 255
}

// Reviews are the review session settings.
type Reviews struct {
	Order         string `toml:"order"`       // see ReviewOrders; "" = "wanikani"
	ActiveSize    int    `toml:"active_size"` // 0 = the default (10)
	Limit         int    `toml:"limit"`       // 0 = all
	BackToBack    bool   `toml:"back_to_back"`
	ReadingFirst  bool   `toml:"reading_first"`
	Strictness    string `toml:"strictness"` // see Strictnesses; "" = "wanikani"
	AllowUndo     bool   `toml:"allow_undo"`
	AutoplayAudio *bool  `toml:"autoplay_audio"` // nil = WaniKani's preference
}

// Lessons are the lesson settings.
type Lessons struct {
	BatchSize     int    `toml:"batch_size"` // 0 = WaniKani's preference
	Order         string `toml:"order"`      // see LessonOrders; "" = "wanikani"
	AutoplayAudio *bool  `toml:"autoplay_audio"`
}

// Audio picks the player.
type Audio struct {
	Player string `toml:"player"` // a command line; "" = the first one found
}

// Display is about drawing.
type Display struct {
	Gfx   string `toml:"gfx"`   // see GfxTiers; "" = "auto"
	Theme string `toml:"theme"` // see ThemeNames; "" = "wanikani"
	Font  string `toml:"font"`  // a font file; "" = discovered
	// Roulette draws each review question in a random Japanese font.
	Roulette      bool     `toml:"roulette"`
	RouletteFonts []string `toml:"roulette_fonts"` // files or family names; empty = all installed
}

// The values the enum settings accept. "wanikani" follows the WaniKani
// preference of the same name.
var (
	ReviewOrders = []string{"wanikani", "shuffled", "lower_levels_first", "lower_stages_first", "by_type"}
	LessonOrders = []string{"wanikani", "ascending_level_then_subject", "ascending_level_then_shuffled", "shuffled"}
	Strictnesses = []string{"wanikani", "strict", "lenient"}
	GfxTiers     = []string{"auto", "kitty", "plain"}
	ThemeNames   = []string{"wanikani", "yadokari", "terminal"} // ui.Themes; a test keeps them in step
)

// Load reads config.toml. A missing file gives the zero Config.
func Load(path string) (Config, error) {
	var c Config
	md, err := toml.DecodeFile(path, &c)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("config: %s: unknown setting %q", path, undecoded[0].String())
	}
	if err := c.validate(); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return c, nil
}

func (c Config) validate() error {
	for _, e := range []struct {
		key, val string
		allowed  []string
	}{
		{"reviews.order", c.Reviews.Order, ReviewOrders},
		{"reviews.strictness", c.Reviews.Strictness, Strictnesses},
		{"lessons.order", c.Lessons.Order, LessonOrders},
		{"display.gfx", c.Display.Gfx, GfxTiers},
		{"display.theme", c.Display.Theme, ThemeNames},
	} {
		if e.val != "" && !slices.Contains(e.allowed, e.val) {
			return fmt.Errorf("%s = %q: use one of %s", e.key, e.val, strings.Join(e.allowed, ", "))
		}
	}
	for _, e := range []struct {
		key string
		val int
	}{
		{"reviews.active_size", c.Reviews.ActiveSize},
		{"reviews.limit", c.Reviews.Limit},
		{"lessons.batch_size", c.Lessons.BatchSize},
	} {
		if e.val < 0 {
			return fmt.Errorf("%s = %d: must be 0 or more", e.key, e.val)
		}
	}
	t := c.Theme
	for _, e := range []struct {
		key, val  string
		paletteOK bool
	}{
		{"radical", t.Radical, true}, {"kanji", t.Kanji, true}, {"vocabulary", t.Vocabulary, true},
		{"ink", t.Ink, false}, {"lessons", t.Lessons, true}, {"reviews", t.Reviews, true},
		{"bar", t.Bar, true}, {"correct", t.Correct, true}, {"wrong", t.Wrong, true},
		{"warn", t.Warn, true}, {"error", t.Error, true}, {"line", t.Line, true},
	} {
		if e.val == "" || validColour(e.val, e.paletteOK) {
			continue
		}
		if e.paletteOK {
			return fmt.Errorf("theme.%s = %q: use \"#rrggbb\" or a palette number \"0\"–\"255\"", e.key, e.val)
		}
		return fmt.Errorf("theme.%s = %q: use \"#rrggbb\"", e.key, e.val)
	}
	if c.Audio.Player != "" && len(strings.Fields(c.Audio.Player)) == 0 {
		return fmt.Errorf("audio.player is blank")
	}
	return nil
}
