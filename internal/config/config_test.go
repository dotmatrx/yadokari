package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fakeKeyring struct {
	secrets map[string]string
	broken  error // returned by every call when set
}

func (k *fakeKeyring) Get(s, u string) (string, error) {
	if k.broken != nil {
		return "", k.broken
	}
	v, ok := k.secrets[s+"/"+u]
	if !ok {
		return "", ErrKeyringNotFound
	}
	return v, nil
}

func (k *fakeKeyring) Set(s, u, p string) error {
	if k.broken != nil {
		return k.broken
	}
	k.secrets[s+"/"+u] = p
	return nil
}

func (k *fakeKeyring) Delete(s, u string) error {
	if k.broken != nil {
		return k.broken
	}
	if _, ok := k.secrets[s+"/"+u]; !ok {
		return ErrKeyringNotFound
	}
	delete(k.secrets, s+"/"+u)
	return nil
}

func newStore(t *testing.T, env map[string]string) (TokenStore, *fakeKeyring) {
	kr := &fakeKeyring{secrets: map[string]string{}}
	return TokenStore{
		Keyring: kr,
		File:    filepath.Join(t.TempDir(), "yadokari", "token"),
		Getenv:  func(k string) string { return env[k] },
	}, kr
}

func TestTokenPrecedence(t *testing.T) {
	env := map[string]string{}
	ts, kr := newStore(t, env)

	if _, _, err := ts.Load(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("empty: %v", err)
	}

	if src, err := ts.Save(" from-keyring \n"); err != nil || src != FromKeyring {
		t.Fatalf("save: %v %v", src, err)
	}
	if v, src, _ := ts.Load(); v != "from-keyring" || src != FromKeyring {
		t.Errorf("keyring: %q %v", v, src)
	}

	env[TokenEnvVar] = "from-env"
	if v, src, _ := ts.Load(); v != "from-env" || src != FromEnv {
		t.Errorf("env should win: %q %v", v, src)
	}
	delete(env, TokenEnvVar)

	if err := ts.Delete(); err != nil {
		t.Fatal(err)
	}
	if len(kr.secrets) != 0 {
		t.Error("delete left the keyring entry")
	}
	if _, _, err := ts.Load(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestBrokenKeyringFallsBackToFile(t *testing.T) {
	ts, kr := newStore(t, map[string]string{})
	kr.broken = errors.New("dbus: no secret service")

	_, _, err := ts.Load()
	if !errors.Is(err, ErrNoToken) || !strings.Contains(err.Error(), "keyring unavailable") {
		t.Fatalf("load with broken keyring: %v", err)
	}

	src, err := ts.Save("from-file")
	if err != nil || src != FromFile {
		t.Fatalf("save: %v %v", src, err)
	}
	info, err := os.Stat(ts.File)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file mode %04o, want 0600", perm)
	}
	if dir, _ := os.Stat(filepath.Dir(ts.File)); dir.Mode().Perm() != 0o700 {
		t.Errorf("config dir mode %04o, want 0700", dir.Mode().Perm())
	}
	if v, src, _ := ts.Load(); v != "from-file" || src != FromFile {
		t.Errorf("load: %q %v", v, src)
	}

	// Once the keyring works again, saving moves the token there and removes the file.
	kr.broken = nil
	if src, _ := ts.Save("moved"); src != FromKeyring {
		t.Fatalf("save to repaired keyring: %v", src)
	}
	if _, err := os.Stat(ts.File); !os.IsNotExist(err) {
		t.Error("file copy should be removed after saving to the keyring")
	}
}

func TestTokenFilePermissionsEnforced(t *testing.T) {
	ts, kr := newStore(t, map[string]string{})
	kr.broken = errors.New("unavailable")
	if _, err := ts.Save("secret"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ts.File, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := ts.Load()
	if err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("world-readable token file should be refused: %v", err)
	}
}

func TestDefaultPaths(t *testing.T) {
	home := func() (string, error) { return "/home/u", nil }
	p, err := DefaultPaths(Env{Getenv: func(string) string { return "" }, HomeDir: home})
	if err != nil {
		t.Fatal(err)
	}
	if p.Config != "/home/u/.config/yadokari/config.toml" || p.Database != "/home/u/.local/share/yadokari/yadokari.db" ||
		p.TokenFile != "/home/u/.config/yadokari/token" {
		t.Errorf("defaults: %+v", p)
	}

	xdg := map[string]string{"XDG_CONFIG_HOME": "/x/cfg", "XDG_DATA_HOME": "relative/ignored"}
	p, _ = DefaultPaths(Env{Getenv: func(k string) string { return xdg[k] }, HomeDir: home})
	if p.Config != "/x/cfg/yadokari/config.toml" || p.DataDir != "/home/u/.local/share/yadokari" {
		t.Errorf("xdg: %+v", p)
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	if c, err := Load(filepath.Join(dir, "missing.toml")); err != nil || !reflect.DeepEqual(c, Config{}) {
		t.Fatalf("missing file: %+v %v", c, err)
	}
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`api_base_url = "http://127.0.0.1:8642/v2/"`+"\n"), 0o600)
	if c, err := Load(path); err != nil || c.APIBaseURL != "http://127.0.0.1:8642/v2/" {
		t.Fatalf("load: %+v %v", c, err)
	}
	os.WriteFile(path, []byte(`api_bsae_url = "typo"`+"\n"), 0o600)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "api_bsae_url") {
		t.Fatalf("unknown keys should be reported: %v", err)
	}
}

