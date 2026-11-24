package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/dotmatrx/yadokari/internal/audio"
	"github.com/dotmatrx/yadokari/internal/config"
	"github.com/dotmatrx/yadokari/internal/store"
	"github.com/dotmatrx/yadokari/internal/wk"
)

// Where a setting's effective value comes from, in the order they win.
const (
	fromFile    = "config file"
	fromWK      = "WaniKani preference"
	fromNoSync  = "WaniKani preference (not synced yet)"
	fromDefault = "default"
)

type settingRow struct{ key, value, from string }

// settingFlags are the command-line flags that override settings: the
// value given, "" if not.
type settingFlags struct{ base, gfx string }

// settingsRows is every setting's effective value and where it comes
// from. prefs is nil before the first sync; player is the audio player
// found on the PATH ("" for none).
func settingsRows(cfg config.Config, flags settingFlags, prefs *wk.Preferences, player string) []settingRow {
	var rows []settingRow
	add := func(key, value, from string) { rows = append(rows, settingRow{key, value, from}) }
	// file: the file's value if it sets key (and the value isn't the one
	// that defers), else the default.
	file := func(key, value, def string, defers bool) {
		if cfg.IsSet(key) && !defers {
			add(key, value, fromFile)
		} else {
			add(key, def, fromDefault)
		}
	}
	// wanikani: the file's value unless it defers, else the preference.
	wanikani := func(key, value string, defers bool, pref func(wk.Preferences) string) {
		switch {
		case cfg.IsSet(key) && !defers:
			add(key, value, fromFile)
		case prefs == nil:
			add(key, "?", fromNoSync)
		default:
			add(key, pref(*prefs), fromWK)
		}
	}
	itoa, btoa := strconv.Itoa, strconv.FormatBool
	ptr := func(b *bool) string {
		if b == nil {
			return ""
		}
		return btoa(*b)
	}

	switch {
	case flags.base != "":
		add("api_base_url", flags.base, "--base")
	default:
		file("api_base_url", cfg.APIBaseURL, wk.DefaultBaseURL+" (live)", cfg.APIBaseURL == "")
	}

	r := cfg.Reviews
	wanikani("reviews.order", r.Order, r.Order == "" || r.Order == "wanikani",
		func(p wk.Preferences) string { return p.ReviewsPresentationOrder })
	file("reviews.active_size", itoa(r.ActiveSize), "10", r.ActiveSize == 0)
	limit := itoa(r.Limit)
	if r.Limit == 0 {
		limit = "0 (all due)"
	}
	file("reviews.limit", limit, "0 (all due)", false)
	file("reviews.back_to_back", btoa(r.BackToBack), "false", false)
	file("reviews.reading_first", btoa(r.ReadingFirst), "false", false)
	file("reviews.strictness", r.Strictness, "wanikani", r.Strictness == "")
	file("reviews.allow_undo", btoa(r.AllowUndo), "false", false)
	file("reviews.relaxed_n", btoa(r.RelaxedN), "false", false)
	file("reviews.lightning", btoa(r.Lightning), "false", false)
	wanikani("reviews.autoplay_audio", ptr(r.AutoplayAudio), r.AutoplayAudio == nil,
		func(p wk.Preferences) string { return btoa(p.ReviewsAutoplayAudio) })

	l := cfg.Lessons
	wanikani("lessons.batch_size", itoa(l.BatchSize), l.BatchSize == 0,
		func(p wk.Preferences) string { return itoa(p.LessonsBatchSize) })
	wanikani("lessons.order", l.Order, l.Order == "" || l.Order == "wanikani",
		func(p wk.Preferences) string { return p.LessonsPresentationOrder })
	wanikani("lessons.autoplay_audio", ptr(l.AutoplayAudio), l.AutoplayAudio == nil,
		func(p wk.Preferences) string { return btoa(p.LessonsAutoplayAudio) })

	found := "none found"
	if player != "" {
		found = player
	}
	file("audio.player", cfg.Audio.Player, "auto: "+found, cfg.Audio.Player == "")
	wanikani("audio.voice", cfg.Audio.Voice, cfg.Audio.Voice == "",
		func(p wk.Preferences) string {
			return "default voice (voice actor " + itoa(p.DefaultVoiceActorID) + ")"
		})

	d := cfg.Display
	file("display.theme", d.Theme, "yadokari", d.Theme == "")
	file("display.field", d.Field, "block", d.Field == "")
	file("display.background", d.Background, "auto", d.Background == "")
	switch {
	case flags.gfx != "":
		add("display.gfx", flags.gfx, "--gfx")
	default:
		file("display.gfx", d.Gfx, "auto", d.Gfx == "")
	}
	file("display.font", d.Font, "auto", d.Font == "")
	file("display.roulette", btoa(d.Roulette), "false", false)
	file("display.roulette_fonts", strings.Join(d.RouletteFonts, ", "), "all installed", len(d.RouletteFonts) == 0)

	t := cfg.Theme
	colours := false
	for _, c := range []struct{ key, value string }{
		{"radical", t.Radical}, {"kanji", t.Kanji}, {"vocabulary", t.Vocabulary}, {"ink", t.Ink},
		{"lessons", t.Lessons}, {"reviews", t.Reviews}, {"bar", t.Bar}, {"correct", t.Correct},
		{"wrong", t.Wrong}, {"warn", t.Warn}, {"error", t.Error}, {"line", t.Line},
	} {
		if c.value != "" {
			add("theme."+c.key, c.value, fromFile)
			colours = true
		}
	}
	if !colours {
		add("theme.*", "the theme's own colours", fromDefault)
	}
	return rows
}

