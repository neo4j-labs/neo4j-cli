# Architecture

## Pattern: Cobra Command Tree

The CLI is built as a tree of Cobra commands, one file per leaf command. Directory structure mirrors the command hierarchy.

```
cmd/neo4j-cli/main.go        # Binary entrypoint (calls cli.Run)
internal/
  cli/                       # Root cobra command + hooks (NewCmd, Version)
  commands/                  # Cobra surface: one directory per resource, one file per action
    aura/                    # `neo4j-cli aura` root (aura.go) and its resources
      instance/
        list.go
        get.go
        create.go
        ...
        snapshot/
      project/ organization/ agent/ graphql/ graphanalytics/ customermanagedkey/ ...
    docker/ desktop/ admin/ query/ ...
  aura/
    api/                     # HTTP client wrapping the Neo4j Aura REST API
    flags/                   # Reusable custom flag types (memory, cloud provider, etc.)
    output/                  # Aura response printing
    testutils/               # Shared aura test helpers
  clicfg/                    # Config struct, credential and project management
  clierr/                    # Shared error types
```

## Command Conventions (enforced by CLI guidelines)

- Commands are singular nouns: `instance`, not `instances`
- Structure: `<resource> <action>`, e.g. `instance list`
- Only one positional argument max; extras become flags
- The positional argument always refers to the nearest noun
- `--format json|table` for read commands
- `--wait` flag for async operations

## Config & State

`clicfg.Config` (backed by Viper + Afero) holds:
- Named credentials (client ID + secret)
- Active credential
- Project-level configuration

Config file location is OS-specific (handled by `internal/clicfg/darwin.go`, `linux.go`, `windows.go`).

## toon-go Notes

- Module: `github.com/toon-format/toon-go` — imported as `toon "github.com/toon-format/toon-go"` in Go source
- Key API: `toon.Marshal(v any, opts ...toon.EncoderOption) ([]byte, error)` and `toon.WithLengthMarkers(bool) toon.EncoderOption`
- `printToon` in `internal/output/output.go` uses a JSON round-trip (marshal → unmarshal to `any` → toon.Marshal) to honour custom MarshalJSON implementations on concrete ResponseData types before encoding to TOON
- `go mod tidy` promotes toon-go from `// indirect` to a direct dependency automatically once the import is added
