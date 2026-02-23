---
name: security
description: Run security scans including vulnerability checks and code auditing
allowed-tools: Bash, Read, Grep
---

# Security Skill

Run security scans and vulnerability checks on the codebase.

## Steps

Run the following checks in sequence:

### 1. Dependency vulnerability scan
```
govulncheck ./...
```
Check all dependencies for known vulnerabilities. Report any findings with CVE IDs, affected packages, and severity.

### 2. Code security analysis
```
golangci-lint run --enable gosec
```
Run the gosec linter for code-level security issues (already enabled in `.golangci.yml`, but this ensures it runs even if the config changes). Look for:
- Hardcoded credentials
- SQL injection risks
- Path traversal vulnerabilities
- Insecure crypto usage
- Unhandled errors in security-sensitive operations

### 3. Go module integrity check
Verify that `go.sum` is consistent:
```
go mod verify
```
This checks that dependencies on disk match the expected hashes in `go.sum`.

## Output

Report findings organized by severity:
- **Critical**: Vulnerabilities with known exploits or high CVSS scores
- **High**: Security issues that should be fixed before release
- **Medium**: Code quality issues with security implications
- **Low/Info**: Informational findings and best-practice suggestions

Provide a summary count at the end: X critical, Y high, Z medium, W low.
If no issues are found, report a clean bill of health.
