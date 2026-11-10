# yadokari task runner. Run `just` to list recipes.

set dotenv-load := true

bin := "bin/yadokari"
version := `git describe --always --dirty --tags`

default:
    @just --list

# Build the binary into bin/
build:
    go build -ldflags "-X main.version={{version}}" -o {{bin}} ./cmd/yadokari

# Install a static binary to ~/.local/bin (shared by the host and the toolbox)
install:
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version={{version}}" -o ~/.local/bin/yadokari ./cmd/yadokari
    @echo "installed ~/.local/bin/yadokari $(~/.local/bin/yadokari --version)"

# Run the app (dry-run by default); pass extra flags after the recipe name
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
    rm -rf bin

# Render screens once from demo data, e.g. `just preview dashboard --size=120x36`
# (flags after screen names are fine); --html=FILE writes a gallery page
preview *args:
    go run ./cmd/preview {{args}}
