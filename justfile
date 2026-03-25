# vars — development workflow

# Default recipe: show help
help:
    @just --list --unsorted

# Check and install dev toolchain dependencies
[group('dev')]
setup:
    #!/usr/bin/env bash
    set -euo pipefail
    ok=1
    if ! command -v protoc &>/dev/null; then
        echo "ERROR: protoc not found. Install with: apt install protobuf-compiler  OR  brew install protobuf"
        ok=0
    fi
    if ! command -v protoc-gen-go &>/dev/null; then
        echo "Installing protoc-gen-go..."
        go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
    fi
    if ! command -v protoc-gen-go-grpc &>/dev/null; then
        echo "Installing protoc-gen-go-grpc..."
        go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
    fi
    if [ $ok -eq 0 ]; then exit 1; fi
    echo "Toolchain ready."

# Regenerate protobuf Go code from agent.proto (commit the result)
[group('dev')]
proto:
    #!/usr/bin/env bash
    set -euo pipefail
    command -v protoc-gen-go &>/dev/null || export PATH="$PATH:$HOME/go/bin"
    protoc --go_out=. --go_opt=paths=source_relative \
           --go-grpc_out=. --go-grpc_opt=paths=source_relative \
           internal/agent/agent.proto
    echo "Generated agent.pb.go and agent_grpc.pb.go — review and commit."

# Format Go source code
[group('dev')]
fmt:
    go fmt ./...

# Run go vet
[group('dev')]
vet:
    go vet ./...

# Run staticcheck linter
[group('dev')]
lint:
    staticcheck ./...

# Pre-commit quality gate: vet + lint + test
[group('dev')]
check: vet lint test

# Run unit tests
[group('test')]
test:
    go test -timeout 300s ./...

# Run unit tests with verbose output
[group('test')]
test-v:
    go test -v -timeout 300s ./...

# Run integration tests (requires built binary)
[group('test')]
test-integration: build
    go test -tags integration -v ./...

# Run unit tests with race detector
[group('test')]
test-race:
    go test -race -timeout 300s ./...

# Run all tests (unit + integration)
[group('test')]
test-all: test test-integration

# Generate test coverage report
[group('test')]
coverage:
    go test -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out
    @echo "HTML report: go tool cover -html=coverage.out"

# Version from git tag, or "dev"
version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-s -w -X github.com/vars-cli/vars/cmd.Version=" + version

# Build the binary
[group('build')]
build:
    go build -ldflags '{{ldflags}}' -o vars .

# Install to GOPATH/bin
[group('build')]
install:
    go install -ldflags '{{ldflags}}' .

# Cross-compile for all supported platforms
[group('build')]
cross-compile:
    GOOS=darwin GOARCH=arm64 go build -ldflags '{{ldflags}}' -o dist/vars-darwin-arm64 .
    GOOS=darwin GOARCH=amd64 go build -ldflags '{{ldflags}}' -o dist/vars-darwin-amd64 .
    GOOS=linux  GOARCH=amd64 go build -ldflags '{{ldflags}}' -o dist/vars-linux-amd64  .
    GOOS=linux  GOARCH=arm64 go build -ldflags '{{ldflags}}' -o dist/vars-linux-arm64  .

# Quick end-to-end smoke test against a temp store
[group('test')]
smoke: build
    bash scripts/smoke.sh ./vars

# Dry-run goreleaser
[group('release')]
release-dry:
    goreleaser release --snapshot --clean
