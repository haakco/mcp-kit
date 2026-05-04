# mcp-kit development commands
# Run `just` to see available recipes.

set dotenv-load := true

default:
    @just --list

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

# Run the full local quality gate.
quality:
    just build
    just vet
    just lint-go-deep
    just lint-go-structural
    just test-race

# Format Go code.
format:
    gofmt -s -w .

# Ensure go.mod/go.sum are tidy.
tidy:
    go mod tidy
