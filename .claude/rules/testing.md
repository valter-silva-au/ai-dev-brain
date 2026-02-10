---
paths:
  - "**/*_test.go"
---

Testing conventions for this project:

- Unit tests use standard testing.T with table-driven patterns
- Property tests use pgregory.net/rapid, named TestPropertyNN_Description
- All file-based tests use t.TempDir() for isolation
- Integration tests are in internal/integration_test.go
- Edge case tests are in internal/qa_edge_cases_test.go
- Run all tests: `go test ./... -count=1`
- Run single package: `go test ./internal/core/ -v`
- Check coverage: `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out`
- On Windows: do not use -race flag (requires CGO/gcc)
- Assert errors with strings.Contains or errors.Is, not direct string comparison
- Use t.Helper() in test helper functions
- Name subtests descriptively in t.Run()
- Create test fixtures programmatically, not from checked-in files
