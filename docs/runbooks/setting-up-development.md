# Runbook: Setting Up a Development Environment

## Prerequisites

- Go 1.24 or later installed
- Git installed and configured
- Access to the repository

## Steps

### 1. Clone the repository

```bash
git clone https://github.com/valter-silva-au/valter-silva.git
cd valter-silva
```

### 2. Install Go dependencies

```bash
go mod download
```

### 3. Install development tools

```bash
# Linter
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Security scanner
go install golang.org/x/vuln/cmd/govulncheck@latest
```

### 4. Build the binary

```bash
go build -ldflags="-s -w" -o adb ./cmd/adb/
```

### 5. Run the test suite

```bash
# All tests
go test ./... -count=1

# With race detector
go test ./... -race -count=1

# Property-based tests only
go test ./... -run "TestProperty" -count=1 -v
```

### 6. Run linting

```bash
golangci-lint run
go vet ./...
gofmt -l .
```

### 7. Verify the binary works

```bash
./adb version
./adb --help
```

### 8. Create a task to start working

```bash
./adb feat my-feature-branch
```

This bootstraps a ticket directory, creates a git worktree, and launches Claude Code in the worktree.

## Verification

- [ ] `go build` succeeds without errors
- [ ] `go test ./...` passes all tests
- [ ] `golangci-lint run` reports no issues
- [ ] `./adb version` prints version information
- [ ] `./adb --help` shows available commands

## Troubleshooting

**`go mod download` fails**: Check your Go proxy settings. Try `GOPROXY=https://proxy.golang.org,direct go mod download`.

**Tests fail with permission errors**: Ensure `t.TempDir()` is used for filesystem isolation. Do not run tests that write to the repository directory.

**`golangci-lint` not found**: Ensure `$GOPATH/bin` (or `$HOME/go/bin`) is on your PATH.

**Build fails with ldflags errors**: The `-ldflags="-s -w"` flags strip debug info for smaller binaries. If you need debug symbols for development, omit the flags: `go build -o adb ./cmd/adb/`.
