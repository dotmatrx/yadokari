// Command yadokari is a terminal client for WaniKani reviews and lessons.
package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	// Writes to the real WaniKani API are irreversible. Nothing may submit
	// reviews or start assignments unless this flag is set.
	live := flag.Bool("live", false, "allow writes to the live WaniKani API (default: dry-run)")
	flag.Parse()

	if *showVersion {
		fmt.Println("yadokari", version)
		return
	}

	mode := "dry-run"
	if *live {
		mode = "live"
	}
	fmt.Fprintf(os.Stderr, "yadokari %s (%s): nothing here yet\n", version, mode)
}