// The example file documents every key: it must load, and every key in
// Config must appear in it (commented or not).
func TestExampleConfig(t *testing.T) {
	const path = "../../docs/config.example.toml"
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var keys func(typ reflect.Type) []string
	keys = func(typ reflect.Type) []string {
		var out []string
		for f := range typ.Fields() {
			tag := f.Tag.Get("toml")
			if f.Type.Kind() == reflect.Struct {
				out = append(out, "["+tag+"]")
				out = append(out, keys(f.Type)...)
				continue
			}
			out = append(out, tag)
		}
		return out
	}
	for _, k := range keys(reflect.TypeFor[Config]()) {
		if !strings.Contains(string(raw), k) {
			t.Errorf("%s is not in the example config", k)
		}
	}
}

func TestConfigRejectsBadValues(t *testing.T) {
	for _, c := range []struct{ toml, want string }{
		{"[reviews]\norder = \"random\"", `reviews.order = "random": use one of wanikani, shuffled`},
		{"[reviews]\nstrictness = \"loose\"", "reviews.strictness"},
		{"[lessons]\nbatch_size = -1", "lessons.batch_size = -1: must be 0 or more"},
		{"[display]\ngfx = \"sixel\"", "display.gfx"},
		{"[display]\nfield = \"neon\"", `display.field = "neon": use one of block, fill`},
		{"[audio]\nplayer = \"  \"", "audio.player is blank"},
		{"[reviews]\nordr = \"shuffled\"", `unknown setting "reviews.ordr"`},
		{"[theme]\nkanji = \"pink\"", `theme.kanji = "pink": use "#rrggbb" or a palette number`},
		{"[theme]\nline = \"256\"", "theme.line"},
		{"[theme]\nink = \"7\"", `theme.ink = "7": use "#rrggbb"`},
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte(c.toml), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: got %v, want %q", c.toml, err, c.want)
		}
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	_ = os.WriteFile(path, []byte("[reviews]\nautoplay_audio = false\nlimit = 50"), 0o600)
	c, err := Load(path)
	if err != nil || c.Reviews.AutoplayAudio == nil || *c.Reviews.AutoplayAudio || c.Reviews.Limit != 50 {
		t.Errorf("valid: %+v %v", c.Reviews, err)
	}
}
