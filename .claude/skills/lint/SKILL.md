---
name: lint
description: Run linting, formatting checks, and static analysis
argument-hint: "[fix]"
allowed-tools: Bash, Read
---

# Lint Skill

Run linting, formatting checks, and static analysis on the codebase.

## Steps

### No arguments (check mode)
Run all checks in sequence and report findings:

1. **Format check** - `gofmt -l .`
   Report any files that are not properly formatted.

2. **Vet check** - `go vet ./...`
   Report any vet findings.

3. **Lint check** - `golangci-lint run`
   Run the full linter suite as configured in `.golangci.yml` (includes errcheck, gosimple, govet, ineffassign, staticcheck, unused, gosec, bodyclose, exhaustive, nilerr, unparam).

### "fix" argument (auto-fix mode)
Auto-fix issues where possible:

1. **Format fix** - `gofmt -s -w .`
   Simplify and format all Go files in place.

2. **Lint fix** - `golangci-lint run --fix`
   Apply automatic fixes for supported linters.

## Output

Report a summary of all findings:
- Number of unformatted files (if any)
- Number of vet issues (if any)
- Number of lint issues by linter/severity
- Overall pass/fail status
