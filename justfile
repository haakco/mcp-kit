# mcp-kit development commands
# Run `just` to see available recipes.

set dotenv-load := true

default:
    @just --list

# Version of govulncheck used locally and in CI. Pinned so the gate is
# reproducible; `@latest` would make the same commit pass or fail on different days.
govulncheck_version := "v1.8.0"

# Build every package.
build:
    go build ./...

# Run all tests.
test:
    go test ./...

# Run all tests with the race detector.
test-race:
    go test ./... -race -count=1

# Run Go vet.
vet:
    go vet ./...

# Run the default lint suite.
lint-go:
    go tool golangci-lint run --allow-serial-runners --fast-only --disable=dupl --disable=gocognit --disable=gocyclo --disable=funlen --disable=nestif --timeout=10m ./...

# Run the deep lint suite, including slower analyzers.
lint-go-deep:
    go tool golangci-lint run --allow-serial-runners --timeout=10m ./...

# Run only structural checks: duplication, complexity, function length, nesting.
lint-go-structural:
    go tool golangci-lint run --allow-serial-runners --timeout=10m --enable-only=dupl,gocognit,gocyclo,funlen,nestif ./...

# Alias for structural lint.
check-structural:
    just lint-go-structural

# Run duplication checks only.
check-dup:
    go tool golangci-lint run --allow-serial-runners --timeout=10m --enable-only=dupl ./...

# Run complexity checks only.
check-complexity:
    go tool golangci-lint run --allow-serial-runners --timeout=10m --enable-only=gocognit,gocyclo,funlen,nestif ./...

# Run the vulnerability check. Identical command in CI.
vulncheck:
    go run golang.org/x/vuln/cmd/govulncheck@{{govulncheck_version}} ./...

# Run the full local quality gate.
quality:
    just build
    just vet
    just lint-go-deep
    just lint-go-structural
    just test-race

# Run the official MCP 2026-07-28 conformance suite.
# Point CONFORMANCE_URL at a consumer server to check its tools/resources/prompts.
conformance:
    ./scripts/conformance/run.sh

# Run the conformance suite against a specific server URL.
conformance-against url:
    ./scripts/conformance/run.sh --url {{url}}

# Format Go code.
format:
    gofmt -s -w .

# Ensure go.mod/go.sum are tidy.
tidy:
    go mod tidy
