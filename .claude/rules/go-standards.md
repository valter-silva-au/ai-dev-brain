---
paths:
  - "**/*.go"
---

Go coding standards for this project:

- Always wrap errors with context: `fmt.Errorf("descriptive context: %w", err)`
- Return early on errors, avoid deep nesting
- Use interfaces for testability; define interfaces locally in the consuming package when crossing package boundaries
- Use `t.TempDir()` for test file isolation, never write to the real filesystem in tests
- Property tests use `rapid.Check(t, func(t *rapid.T) {...})` with descriptive names (TestPropertyNN_Description)
- File naming: implementation.go, implementation_test.go, implementation_property_test.go
- No import cycles: core/ must not import storage/ or integration/
- Use text/template for dynamic content generation (not string concatenation)
- YAML tags: use `yaml:"field_name"` for all struct fields in models and storage types
- Constructors return interfaces, not concrete types
- File permissions: 0o644 for files, 0o755 for directories
- Cross-package dependencies use the adapter pattern wired in app.go
