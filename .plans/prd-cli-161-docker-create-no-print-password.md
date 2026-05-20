# PRD: CLI-161 — `docker create --no-print-password` flag

Linear: https://linear.app/neo4j/issue/CLI-161/docker-create-add-no-print-password-flag-to-suppress-generated
Parent: CLI-159 (Oplane docker subtree threat model, REQ-00065575).

## Overview

`neo4j-cli docker create` emits the generated Neo4j password in the rendered output (table/JSON/TOON) at `neo4j-cli/internal/subcommands/docker/create.go:375-386`. This is correct by default — the operator needs the password — but operators who:

- pipe stdout into logs / CI artifacts,
- post-process JSON output with `jq`,
- prefer to retrieve the password later via `credential dbms get`,

have no opt-out today. They must `2>&1 >/dev/null` (losing structured output) or pipe through `jq 'del(.password)'`.

This card adds a `--no-print-password` flag (default `false`) that omits the `password` field from the rendered output map and field slice. The credential is still stored (unless `--no-store-credential`) so the password remains recoverable via `neo4j-cli credential dbms get <name>`.

Oplane grade: REQ-00065575 PARTIALLY_IMPLEMENTED → IMPLEMENTED after this card. Severity High, but moderate value / low cost — gap is "no operator opt-out", not a leak.

## Goals

- Add `--no-print-password` boolean flag to `neo4j-cli docker create` (default `false`). When set, the `password` field is absent from the rendered output across all formats (`--format default|json|toon|table`).
- Reject combinations that would render the password unrecoverable, with clear `clierr.UsageError` messages.
- Document the flag in `README.md`, the skill bundle additions (`additions.md`), and the embedded `--help` text so the bundle round-trip test stays green.

## Non-Goals

- Redacting the password from `docker run` stderr / argv echo — already handled by `redactArgs` (`neo4j-cli/internal/subcommands/docker/client.go:151-180`).
- Changing the default behaviour. The current "print password to stdout" default is intentional and preserved.
- Suppressing the ephemeral `.env` blob output. `--no-print-password` is incompatible with `--ephemeral` (see edge case rules below); operators wanting the blob in a file should use the existing `--env-out-file <path>` flag.
- Changing `.env` file write semantics (mode 0o600 + atomic temp-rename via `writeEnvFile`).
- Changing the password generation pipeline (`crypto/rand` → 16 bytes → base64 URL-safe).

## Requirements

### Functional Requirements

- REQ-F-001: Add `noPrintPassword bool` to the `var (...)` block in `newCreateCmd` (around `create.go:95`, next to `noStoreCredential`). Add `noPrintPasswordFlag = "no-print-password"` to the matching `const (...)` block (around `create.go:112`).

- REQ-F-002: Register the flag with `pflag` (around `create.go:400`, after `noStoreCredential`):
  ```go
  cmd.Flags().BoolVar(&noPrintPassword, noPrintPasswordFlag, false,
      "Don't include the generated password in stdout output. Retrieve later via `neo4j-cli credential dbms get <name>`.")
  ```

- REQ-F-003: Add two validation branches in `RunE` immediately after the existing ephemeral/no-store check at `create.go:181-183`:
  ```go
  if noPrintPassword && ephemeral {
      return clierr.NewUsageError(
          "--%s is incompatible with --%s (ephemeral emits a .env blob; use --%s to write it to a file)",
          noPrintPasswordFlag, ephemeralFlag, envOutFileFlag,
      )
  }
  if noPrintPassword && noStoreCredential && password == "" {
      return clierr.NewUsageError(
          "--%s with --%s would discard the generated password unrecoverably; supply --%s explicitly or drop one of the flags",
          noPrintPasswordFlag, noStoreCredentialFlag, passwordFlag,
      )
  }
  ```
  - The `--password != ""` exception means the operator already knows the password, so suppression-without-storage is safe.

- REQ-F-004: Modify the output-construction block at `create.go:375-386` to conditionally include the password:
  ```go
  row := map[string]any{
      "name":      chosenName,
      "edition":   edition,
      "version":   version,
      "bolt-port": boltPort,
      "http-port": httpPort,
      "uri":       uri,
      "username":  "neo4j",
  }
  fields := []string{"name", "edition", "version", "bolt-port", "http-port", "uri", "username"}
  if !noPrintPassword {
      row["password"] = resolvedPassword
      fields = append(fields, "password")
  }
  commonoutput.PrintBodyMap(cmd, cfg, singleRow{row: row}, fields)
  ```
  - `PrintBodyMap` honours the `fields` slice consistently across JSON/table/TOON, so omitting the field at both the map and slice level suppresses it from every format.

