# PRD: Unify Config and Credential Commands

## Overview

Standardise the `config` and `credential` command trees across `neo4j-cli` and `aura-cli` so they follow consistent resource-verb-target ordering, use dot-notation for config key scoping, and expose a uniform interface regardless of which binary the user invokes.

## Goals

- Remove the `neo4j config aura` subcommand group and replace it with a single unified `neo4j config` that accepts dot-notation keys (e.g. `aura.default-tenant`).
- Make `aura-cli` config commands flat while resolving scope internally, so `aura config get default-tenant` and `neo4j config get aura.default-tenant` are equivalent. `aura config get output` and `neo4j config get output` are equivalent 
- Simplify `neo4j credential list` by removing the positional type argument; listing always returns all stored credentials.
- Move `neo4j config aura project` to `neo4j config project`.

## Non-Goals

- Adding new credential types (e.g. `db-user`) or new config keys.
- Adding a `--type` filter flag to `credential list`.
- Any changes to how credentials or config are stored on disk.
- Changes to the beta-gated project subcommand behaviour beyond the path rename.

## Requirements

### Functional Requirements

#### Config — neo4j-cli

- REQ-F-001: `neo4j config list` lists all configuration keys (global and aura-scoped) in a single table/JSON response. Aura-scoped keys appear with the `aura.` prefix (e.g. `aura.base-url`). Global keys appear flat (e.g. `output`). `output` is a global key only — it appears once as `output`, never as `aura.output`.
- REQ-F-002: `neo4j config get <key>` accepts both flat global keys (e.g. `output`) and dot-notation aura keys (e.g. `aura.default-tenant`). Tab-completion (`ValidArgs`) covers all valid keys in both namespaces.
- REQ-F-003: `neo4j config set <key> <value>` accepts the same dot-notation key space as `get`, validates values where applicable (e.g. `output` must be one of `default`, `json`, `table`), and persists to the correct viper namespace.
- REQ-F-004: The `neo4j config aura` subcommand group (`neo4j config aura list`, `neo4j config aura get`, `neo4j config aura set`) is removed entirely.
- REQ-F-005: `neo4j config project` (add/list/use/remove) is moved from `neo4j config aura project` to `neo4j config project`. Behaviour and flags are unchanged.

#### Config — aura-cli

- REQ-F-006: `aura config list` lists all accessible keys flat — both aura-scoped keys (shown without prefix: `default-tenant`) and global keys (shown without prefix: `output`). `output` appears exactly once; it is a global key and is never duplicated as an aura-scoped key.
- REQ-F-007: `aura config get <key>` accepts flat keys for both namespaces. Internally, aura-scoped keys resolve to the `aura.<key>` viper path; global keys resolve to the top-level viper path. `output` resolves as a global key only — `aura.output` is not a valid key.
- REQ-F-008: `aura config set <key> <value>` accepts the same flat key space as `aura config get` with equivalent resolution logic.
- REQ-F-009: `aura config project` (add/list/use/remove) is unchanged (already at the correct path).

#### Credentials — neo4j-cli

- REQ-F-010: `neo4j credential list` lists all stored credentials with no positional type argument. The positional `aura-client` argument currently required is removed.
- REQ-F-011: `neo4j credential add aura-client [flags]` is unchanged — type remains a subcommand.
- REQ-F-012: `neo4j credential remove aura-client <name>` is unchanged.
- REQ-F-013: `neo4j credential use aura-client <name>` is unchanged.
- REQ-F-014: `neo4j credential list` output columns are `name`, `type`, `identifier`. The column header renders as `IDENTIFIER` regardless of credential type. For `aura-client`, the value in the `identifier` column is the `client-id`.

#### Credentials — aura-cli

- REQ-F-015: `aura credential add` is unchanged.
- REQ-F-016: `aura credential list` already lists all credentials with no type argument — no change needed, but output columns must be consistent with `neo4j credential list`: `name`, `type`, `identifier` with the `IDENTIFIER` column header. This should still only list `aura-client` credentials.
- REQ-F-017: `aura credential remove <name>` and `aura credential use <name>` are unchanged.

