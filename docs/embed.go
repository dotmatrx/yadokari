// Package docs embeds the documented example config, so the binary can
// print the defaults of its own version (yadokari --default-config).
package docs

import _ "embed"

// DefaultConfig is config.example.toml: every setting, documented, with
// its default commented out.
//
//go:embed config.example.toml
var DefaultConfig string
