# Testing Framework

## Framework

Go standard `testing` package + `testify` for assertions.

## Running Tests

```bash
# Canonical command (preferred)
make test

# Verbose output
go test -v ./...
```

## Test Structure

Tests live alongside source files as `*_test.go`. Integration-style tests use:

- `github.com/spf13/afero` — in-memory filesystem via `afero.NewMemMapFs()` to avoid touching disk
- `testutils.AuraTestHelper` — helper in `neo4j-cli/aura/internal/test/testutils/` for constructing test commands with mock HTTP handlers
- `testutils.RequestHandlerMock` — mock HTTP server for API calls
- `github.com/google/go-cmp` — deep equality checks on structured output

## Test Helpers Location

```
neo4j-cli/aura/internal/test/testutils/
  auratesthelper.go      - AuraTestHelper wrapping cobra cmd + mock server
  requesthandlermock.go  - HTTP handler mock
  formatjson.go          - JSON formatting helpers
test/utils/testfs/
  testfs.go              - Shared filesystem test utilities
```

## CI Matrix

Tests run on `ubuntu-latest`, `windows-latest`, and `macos-latest` on every push/PR to `main`.
