# Architecture

## Pattern: Cobra Command Tree

The CLI is built as a tree of Cobra commands, one file per leaf command. Directory structure mirrors the command hierarchy.

```
neo4j-cli/
  aura/
    cmd/main.go              # Binary entrypoint
    aura.go                  # Root cobra command, registers top-level subcommands
    internal/
      api/                   # HTTP client wrapping the Neo4j Aura REST API
      flags/                 # Reusable custom flag types (memory, cloud provider, etc.)
      output/                # JSON and table output rendering
      subcommands/           # One directory per resource, one file per action
        instance/
          list.go
          get.go
          create.go
          ...
          snapshot/
            list.go
            ...
        credential/
        tenant/
        config/
        deployment/
        dataapi/graphql/
        graphanalytics/
        import/
        customermanagedkey/
      test/testutils/        # Shared test helpers
common/
  clicfg/                    # Config struct, credential and project management
  clierr/                    # Shared error types
```

## Command Conventions (enforced by CLI guidelines)

- Commands are singular nouns: `instance`, not `instances`
- Structure: `<resource> <action>`, e.g. `instance list`
- Only one positional argument max; extras become flags
- The positional argument always refers to the nearest noun
- `--output json|table` for read commands
- `--await` flag for async operations

## Config & State

`clicfg.Config` (backed by Viper + Afero) holds:
- Named credentials (client ID + secret)
- Active credential
- Project-level configuration

Config file location is OS-specific (handled by `common/clicfg/darwin.go`, `linux.go`, `windows.go`).
