---
name: health-dashboard
description: Comprehensive health check with build, test, lint, coverage, security, and task metrics
allowed-tools: Bash, Read, Grep, Glob
---

# Health Dashboard Skill

Run a comprehensive health check covering build, tests, lint, coverage, security scanning, and task metrics.

## Steps

### 1. Build Check
```
go build ./cmd/adb/
```
Record: pass/fail

### 2. Vet Check
```
go vet ./...
```
Record: pass/fail, number of issues

### 3. Test Check
```
go test ./... -count=1
```
Record: pass/fail, tests passed, tests failed

### 4. Lint Check
```
golangci-lint run
```
Record: pass/fail, number of issues

### 5. Coverage Check
```
go test ./... -coverprofile=coverage.out -count=1
go tool cover -func=coverage.out
```
Record: total coverage percentage, per-package coverage

### 6. Security Scan
```
govulncheck ./...
```
Record: pass/fail, number of vulnerabilities found

### 7. Code Metrics
Count using file searches:
- Total .go files (excluding test files)
- Total _test.go files
- Total _property_test.go files
- Packages (directories containing .go files)

### 8. Task Metrics
Read `backlog.yaml` and count tasks by status and priority.

## Output

```
=== Health Dashboard ===
Generated: YYYY-MM-DD HH:MM

-- Build & Quality --
Check      | Status | Details
-----------|--------|--------
Build      | PASS   | compiled successfully
Vet        | PASS   | no issues
Tests      | PASS   | N passed, 0 failed
Lint       | PASS   | no issues
Coverage   | PASS   | XX.X% total
Security   | PASS   | no vulnerabilities

-- Coverage by Package --
Package                    | Coverage
---------------------------|--------
internal/core              | XX.X%
internal/storage           | XX.X%
internal/integration       | XX.X%

-- Code Metrics --
Metric                | Count
----------------------|------
Go source files       | N
Test files            | N
Property test files   | N
Packages              | N

-- Task Metrics --
Status       | Count
-------------|------
in_progress  | N
blocked      | N
review       | N
backlog      | N
done         | N
archived     | N

Overall: [ALL CHECKS PASSED / N ISSUES FOUND]
```

Clean up the coverage.out file after generating the report.
