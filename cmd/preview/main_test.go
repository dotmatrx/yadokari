package main

import (
	"flag"
	"slices"
	"testing"
)

func TestParseArgsAnyOrder(t *testing.T) {
	for _, args := range [][]string{
		{"dashboard", "--layout=panels,rules", "review"},
		{"--layout", "panels,rules", "dashboard", "review"},
		{"dashboard", "review", "-layout=panels,rules"},
	} {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		layout := fs.String("layout", "", "")
		screens, err := parseArgs(fs, args)
		if err != nil || *layout != "panels,rules" || !slices.Equal(screens, []string{"dashboard", "review"}) {
			t.Errorf("%q: screens %q layout %q err %v", args, screens, *layout, err)
		}
	}
}
