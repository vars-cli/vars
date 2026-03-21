# secrets — development workflow

# Default recipe: show help
help:
    @just --list --unsorted

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
    go test ./...

# Run unit tests with verbose output
[group('test')]
test-v:
    go test -v ./...

# Run integration tests (requires built binary)
[group('test')]
test-integration: build
    go test -tags integration -v ./...

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
ldflags := "-s -w -X github.com/brickpop/secrets/cmd.Version=" + version

# Build the binary
[group('build')]
build:
    go build -ldflags '{{ldflags}}' -o secrets .

# Install to GOPATH/bin
[group('build')]
install:
    go install -ldflags '{{ldflags}}' .

# Cross-compile for all supported platforms
[group('build')]
cross-compile:
    GOOS=darwin  GOARCH=arm64 go build -ldflags '{{ldflags}}' -o dist/secrets-darwin-arm64  .
    GOOS=darwin  GOARCH=amd64 go build -ldflags '{{ldflags}}' -o dist/secrets-darwin-amd64  .
    GOOS=linux   GOARCH=amd64 go build -ldflags '{{ldflags}}' -o dist/secrets-linux-amd64   .
    GOOS=linux   GOARCH=arm64 go build -ldflags '{{ldflags}}' -o dist/secrets-linux-arm64   .
    GOOS=windows GOARCH=amd64 go build -ldflags '{{ldflags}}' -o dist/secrets-windows-amd64.exe .

# Quick end-to-end smoke test against a temp store
[group('test')]
smoke: build
    #!/usr/bin/env bash
    set -euo pipefail
    export SECRETS_STORE_DIR=$(mktemp -d)
    trap "rm -rf $SECRETS_STORE_DIR" EXIT
    BIN="./secrets"

    # Cleanup: stop agent on exit
    trap "$BIN agent stop 2>/dev/null; rm -rf $SECRETS_STORE_DIR" EXIT

    echo "--- init (no passphrase) ---"
    echo -e "\n\n" | $BIN init

    echo "--- set keys (agent auto-starts) ---"
    $BIN set RPC_URL https://rpc.example.com
    $BIN set PRIVATE_KEY 0xTESTKEY
    $BIN set ETHERSCAN_API abc123

    echo "--- get ---"
    test "$($BIN get RPC_URL)" = "https://rpc.example.com"

    echo "--- ls ---"
    $BIN ls
    test "$($BIN ls | wc -l)" -eq 3

    echo "--- export (posix) ---"
    WORKDIR=$(mktemp -d)
    trap "$BIN agent stop 2>/dev/null; rm -rf $SECRETS_STORE_DIR $WORKDIR" EXIT
    cat > "$WORKDIR/.secrets.yaml" <<'YAML'
    project: smoke-test
    keys:
      - RPC_URL
      - PRIVATE_KEY
    YAML
    eval "$($BIN export -f "$WORKDIR/.secrets.yaml")"
    test "$RPC_URL" = "https://rpc.example.com"
    test "$PRIVATE_KEY" = "0xTESTKEY"

    echo "--- export (fish) ---"
    $BIN export -f "$WORKDIR/.secrets.yaml" --format fish | grep -q "set -x"

    echo "--- export (dotenv) ---"
    $BIN export -f "$WORKDIR/.secrets.yaml" --format dotenv | grep -q 'RPC_URL='

    echo "--- export --partial ---"
    cat > "$WORKDIR/.secrets.yaml" <<'YAML'
    project: smoke-test
    keys:
      - RPC_URL
      - MISSING_KEY
    YAML
    $BIN export -f "$WORKDIR/.secrets.yaml" --partial 2>/dev/null | grep -q "MISSING_KEY"

    echo "--- dump ---"
    DUMP_OUT=$($BIN dump --format dotenv 2>/dev/null)
    echo "$DUMP_OUT" | grep -q "ETHERSCAN_API"

    echo "--- rm ---"
    $BIN rm ETHERSCAN_API --force
    test "$($BIN ls | wc -l)" -eq 2

    echo "--- agent stop + auto-restart ---"
    $BIN agent stop
    sleep 0.2
    # Next command should auto-start agent again
    test "$($BIN get RPC_URL)" = "https://rpc.example.com"

    echo "--- version ---"
    $BIN --version | grep -q "secrets"

    echo ""
    echo "All smoke tests passed!"

# Dry-run goreleaser
[group('release')]
release-dry:
    goreleaser release --snapshot --clean
