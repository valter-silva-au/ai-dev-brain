---
name: build
description: Build the adb binary for the current platform or cross-compile for all targets
argument-hint: "[platform]"
allowed-tools: Bash, Read
---

# Build Skill

Build the `adb` binary from `./cmd/adb/`.

## Steps

1. Run `go vet ./...` before building. If vet fails, report the errors and stop.
2. Based on the argument provided:

### No arguments
Build for the current platform:
```
go build -o adb ./cmd/adb/
```
On Windows, use `adb.exe` as the output name instead.

### Specific platform (e.g., "linux/amd64")
Parse the argument as GOOS/GOARCH and cross-compile:
```
CGO_ENABLED=0 GOOS=<os> GOARCH=<arch> go build -o adb-<os>-<arch> ./cmd/adb/
```
On Windows targets, append `.exe` to the binary name.

### "all"
Build for all 6 supported targets from .goreleaser.yml:
- linux/amd64
- linux/arm64
- darwin/amd64
- darwin/arm64
- windows/amd64
- windows/arm64

Use `CGO_ENABLED=0` for all cross-compiled builds. Name each binary `adb-<os>-<arch>` (with `.exe` for Windows targets).

3. After each successful build, report the binary file size.
4. Summarize all built binaries at the end.
