# Contributing to adb

Thanks for considering a contribution. adb is a single-developer-scale tool kept
deliberately boring: file-backed, no server, no database. Contributions that fit that
shape are welcome; ones that add an account or a service usually belong in a separate
project.

## Setup

```bash
git clone https://github.com/valter-silva-au/ai-dev-brain.git
cd ai-dev-brain
make all      # fmt, vet, lint, test, build — must be green before any PR
```

Go 1.25+. `make security` runs govulncheck; `make docker-build` exercises the
multi-stage image.

## How to work

Work test-driven: red → green → refactor. The three levels of Go test, and which to
reach for, are described in
[L400 §8](docs/learning/L400-architecture-and-extending.md). Two rules the end-to-end
level exists to enforce:

- pin `ADB_HOME` *and* `HOME` for any child `adb` process (use
  `internal.NewAppIsolated(dir)` in tests);
- never copy the built binary on macOS Apple Silicon (it invalidates the Go linker's
  ad-hoc code signature).

```bash
go test ./... -race -count=1          # full suite with the race detector
make build                            # ./cmd/adb with version ldflags
make install-local                    # build + install to ~/.local/bin
```

## House conventions

- Wrap errors with `fmt.Errorf("context: %w", err)`; messages start lowercase.
- Define interfaces where they are consumed; constructors return interfaces.
- `time.Now().UTC()` for timestamps. Dirs `0o755`, files `0o644` — with one named
  exception (`sessionstore.go` writes transcripts `0o600`; add to that list with the
  same justification, never quietly).
- `yaml` tags on persisted struct fields.
- Generated YAML frontmatter values go through `{{yamlq}}`/`YAMLScalar` — a plain
  scalar cannot contain `: ` (TASK-00049).
- Keep command handlers thin; logic lives in `internal/core`. New top-level command →
  register it in the CLI root. New event type → declare it, add it to the known set,
  and cover it in the schema test (a drift guard fails the build otherwise).

## Commits and PRs

- Conventional Commits for branch, commit, and PR titles (`feat:`, `fix:`, `docs:`,
  `chore:`, `test:`, `refactor:`, `perf:`, `spike:`).
- Keep PRs small and verifiable; state what you ran, not just what you changed.

## Template provenance boundary

Every document-program template under `templates/programs/` is authored from
**publicly published** sources only, recorded in
[`templates/programs/SOURCES.md`](templates/programs/SOURCES.md). A guard test fails
the build on non-public marker strings (confidentiality stamps, internal hosts) —
if your contribution draws on a non-public source, author the structure from a public
equivalent instead. For every artifact type, one exists.

## CI

CI runs on the maintainer's self-hosted Jenkins hub (the repo's `Jenkinsfile`):
gofmt gate, `go vet`, `golangci-lint`, and the full `go test -race` suite. PRs are
verified there before merge.

## License

MIT — see [LICENSE](LICENSE).
