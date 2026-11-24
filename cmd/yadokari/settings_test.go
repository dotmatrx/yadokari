package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dotmatrx/yadokari/docs"
	"github.com/dotmatrx/yadokari/internal/config"
	"github.com/dotmatrx/yadokari/internal/wk"
)

func TestSettingsRows(t *testing.T) {
	load := func(text string) config.Config {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		c, err := config.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	prefs := &wk.Preferences{ReviewsPresentationOrder: "shuffled", LessonsBatchSize: 7, DefaultVoiceActorID: 2}
	for _, tt := range []struct {
		name   string
		config string
		flags  settingFlags
		prefs  *wk.Preferences
		player string
		want   []settingRow // rows that must appear, among the rest
	}{
		{"no file, not synced", "", settingFlags{}, nil, "", []settingRow{
			{"reviews.order", "?", fromNoSync},
			{"reviews.active_size", "10", fromDefault},
			{"audio.player", "auto: none found", fromDefault},
			{"theme.*", "the theme's own colours", fromDefault},
		}},
		{"no file, synced", "", settingFlags{}, prefs, "pw-play", []settingRow{
			{"reviews.order", "shuffled", fromWK},
			{"lessons.batch_size", "7", fromWK},
			{"audio.voice", "default voice (voice actor 2)", fromWK},
			{"audio.player", "auto: pw-play", fromDefault},
		}},
		{"values that defer", "[reviews]\norder = \"wanikani\"\nactive_size = 0\n[lessons]\nbatch_size = 0\n", settingFlags{}, prefs, "", []settingRow{
			{"reviews.order", "shuffled", fromWK},
			{"reviews.active_size", "10", fromDefault},
			{"lessons.batch_size", "7", fromWK},
		}},
		{"set in the file", "[reviews]\norder = \"by_type\"\nallow_undo = false\nautoplay_audio = true\n[audio]\nvoice = \"Kyoko\"\n[theme]\nkanji = \"#112233\"\n", settingFlags{}, prefs, "", []settingRow{
			{"reviews.order", "by_type", fromFile},
			{"reviews.allow_undo", "false", fromFile}, // set, even to the default
			{"reviews.autoplay_audio", "true", fromFile},
			{"audio.voice", "Kyoko", fromFile},
			{"theme.kanji", "#112233", fromFile},
		}},
		{"flags win", "api_base_url = \"http://x/v2/\"\n[display]\ngfx = \"kitty\"\n", settingFlags{base: "http://127.0.0.1:8642/v2/", gfx: "plain"}, nil, "", []settingRow{
			{"api_base_url", "http://127.0.0.1:8642/v2/", "--base"},
			{"display.gfx", "plain", "--gfx"},
		}},
	} {
		rows := settingsRows(load(tt.config), tt.flags, tt.prefs, tt.player)
		got := map[string]settingRow{}
		for _, r := range rows {
			got[r.key] = r
		}
		for _, w := range tt.want {
			if got[w.key] != w {
				t.Errorf("%s: %s = %+v, want %+v", tt.name, w.key, got[w.key], w)
			}
		}
		if _, ok := got["theme.*"]; ok == strings.Contains(tt.config, "[theme]") {
			t.Errorf("%s: theme.* row shown %v", tt.name, ok)
		}
	}
}

// Every setting in the example config has a row (theme colours as theme.*
// when none is set), so a new setting can't be left out.
func TestSettingsRowsCoverEveryKey(t *testing.T) {
	rows := map[string]bool{}
	for _, r := range settingsRows(config.Config{}, settingFlags{}, nil, "") {
		rows[r.key] = true
	}
	section, n := "", 0
	for _, line := range strings.Split(docs.DefaultConfig, "\n") {
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[]") + "."
			continue
		}
		m := exampleKey.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := section + m[1]
		if section == "theme." {
			key = "theme.*"
		}
		if n++; !rows[key] {
			t.Errorf("no row for %s", key)
		}
	}
	if n < 30 {
		t.Errorf("found %d keys in the example config; the pattern is out of date", n)
	}
}

var exampleKey = regexp.MustCompile(`^# ([a-z_]+) = `)

// Piped (here, a buffer), the report has its rows and no colour codes.
func TestSettingsReportPiped(t *testing.T) {
	var b strings.Builder
	dir := t.TempDir()
	err := settingsReport(&b, config.Config{}, filepath.Join(dir, "config.toml"), filepath.Join(dir, "none.db"), settingFlags{gfx: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"reviews.order", "not synced yet", "--gfx", "╭"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("piped report has escape codes:\n%q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "none.db")); err == nil {
		t.Error("the report created a database")
	}
}