- REQ-F-005: Extend the `Long:` description on `newCreateCmd` (`create.go:124-144`) with a single short sentence noting `--no-print-password` so it surfaces in `--help` and the regenerated skill bundle.

- REQ-F-006: Update `README.md:172` (the existing "Heads up: the generated password is part of the standard `create` output…" paragraph) to mention `--no-print-password` as the recommended opt-out for operators who still want the credential stored:
  > Pass `--password <s>` to choose the password yourself, `--no-print-password` to suppress it from stdout (retrieve later via `credential dbms get <name>`), or `--no-store-credential` if you want neither a stored credential nor the rendered password.

- REQ-F-007: Update `neo4j-cli/internal/skill/additions.md:26` (the parallel one-liner) with the same `--no-print-password` mention.

- REQ-F-008: Regenerate the skill bundle:
  ```sh
  go generate ./neo4j-cli/internal/skill/...
  ```
  Required because the `--help` text for `docker create` changes (REQ-F-002 / REQ-F-005) and the bundle embeds it. `TestGenerator_RoundTrip` is the gate.

- REQ-F-009: Changelog entry:
  ```sh
  changie new --projects neo4j-cli --kind Minor \
      --body "docker create: --no-print-password suppresses generated password from stdout output (CLI-161)"
  ```

### Non-Functional Requirements

- REQ-NF-001: Test coverage — colocated in `neo4j-cli/internal/subcommands/docker/create_test.go`, mirroring the existing patterns (`TestCreate_NoStoreCredential_SkipsPersistence` around line 222, `TestCreate_GeneratedPassword_*` around line 302). Prefer table-driven where the case shapes match. Must cover:
  - JSON format: `password` key absent (decode via `json.Unmarshal` into `[]map[string]any`).
  - Default/table format: `password` substring absent from `stdout` (and other non-secret fields still present).
  - TOON format: `password` key absent.
  - Regression: omitting `--no-print-password` continues to render password (default behaviour preserved).
  - Recoverability: when `--no-print-password` is set alone, the stored dbms credential contains the generated password (assert via `cfg.Credentials.Dbms.List()`), so `credential dbms get <name>` would surface it.
  - Edge case A: `--no-print-password --no-store-credential` without `--password` → UsageError with the documented hint, exit 2, no docker side effect (`fake.RunCalls` empty).
  - Edge case B: `--no-print-password --no-store-credential --password=<value>` → success, password suppressed in output, no credential stored.
  - Edge case C: `--no-print-password --ephemeral` → UsageError, no docker side effect.
  - Stderr cleanliness: stdout AND stderr combined do NOT contain the resolved password substring on the happy path with `--no-print-password` (regression test for unintended leaks).

- REQ-NF-002: All gates clean: `make fmt-check`, `make lint`, `make test`, `make generate-check`, `make license-check`. CI runs the test matrix on linux/macos/windows.

- REQ-NF-003: Any new `.go` file (none expected — edits are all in existing files) starts with the standard Neo4j copyright header.

- REQ-NF-004: No new dependencies. Implementation uses existing imports (`clierr`, `commonoutput`, `pflag`).

## Technical Considerations

### Why suppress at the map+slice level

`commonoutput.PrintBodyMap(cmd, cfg, body, fields)` iterates `fields` and reads keys from the map for each emitted row. Both JSON encoding and table/TOON rendering honour the same field slice, so omitting `"password"` from both the `row` map AND the `fields` slice gives a single point of suppression that covers every output format without any format-specific branching.

Not viable alternatives considered:
- Leave the password in `row` but drop only from `fields`: the JSON encoder would still include it if `singleRow.MarshalJSON` ever shifts to a struct-of-all-keys encoding. Belt-and-braces: drop both.
- Render then post-process the string: brittle (TOON / table format strings change), and would re-introduce the secret in transient memory paths.

### Recoverability semantics

The flag is meaningful only when the password remains recoverable by some other means. Three sources:

| Source | When | Recoverability |
| -- | -- | -- |
| Stored dbms credential | Default — credential stored by `cfg.Credentials.Dbms.Add` (`create.go:333`) | `credential dbms get <name>` returns the password |
| Operator-supplied `--password` | When user passed their own | Operator already knows it |
| `.env` file via `--env-out-file` | Ephemeral path only | File on disk (mode 0o600) contains password |

The two validation branches in REQ-F-003 eliminate the only combos where ALL three sources are absent:

