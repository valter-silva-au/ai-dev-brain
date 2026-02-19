# Contributing to AI Dev Brain

## Prerequisites

- **Go 1.26+** (see `go.mod` for the exact version)
- **golangci-lint** (latest) -- static analysis and linting
- **git** -- worktree support required for integration tests (use `fetch-depth: 0` in CI)
- **govulncheck** (optional) -- `go install golang.org/x/vuln/cmd/govulncheck@latest`

## Development Setup

```bash
git clone https://github.com/valter-silva-au/ai-dev-brain.git
cd ai-dev-brain
go mod download
make build
```

Verify the build:

```bash
./adb version
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Compile the `adb` binary |
| `make test` | Run all tests with race detection |
| `make test-coverage` | Generate HTML coverage report |
| `make test-property` | Run property-based tests only |
| `make lint` | Run golangci-lint |
| `make vet` | Run `go vet` |
| `make fmt` | Format all Go source files |
| `make fmt-check` | Check formatting (fails if files need formatting) |
| `make security` | Run govulncheck |
| `make clean` | Remove build artifacts |
| `make install` | Install to `$GOPATH/bin` |

## Code Organization

```
cmd/adb/main.go           Entry point (ldflags for version info)
internal/
  app.go                   Dependency wiring and adapter structs
  cli/                     Cobra command definitions (one file per command)
  core/                    Business logic and local interface definitions
  storage/                 File-based persistence (YAML, Markdown)
  integration/             External system interaction (git, OS tools)
  observability/           Event logging, metrics, alerting (JSONL-backed)
pkg/models/                Shared domain types
```

See [docs/architecture.md](docs/architecture.md) for detailed diagrams and data flow descriptions.

## Coding Conventions

These conventions are enforced by linters and code review:

- **Error wrapping**: `fmt.Errorf("descriptive context: %w", err)` -- always preserve the error chain.
- **Return early** on errors; avoid deep nesting.
- **Interfaces at the consumer**: define interfaces in the consuming package, not the implementing package. Core defines `BacklogStore`, `ContextStore`, etc. locally to avoid importing storage/integration.
- **Constructors return interfaces**: `func NewFoo(...) FooInterface { return &foo{...} }`.
- **Adapter pattern**: cross-layer dependencies are bridged in `internal/app.go` (e.g., `backlogStoreAdapter`, `eventLogAdapter`).
- **No import cycles**: `core/` must not import `storage/`, `integration/`, or `observability/`.
- **YAML struct tags**: use `yaml:"field_name"` on all persisted types.
- **File permissions**: `0o644` for files, `0o755` for directories.
- **Timestamps**: `time.Now().UTC()` for all timestamps.
- **Templates**: use `text/template` for dynamic content, not string concatenation.

### File Naming

| Type | Pattern | Example |
|------|---------|---------|
| Implementation | `lowercase.go` | `taskmanager.go` |
| Unit tests | `lowercase_test.go` | `taskmanager_test.go` |
| Property tests | `lowercase_property_test.go` | `taskmanager_property_test.go` |
| Package docs | `doc.go` | `doc.go` |

## Testing

### Running Tests

```bash
# All tests
go test ./... -count=1

# All tests with race detection (Linux/macOS only)
go test ./... -race -count=1

# Single package
go test ./internal/core/ -v

# Property-based tests only
go test ./... -run "TestProperty" -count=1 -v

# Coverage report
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
```

Note: the `-race` flag requires CGO and a C compiler. It is not available on Windows runners by default.

### Test Patterns

- **Unit tests**: standard `testing.T` with table-driven subtests and `t.Run()`.
- **Property tests**: `pgregory.net/rapid` with `TestProperty_*` naming. Use `rapid.Check(t, func(rt *rapid.T) { ... })`.
- **Integration tests**: `internal/integration_test.go` for cross-package scenarios.
- **Edge case tests**: `internal/qa_edge_cases_test.go`.
- **Test isolation**: always use `t.TempDir()` for filesystem tests -- never write to the real filesystem.
- **Fakes over mocks**: implement in-memory versions of interfaces (see `inMemoryBacklog` in `internal/core/taskmanager_test.go`).

## Adding a New CLI Command

1. Create `internal/cli/commandname.go` with a Cobra command.
2. Use `RunE` (not `Run`) to return errors.
3. Use `cobra.ExactArgs(N)` or `cobra.MinimumNArgs(N)` for argument validation.
4. Keep the command thin: validate input, call a core service, format output.
5. Add a package-level variable if the command needs a dependency (e.g., `var MyService core.MyInterface`).
6. Wire the variable in `internal/app.go` inside `NewApp()`.
7. Register the command in `internal/cli/root.go`.
8. Add tests and update `docs/commands.md`.

## Adding a New Interface

1. Define the interface in its consuming package (usually `internal/core/`).
2. Implement the concrete type in the appropriate package (`storage/`, `integration/`, etc.) with an unexported struct.
3. Provide a `New*` constructor that returns the interface type.
4. If the interface crosses layers, create an adapter struct in `internal/app.go`.
5. Wire the adapter in `NewApp()`.
6. Add unit tests with an in-memory fake implementation.

## Commit Messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

| Type | When to Use |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `test` | Adding or correcting tests |
| `docs` | Documentation only |
| `chore` | Build, CI, tooling changes |

Include a task ID reference when applicable: `feat(core): add knowledge query command (#TASK-00042)`.

## Pull Requests

1. Create a branch from `main` following the naming convention: `{type}/{task-id}-{description}` (e.g., `feat/TASK-00042-knowledge-query`).
2. Ensure all CI checks pass: lint, test (Linux, macOS, Windows), build, and security scan.
3. Fill out the [PR template](.github/PULL_REQUEST_TEMPLATE.md) with a summary, changes, testing checklist, and general checklist.
4. Keep PRs focused -- one logical change per PR.

### CI Pipeline

The CI pipeline runs on every push to `main` and on pull requests:

- **Lint**: golangci-lint on Ubuntu
- **Test**: `go test ./...` on Ubuntu, macOS, and Windows (race detector on Linux/macOS only)
- **Build**: cross-compilation for linux/darwin/windows on amd64/arm64
- **Security**: govulncheck vulnerability scanning

### Linter Configuration

Enabled linters (`.golangci.yml`): errcheck, govet, ineffassign, staticcheck, unused, gosec, bodyclose, exhaustive, nilerr, unparam.

`gosec` and `unparam` are excluded from test files. Specific gosec rules (G304, G301/G302/G306, G204) are suppressed project-wide with documented rationale in `.golangci.yml`.
