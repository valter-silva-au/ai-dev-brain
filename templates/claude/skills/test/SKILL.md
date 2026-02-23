---
name: test
description: Run tests with options for coverage, race detection, specific packages, or property tests only
argument-hint: "[package|coverage|property|race]"
allowed-tools: Bash, Read, Grep
---

# Test Skill

Run the project test suite with various options.

## Steps

Based on the argument provided:

### No arguments
Run the full test suite:
```
go test ./... -count=1
```

### "coverage"
Run tests with coverage analysis:
```
go test ./... -coverprofile=coverage.out -covermode=atomic
go tool cover -func=coverage.out
```
Report the total coverage percentage and per-package breakdown.

### "property"
Run only property-based tests (tests with names starting with TestProperty):
```
go test ./... -run TestProperty -v -count=1
```

### "race"
Run tests with the race detector enabled:
```
go test ./... -race -count=1
```
**Important:** On Windows, the race detector requires CGO_ENABLED=1 and a working C compiler (gcc). Warn the user about this requirement if running on Windows.

### Package name (e.g., "core", "storage", "cli", "integration")
Run tests for a specific package:
```
go test ./internal/<package>/ -v -count=1
```
If the argument matches a known package directory under `internal/` or `pkg/`, run tests for that package. Use Grep to find the matching package directory if needed.

## Output

- Report the pass/fail summary.
- For any failures, show the full failure details including test name, file, line number, and assertion message.
- Report total test count and elapsed time.
