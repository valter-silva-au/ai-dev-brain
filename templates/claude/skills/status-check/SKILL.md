---
name: status-check
description: Quick health check of the entire project - build, tests, lint, vet
allowed-tools: Bash, Read
---

# Status Check Skill

Run a comprehensive health check of the project and report a summary dashboard.

## Steps

Run each check in sequence and track results:

### 1. Build check
```
go build ./cmd/adb/
```
Record: pass/fail

### 2. Vet check
```
go vet ./...
```
Record: pass/fail, number of issues if any

### 3. Test check
```
go test ./... -count=1
```
Record: pass/fail, number of tests passed, number of tests failed

### 4. Lint check
```
golangci-lint run
```
Record: pass/fail, number of issues if any

## Output

Print a summary table:

```
Project Health Check
====================

Check    | Status | Details
---------|--------|--------
Build    | PASS   | compiled successfully
Vet      | PASS   | no issues
Tests    | PASS   | 42 passed, 0 failed
Lint     | PASS   | no issues

Overall: ALL CHECKS PASSED
```

If any check fails, show the details of the failure below the table.
Use PASS/FAIL (not emojis) for status indicators.
