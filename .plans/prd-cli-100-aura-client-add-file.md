# PRD: `aura credential add --file` (Import Aura aura-client Credentials File)

Linear: [CLI-100](https://linear.app/neo4j/issue/CLI-100/support-aura-credential-file-format-for-creating-a-new-aura-client)
Source plan: `/Users/oskarhane/.claude/plans/let-s-do-https-linear-app-neo4j-issue-cl-sorted-kite.md`
Mirrors: [CLI-75](https://linear.app/neo4j/issue/CLI-75/support-aura-credential-file-format-for-creating-a-new-dbms-credential) (shipped — `credential dbms add --file`, merged as PR #102 at commit `5e18557`).

## Overview

Add a `--file` flag to `neo4j-cli aura credential add` that imports a Neo4j Aura–exported credentials `.env`-style file containing `CLIENT_ID` / `CLIENT_SECRET` / `CLIENT_NAME`. Direct mirror of CLI-75 (which did the same for `credential dbms add`). To avoid duplicating the small env-file parser introduced in CLI-75, this work also lifts that helper to a new shared package `common/clicfg/envfile/` and updates the dbms call site to import it. The shared `AuraTestHelper` gains a `SeedFile` method so the new file-driven test cases can write into the in-memory FS before `ExecuteCommand` creates the cfg.

## Goals

- Let users import an Aura console–exported aura-client credentials file with one command: `neo4j-cli aura credential add --file ./aura-client.txt`.
- Accept the exact file shape Aura exports today, including comments and blank lines (gotenv handles natively).
- Allow flag overrides: file supplies most fields, `--name` / `--client-id` / `--client-secret` override individual values.
- Use `CLIENT_NAME` as the credential `--name` when `--name` not explicitly passed.
- Lift the CLI-75 parser to a shared package so the dbms and aura-client subtrees share the same implementation.
- Add `SeedFile` to `AuraTestHelper` so file-driven aura-client tests don't need a parallel local helper.
- Preserve flag-based usage: every existing flag continues to work identically; existing tests stay green (with the four required-flag tests updated to match the new RunE-emitted error shape, as in CLI-75).

## Non-Goals

- Stdin / pipe support (`cat creds.txt | aura credential add` or `--file -`). Mirrors CLI-75's locked decision.
- Auto-discovery / `.env` walk-up. Explicit `--file <path>` only.
- `~` / `$VAR` expansion in `--file` path. Shell handles it.
- Interactive prompts when fields are missing. CLI stays non-interactive.
- Mirroring the same flag on `credential embed add`. Separate issue if wanted.
- Moving dbms tests onto the shared `AuraTestHelper`, or moving aura-client `add` tests away from `AuraTestHelper`. Decision: keep both patterns; both gain `SeedFile`-shaped seeding idioms (dbms via its existing local `files map`, aura-client via the new `SeedFile` method on the shared helper).
- Behaviour change for `credential dbms add`. The dbms callsite update is a strict-equivalence refactor: replace inline `parseAuraEnvFile` call with `envfile.Parse` + caller-side key filter; remove the now-unused private helper.

## Requirements

### Functional Requirements

- **REQ-F-001:** Add a new `--file <path>` flag on `neo4j-cli aura credential add` (long form only — no shorthand). Implementation in `neo4j-cli/aura/internal/subcommands/credential/add.go`.
- **REQ-F-002:** When `--file` is set, open the path via `cfg.Aura.Fs()` and parse with the new shared `envfile.Parse` helper (REQ-F-006).
- **REQ-F-003:** Map file keys to `AuraCredential` fields as follows; unrecognised keys silently ignored.

  | File key        | Target field      |
  | --------------- | ----------------- |
  | `CLIENT_ID`     | `client-id`       |
  | `CLIENT_SECRET` | `client-secret`   |
  | `CLIENT_NAME`   | `name` (fallback) |

- **REQ-F-004:** Lift `cmd.MarkFlagRequired(...)` from `--name`, `--client-id`, `--client-secret`. Usage strings (`(required) Name`, etc.) stay unchanged.
- **REQ-F-005:** Field-resolution order in `RunE` (mirrors CLI-75 dbms `add.go:49-137`):
  1. Seed locals from `--file` (track per-key file-presence separately from value).
  2. For each of `name` / `client-id` / `client-secret`: if `cmd.Flag(K).Changed`, overwrite local and mark `flagOverrode[K] = true`.
  3. Empty-value check (REQ-F-008): bail before required-check.
  4. Required check (REQ-F-009).
  5. Call existing `cfg.Credentials.Aura.Add(name, clientId, clientSecret)` (`common/clicfg/credentials/aura.go:26`) unchanged.
- **REQ-F-006:** Create `common/clicfg/envfile/envfile.go` exporting:

  ```go
  func Parse(fs afero.Fs, path string) (vals map[string]string, present map[string]bool, err error)
  ```

  Reads via `fs.Open`, parses with `github.com/subosito/gotenv`. Returns the raw key/value map AND a per-key presence map (so callers can distinguish `KEY=` from missing-key — gotenv collapses both to `""`). Key filtering is the caller's responsibility. Open/read errors wrap as `clierr.NewUsageError("--file %q: %s", path, err.Error())` to preserve CLI-75's user-facing error shape.

- **REQ-F-007:** Update `neo4j-cli/internal/subcommands/credential/dbms/add.go` to import `envfile` and use `envfile.Parse` in place of the private `parseAuraEnvFile`. Move the recognised-keys filter (`NEO4J_URI`, `NEO4J_USERNAME`, `NEO4J_PASSWORD`, `NEO4J_DATABASE`, `AURA_INSTANCENAME`) into the dbms RunE (or a small private dbms helper). Delete the now-unused `parseAuraEnvFile`. Strict-equivalence — no behaviour change; existing CLI-75 tests must stay green untouched.
- **REQ-F-008:** If a required field is empty after merging because the file had the key with an empty value AND no flag overrode it → return `clierr.NewUsageError("--file %q: %s has an empty value", path, K)`. Applies to `CLIENT_ID`, `CLIENT_SECRET`, `CLIENT_NAME`.
- **REQ-F-009:** If a required field is empty after merging because neither file nor flag supplied it → return `clierr.NewUsageError("--%s is required (provide via --file as %s, or pass --%s)", flag, envKey, flag)`. Surface only the first missing field (single-error-per-run; mirrors CLI-75 REQ-F-007).
- **REQ-F-010:** If `--file` points at a missing/unreadable path → return the wrapped clierr from `envfile.Parse` (REQ-F-006). Don't echo file contents in the error message.
- **REQ-F-011:** If `--name` is not passed and the file contains a non-empty `CLIENT_NAME`, use it as the credential name. If `--name` IS passed, it wins; `CLIENT_NAME` is ignored entirely.
- **REQ-F-012:** Flags override file values whenever `cmd.Flag(<name>).Changed` is true.
- **REQ-F-013:** Update `Long` description on `add.go` to briefly mention `--file` and the recognised keys (mirror the dbms `Long` line added in CLI-75 at `neo4j-cli/internal/subcommands/credential/dbms/add.go:44-47`).
- **REQ-F-014:** Extend `AuraTestHelper` (`neo4j-cli/aura/internal/test/testutils/auratesthelper.go`) with a `SeedFile(path, content string)` method. Stash pending writes in a `pendingFiles map[string]string` field; flush them into `helper.fs` at the top of `ExecuteCommand` (immediately after `helper.fs = fs` at line 53) via `afero.WriteFile(...)` with `0o644`. Backwards-compatible: existing tests that don't call `SeedFile` see no behaviour change.

### Non-Functional Requirements

- **REQ-NF-001:** Compatible with the in-memory afero filesystem used in tests (`testutils.AuraTestHelper` already constructs an afero memFs). All file reads must go through `cfg.Aura.Fs()` — no direct `os.Open`.
- **REQ-NF-002:** Cross-platform (Linux, macOS, Windows). Use forward-slash literal paths in `shlex`-parsed command strings (e.g. `/tmp/aura-client.txt`) per CLI-75's "Windows CI Gotchas" learning. `afero.MemMapFs` normalises via `filepath.Clean`.
- **REQ-NF-003:** No new third-party dependencies. `gotenv` is already in `go.mod:48`.
- **REQ-NF-004:** Skill bundle (`neo4j-cli/internal/skill/bundle/references/credential.md`) regenerates cleanly. Per AGENTS.md "Cobra Help / Skill Bundle Rendering Notes", any `Long` / Usage edit on a credential leaf requires `go generate ./neo4j-cli/internal/skill/...` and `TestGenerator_RoundTrip` gates `make test`.
- **REQ-NF-005:** Changelog entry added via `changie new --projects neo4j-cli --kind Minor` (or hand-authored YAML at `.changes/unreleased/neo4j-cli-Minor-<ts>.yaml`).
- **REQ-NF-006:** No secrets in logs or error messages. The `--file` open/parse error must cite the path but never the file contents. Existing redaction patterns (e.g. `PrintableDbmsCredentials` omitting `password`; aura-client's `client-secret` rendered via similar pattern in `common/clicfg/credentials/aura.go`) are unaffected.
- **REQ-NF-007:** All `.go` files start with the Neo4j copyright header (enforced in CI via `addlicense`). Apply to the new `common/clicfg/envfile/envfile.go` and `envfile_test.go`.

## Technical Considerations

- **Helper lift mechanics.** The current `parseAuraEnvFile` in `neo4j-cli/internal/subcommands/credential/dbms/add.go:165-195` already has a clean signature (`(afero.Fs, path) → (vals, present, error)`). Only the recognised-keys filter is dbms-specific. The lift extracts the unfiltered parse+presence-map logic into `common/clicfg/envfile/`; key filtering becomes a per-caller concern. This keeps `envfile.Parse` domain-neutral and reusable for any future `.env` file consumers (embed creds, etc.).
- **Error-shape preservation.** Both callers (dbms refactored, aura-client new) emit `clierr.NewUsageError("--file %q: ...", ...)`. Centralising the open-error wrap in `envfile.Parse` means both call sites get the same shape for free. Keep the wrap inside `envfile` rather than at each call site.
- **SeedFile timing.** `AuraTestHelper.fs` is constructed inside `ExecuteCommand` (line 53), not in `NewAuraTestHelper`. A direct `helper.fs.WriteFile(...)` call from a test BEFORE `ExecuteCommand` would NPE. Stashing pending writes in a `pendingFiles map[string]string` and flushing at the top of `ExecuteCommand` solves this without changing helper construction or affecting other tests.
- **Required-flag handling.** Same rationale as CLI-75: cobra's `MarkFlagsOneRequired` doesn't express "this flag OR --file supplied it"; moving validation into `RunE` is simpler and yields precise per-field error messages. The three existing tests in `add_test.go` that assert cobra's auto-generated `required flag(s) "X" not set` text need updating to match the new RunE-emitted shape (mirrors CLI-75 task-001's test updates).
- **`AuraCredential` struct unchanged.** The struct at `common/clicfg/credentials/aura.go:156-162` has additional fields (`AccessToken`, `TokenExpiry`) that `add` does not populate today and won't populate from the file either. No struct change.
- **No default value.** Unlike dbms (`database-name="neo4j"`), aura-client has no default. Empty `CLIENT_ID=` with no flag override is purely an empty-value error (REQ-F-008), no special-casing needed.
- **CRLF on Windows.** gotenv handles `\r\n` natively; no extra logic needed.
- **Path expansion.** `~` and `$VAR` in `--file` paths are NOT expanded; shell handles `~`. Same decision as CLI-75.
- **Lift blast radius.** The dbms refactor touches a recently-shipped file. Mitigation: lift is strict-equivalence; the unchanged CLI-75 dbms tests act as the regression net. If a dbms test fails post-lift, lift is wrong.

## Acceptance Criteria

- [ ] `neo4j-cli aura credential add --file <path-to-aura-client.txt>` (no other flags) parses the file and stores a credential with name = `CLIENT_NAME`, client-id and client-secret from the file.
- [ ] `aura credential add --file <path> --name custom` stores the credential as `custom`, ignoring `CLIENT_NAME`.
- [ ] `aura credential add --file <path> --client-secret override` stores `override` as the client-secret regardless of the file.
- [ ] Running with `--file` against an empty file → usage error naming the first missing required field; no credential persisted.
- [ ] Running with `--file` against a file with `CLIENT_ID=` (empty value) and NO `--client-id` flag → usage error `--file %q: CLIENT_ID has an empty value`; no credential persisted.
- [ ] Same file but WITH `--client-id real` → success; flag overrides empty.
- [ ] Running with `--file /nonexistent` → wrapped open error citing the path; no credential persisted.
- [ ] Comments (`#`) and blank lines in the file ignored without error.
- [ ] All flags work without `--file` exactly as before — backward compatibility preserved.
- [ ] All three currently-required flags accept being omitted when `--file` supplies them.
- [ ] `common/clicfg/envfile/envfile.go` exists with the exported `Parse(fs, path)` signature; covered by a focused unit test file (`envfile_test.go`) with happy path, comments/blanks, `KEY=` empty value, missing path, unrecognised keys.
- [ ] `neo4j-cli/internal/subcommands/credential/dbms/add.go` imports `envfile`, uses `envfile.Parse`, and no longer defines `parseAuraEnvFile`. ALL existing dbms `add_test.go` cases pass untouched (regression net for the lift).
- [ ] `AuraTestHelper` has a `SeedFile(path, content string)` method; new aura-client `--file` tests use it; existing aura-client tests (list, remove, use) unaffected.
- [ ] `references/credential.md` regenerated and mentions `--file` in the aura-client add section.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check`, `make generate-check` all clean.
- [ ] `TestGenerator_RoundTrip` passes after `go generate ./neo4j-cli/internal/skill/...`.
- [ ] Changelog entry added at `.changes/unreleased/neo4j-cli-Minor-<ts>.yaml`.

## Out of Scope

- Stdin / pipe support.
- Mirroring on `credential embed add`.
- `~` / `$VAR` expansion of the `--file` path.
- An `import` subcommand or batch import.
- Moving dbms tests to the shared `AuraTestHelper` (or vice-versa).
- Any behaviour change for `credential dbms add` beyond the strict-equivalence helper refactor.
- Skill bundle `additions.md` content updates for the new flag — addressed in implementation but content not specified here.

## Open Questions

None. All design questions resolved in the source plan and prior Q&A.
