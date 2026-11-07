# yadokari task runner. Run `just` to list recipes.

set dotenv-load := true

bin := "bin/yadokari"

default:
    @just --list

# Build the binary into bin/
build:
    go build -o {{bin}} ./cmd/yadokari

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

# Run a throwaway spike, e.g. `just spike placeholders`
spike name *args:
    go run ./spikes/{{name}} {{args}}

clean:
    rm -rf bin
