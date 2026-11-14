package main

import "testing"

// Only builds made to be used may write to the live API.
func TestBuildKind(t *testing.T) {
	for _, tt := range []struct {
		stamped, module string
		want            string
		live            bool
	}{
		{"v0.5.0", "", "v0.5.0", true},                                                                 // just install, release builds
		{"v0.5.0-3-gabc1234-dirty", "", "v0.5.0-3-gabc1234-dirty", true},                               // just install between tags
		{"dev", "v0.6.0", "v0.6.0", true},                                                              // go install …@v0.6.0
		{"dev", "v1.0.0-rc.1", "v1.0.0-rc.1", true},                                                    // a pre-release tag
		{"dev", "v0.5.0+dirty", "v0.5.0+dirty", false},                                                 // go build in a changed work tree
		{"dev", "v0.5.1-0.20261114120000-abcdef123456", "v0.5.1-0.20261114120000-abcdef123456", false}, // go build between tags
		{"dev", "(devel)", "dev", false},                                                               // go run, tests
		{"dev", "", "dev", false},
	} {
		got, live := buildKind(tt.stamped, tt.module)
		if got != tt.want || live != tt.live {
			t.Errorf("buildKind(%q, %q) = %q, %v; want %q, %v", tt.stamped, tt.module, got, live, tt.want, tt.live)
		}
	}
	if liveWrites {
		t.Error("a test binary is a development build")
	}
}
