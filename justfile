# yadokari task runner. Run `just` to list recipes.

set dotenv-load := true

bin := "bin/yadokari"
version := `git describe --always --dirty --tags`

default:
    @just --list

# Build a development binary into bin/ (never writes to the live API)
build:
    go build -o {{bin}} ./cmd/yadokari

# Install a static binary to ~/.local/bin (shared by the host and the toolbox).
# Stamped with the version, it is a build for use: it writes to the live API.
install:
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version={{version}}" -o ~/.local/bin/yadokari ./cmd/yadokari
    @echo "installed ~/.local/bin/yadokari $(~/.local/bin/yadokari --version)"

# Run a development build (never writes to the live API); extra flags after the recipe name
run *args:
    go run ./cmd/yadokari {{args}}

# Run all tests (spikes excluded)
test *args:
    go test {{args}} $(go list ./... | grep -v /spikes/)

# Format and vet
check:
    test -z "$(gofmt -l cmd internal spikes 2>/dev/null)" || (gofmt -l cmd internal spikes; exit 1)
    go vet ./...

fmt:
    gofmt -w cmd internal spikes

# Serve the fake API on 127.0.0.1:8642 (sample data, or --fixtures=DIR)
mock *args:
    go run ./cmd/wkmock {{args}}

# Read-only full sync against the mock (needs `just mock` running)
probe-mock *args:
    WANIKANI_API_TOKEN=${WANIKANI_API_TOKEN:-mock} go run ./cmd/wkprobe --base=http://127.0.0.1:8642/v2/ {{args}}

# Read-only full sync against the LIVE API; records fixtures (needs a read-only WANIKANI_API_TOKEN in .env)
probe-live:
    go run ./cmd/wkprobe --record=testdata/recordings/$(date +%Y%m%d-%H%M%S)

# Serve the newest local recording through the mock (for `just dev`)
mock-recorded:
    go run ./cmd/wkmock --fixtures="$(ls -d testdata/recordings/*/ | tail -1)"

# Run the app against the local mock with a scratch database in .dev/
dev *args:
    mkdir -p .dev && WANIKANI_API_TOKEN=mock go run ./cmd/yadokari --base=http://127.0.0.1:8642/v2/ --db=.dev/yadokari.db {{args}}

# Run a throwaway spike, e.g. `just spike placeholders`
spike name *args:
    go run ./spikes/{{name}} {{args}}

clean:
    rm -rf bin dist

# Render screens once from demo data, e.g. `just preview dashboard --size=120x36`
# (flags after screen names are fine); --html=FILE writes a gallery page
preview *args:
    go run ./cmd/preview {{args}}

# Build for every release target (no output), as CI does
cross:
    for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do GOOS=${t%/*} GOARCH=${t#*/} CGO_ENABLED=0 go build -o /dev/null ./cmd/yadokari && echo "$t ok"; done

# Check .goreleaser.yaml and build a snapshot release into dist/ (downloads GoReleaser, and a newer Go if it needs one)
release-check:
    GOTOOLCHAIN=auto go run github.com/goreleaser/goreleaser/v2@v2.20.2 check
    GOTOOLCHAIN=auto go run github.com/goreleaser/goreleaser/v2@v2.20.2 release --snapshot --clean