### Non-Functional Requirements

- REQ-NF-001: All commands continue to support `--output json` and `--output table` flags. Default rendering matches existing behaviour.
- REQ-NF-002: All tests pass (`make test`). New behaviour is covered by table-driven tests following the existing patterns in `*_test.go` files.
- REQ-NF-003: All `.go` files retain the Neo4j copyright header.

## Technical Considerations

- **Go `internal` package isolation:** `neo4j-cli/internal/subcommands/config` cannot import `neo4j-cli/aura/internal/subcommands/config` directly. The bridge is the non-internal wrapper (e.g. `aura/config.go`). The unified `neo4j config` command must go through this bridge or be implemented in `neo4j-cli/internal/subcommands/config` with the dot-notation resolution logic duplicated or extracted to `common/`. Try to avoid duplication of logic.
- **Dot-notation key resolution:** `neo4j config get aura.default-tenant` must strip the `aura.` prefix and delegate to the AuraConfig getter. The resolution logic (split on first `.`, look up namespace) should live in a shared helper to avoid duplication between `get.go` and `set.go`.
- **ValidArgs coverage:** `neo4j config get` and `neo4j config set` currently only offer `cfg.Global.ValidConfigKeys` for tab-completion. After this change they must offer `cfg.Global.ValidConfigKeys` plus `aura.<key>` for each key in `cfg.Aura.ValidConfigKeys` — with the exception of any key that also appears in `cfg.Global.ValidConfigKeys` (e.g. `output`). Such keys must not be emitted as `aura.<key>` since `aura.output` is not a valid key.
- **`neo4j config aura project` path change:** The `project` subcommand tree currently mounts under the `aura` group. After removing `neo4j config aura`, it must be mounted directly under `neo4j config`. The `cmd.Use` field must be updated to `"project"` if it isn't already.
- **Removing the positional type arg from `neo4j credential list`:** The existing `list.go` in `neo4j-cli/aura/internal/subcommands/credential/` uses a sub-subcommand (`aura-client`) for list. This sub-subcommand must be removed; the parent `list` command becomes the leaf that iterates all credential stores. Cobra's `legacyArgs` behavior means child commands accept arbitrary positional args by default — `Args: cobra.NoArgs` must be set explicitly on the `list` command, otherwise `neo4j credential list aura-client` silently succeeds instead of erroring.
- **`cobra.EnableTraverseRunHooks`**: already set globally in the test helper — no change needed.

## Acceptance Criteria

- [ ] `neo4j config list` shows both global and aura keys with dot-notation aura prefixes in a single response.
- [ ] `neo4j config get output` returns the global output setting.
- [ ] `neo4j config get aura.default-tenant` returns the aura default-tenant value.
- [ ] `neo4j config set aura.base-url <value>` persists to the `aura.base-url` viper key.
- [ ] `neo4j config aura list/get/set` no longer exists (command not found).
- [ ] `neo4j config project list/add/use/remove` works at the new path.
- [ ] `aura config list` shows all keys flat (no `aura.` prefix), including global keys.
- [ ] `aura config get default-tenant` returns the same value as `neo4j config get aura.default-tenant`.
- [ ] `aura config set default-tenant <value>` updates the same underlying config as `neo4j config set aura.default-tenant <value>`.
- [ ] `neo4j credential list` (no arguments) lists all credentials. Secrets should never be printed.
- [ ] `neo4j credential list aura-client` returns an error or shows help (positional arg removed).
- [ ] `aura credential add --name X --client-id Y --client-secret Z` works.
- [ ] `make test` passes with no failures.

## Out of Scope

- `db-user` credential type.
- `--type` filter flag on `credential list`.
- New config keys or new config scopes.
- Backward-compatibility shims or deprecation warnings for removed command paths.
- Changes to config file format or storage location.

## Open Questions

- None remaining — all clarifications resolved during PRD Q&A.
