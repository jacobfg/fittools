# List available recipes.
default:
    @just --list

# Install frontend and validation dependencies and build static assets.
ui:
    cd web && npm ci && npm run build

# Build the native CLI and server with embedded UI assets and example presets.
build: ui
    go build -trimpath -ldflags='-s -w' -o bin/fittools ./cmd/fittools

# Run native regression tests with the race detector.
test:
    go test -race ./internal/...

# Build and run browser tests.
test-browser: build
    cd web && npx playwright test

# Check the built CLI against frozen references and Garmin's independent decoder.
verify-cli:
    node web/scripts/verify-cli.mjs bin/fittools

# Verify a running server or container.
verify-http url="http://127.0.0.1:8080":
    node web/scripts/verify-http.mjs {{quote(url)}}

# Reproducible Go throughput and allocation measurements.
benchmark:
    go test ./internal/fit -run '^$' -bench BenchmarkTransform -benchmem -count 5

# Build and run all local checks (browser tests include live HTTP verification).
check: build test verify-cli
    go vet ./...
    cd web && npx playwright test