- `--no-print-password && --no-store-credential` with no `--password` → no stored credential, operator doesn't know it, nothing to retrieve.
- `--no-print-password && --ephemeral` → ephemeral skips credential storage by design, and the ephemeral output path is the `.env` blob (a separate channel that `--no-print-password` doesn't affect). Operators wanting a clean stdout on the ephemeral path should use `--env-out-file <path>` (existing flag, already silent on stdout, writes 0o600).

The ephemeral incompatibility parallels the existing `--no-store-credential && --ephemeral` rejection at `create.go:181-183` — keeps the flag matrix easy to reason about.

### Reusable utilities (already exist)

- `clierr.NewUsageError(msg, args...)` — `common/clierr/error.go:111`. Printf-style. Exit code 2.
- `commonoutput.PrintBodyMap(cmd, cfg, body, fields)` — `common/output/`. Honours `fields` for JSON/table/TOON.
- `flags.RegisterOutputFlag` — already wired on the `docker` parent.
- `singleRow` + `MarshalJSON` — `create.go:489-504`. Continues to work because it just calls `AsArray()` over the (now-shorter) `row` map.

### No new test seams needed

`clientFactory`, `randSource`, `listenerFactory`, `waitForBoltFn`, `homeDirFn` are all already in place. The new tests reuse `runCreate` / `runCreateWithOccupiedPortsAndStderr` (`create_test.go:90, 122`).

### Skill bundle round-trip

`--help` for `docker create` is embedded in `neo4j-cli/internal/skill/bundle/**`. Touching the flag set OR the `Long:` text changes the rendered help, which means:

1. Local: `go generate ./neo4j-cli/internal/skill/...` after the source edits, commit the regenerated bundle alongside the source change.
2. CI: `make generate-check` would fail otherwise.

`TestGenerator_RoundTrip` is the gate; it surfaces as `references/docker.md differs`.

### Files touched

- `neo4j-cli/internal/subcommands/docker/create.go` — flag declaration, registration, validation, output construction.
- `neo4j-cli/internal/subcommands/docker/create_test.go` — new tests.
- `README.md` — `:172` paragraph update.
- `neo4j-cli/internal/skill/additions.md` — `:26` one-liner update.
- `neo4j-cli/internal/skill/bundle/**` — regenerated via `go generate`.
- `.changes/unreleased/neo4j-cli-Minor-<timestamp>.yaml` — via `changie new`.

### Branch & PR

- Branch: `oskar/cli-161-no-print-password` (per user's `oskar/` prefix convention).
- PR title: `feat: docker create --no-print-password flag (CLI-161)`.

## Acceptance Criteria

- [ ] `--no-print-password` flag exists on `neo4j-cli docker create`, default `false`, help text reads "Don't include the generated password in stdout output. Retrieve later via `neo4j-cli credential dbms get <name>`."
- [ ] When `--no-print-password` is set and the command succeeds, the rendered output (JSON, default/table, TOON) contains no `password` key/column/field, and the password substring is absent from combined stdout+stderr.
- [ ] When `--no-print-password` is omitted (default), the password is rendered exactly as today (regression preserved).
- [ ] `--no-print-password --no-store-credential` without `--password` → `clierr.UsageError`, exit code 2, no docker invocation occurs (`fake.RunCalls` empty in test). Error message matches the documented hint.
- [ ] `--no-print-password --no-store-credential --password=<value>` → success, password suppressed from output, credential NOT stored.
- [ ] `--no-print-password --ephemeral` → `clierr.UsageError`, exit code 2, no docker invocation occurs. Error message names `--env-out-file` as the alternative.
- [ ] When `--no-print-password` is set alone, the dbms credential IS stored with the generated password (`cfg.Credentials.Dbms.List()` returns the entry), so `credential dbms get <name>` would retrieve it. (Recovery path is exercisable.)
- [ ] `README.md:172` paragraph and `neo4j-cli/internal/skill/additions.md:26` one-liner both mention `--no-print-password`.
- [ ] Skill bundle regenerated via `go generate ./neo4j-cli/internal/skill/...`; `make generate-check` passes.
- [ ] Changelog entry under `.changes/unreleased/` (`Minor`) describing the new flag, referencing `(CLI-161)`.
- [ ] `make fmt-check && make lint && make test && make generate-check && make license-check` all clean.

## Out of Scope

- Stderr / argv redaction (already handled by `redactArgs`).
- Ephemeral `.env` blob suppression — operators wanting silent stdout on ephemeral use `--env-out-file`.
- Default-on behaviour or auto-suppression based on `!isatty(stdout)`.
- Changes to other docker leaves (`list`, `get`, `start`, `stop`, `delete`).
- Pre-flight quota checks, retries, or any docker-CLI behaviour.
- Cross-leaf consistency review of "should other leaves also have `--no-print-password`?" — only `create` emits a generated password.

## Open Questions

(none — design locked in plan-mode discussion: ephemeral combo rejected outright; password recovery flows through stored credential or `--password` or `--env-out-file`.)
