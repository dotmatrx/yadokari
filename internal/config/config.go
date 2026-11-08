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

// Config is the contents of config.toml. Every field is optional.
type Config struct {
	// APIBaseURL overrides the API endpoint, e.g. the local mock
	// "http://127.0.0.1:8642/v2/". Empty means the live API.
	APIBaseURL string `toml:"api_base_url"`
}

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
	return c, nil
}
