---
name: coverage-report
description: Generate detailed test coverage report with per-package breakdown
allowed-tools: Bash, Read
---

# Coverage Report Skill

Generate a detailed test coverage report with per-package breakdown.

## Steps

### 1. Run tests with coverage profiling
```
go test ./... -coverprofile=coverage.out -covermode=atomic
```

### 2. Show per-package coverage
```
go tool cover -func=coverage.out
```
Display coverage for every function in every package.

### 3. Generate HTML report
```
go tool cover -html=coverage.out -o coverage.html
```
Tell the user they can open `coverage.html` in a browser for a visual report.

### 4. Summarize findings

Provide a summary that includes:
- **Total coverage percentage** (from the last line of `go tool cover -func` output)
- **Packages above 80% coverage** - list them as well-covered
- **Packages below 80% coverage** - flag these as needing attention
- **Uncovered functions** - list functions with 0% coverage

### 5. Suggest improvements

Based on the coverage data, suggest specific areas that need more testing:
- Packages with lowest coverage
- Critical functions (error handling, core logic) that lack tests
- Recommend what types of tests would help (unit, integration, property-based)