// settingsReport prints settingsRows, for yadokari --settings. The
// WaniKani preferences come from the database, read only if it exists
// (it's never created here).
func settingsReport(w io.Writer, cfg config.Config, configPath, dbPath string, flags settingFlags) error {
	var prefs *wk.Preferences
	if _, err := os.Stat(dbPath); err == nil {
		st, err := store.Open(context.Background(), dbPath)
		if err != nil {
			return err
		}
		u, err := st.User(context.Background())
		st.Close()
		if err != nil {
			return err
		}
		if u != nil {
			prefs = &u.Data.Preferences
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	player := ""
	if p, err := audio.Find(exec.LookPath); err == nil {
		player = p.Name
	}

	if _, err := lipgloss.Fprintln(w, settingsTable(settingsRows(cfg, flags, prefs, player))); err != nil {
		return err
	}
	fmt.Fprintf(w, "\nConfig file: %s\n", configPath)
	if prefs == nil {
		fmt.Fprintln(w, "WaniKani preferences are read from the last sync; there hasn't been one yet.")
	}
	fmt.Fprintln(w, "`yadokari --terminal` shows what auto settings found; --default-config documents every key.")
	return nil
}

// settingsTable lays the rows out as a table: the section prefix dimmed,
// the source coloured by kind, in the terminal's own palette so it suits
// light and dark schemes. lipgloss.Fprintln drops the colours for a pipe
// or NO_COLOR.
func settingsTable(rows []settingRow) *table.Table {
	dim := lipgloss.NewStyle().Faint(true)
	source := func(from string) lipgloss.Style {
		switch {
		case strings.HasPrefix(from, "--"):
			return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
		case from == fromFile:
			return lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
		case from == fromWK, from == fromNoSync:
			return lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
		}
		return dim
	}
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("8"))).
		Headers("setting", "value", "from").
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().Padding(0, 1)
			if row == table.HeaderRow {
				return s.Bold(true)
			}
			return s
		})
	for _, r := range rows {
		key := r.key
		if i := strings.LastIndexByte(key, '.'); i >= 0 {
			key = dim.Render(key[:i+1]) + key[i+1:]
		}
		t.Row(key, r.value, source(r.from).Render(r.from))
	}
	return t
}
