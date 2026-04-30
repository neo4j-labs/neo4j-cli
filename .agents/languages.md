# Language Stack

## Primary Language: Go

- Module: `github.com/neo4j/cli`
- Go version: 1.25.0+

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI command framework |
| `github.com/spf13/viper` | Configuration management |
| `github.com/spf13/afero` | Filesystem abstraction (enables testing) |
| `github.com/spf13/pflag` | POSIX-compatible flag parsing |
| `github.com/jedib0t/go-pretty/v6` | Table output formatting |
| `github.com/tidwall/gjson` / `sjson` | JSON querying and manipulation |
| `github.com/stretchr/testify` | Test assertions |
| `github.com/google/go-cmp` | Deep comparison in tests |

## License Requirement

All `.go` files must begin with:

```go
// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]
```

Enforced in CI via `addlicense` (`addlicense -f ./addlicense -check ...`).
