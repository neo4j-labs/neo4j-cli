# PRD: `credential dbms add --file` (Import Aura Credentials File)

Linear: [CLI-75](https://linear.app/neo4j/issue/CLI-75/support-aura-credential-file-format-for-creating-a-new-dbms-credential)
Source plan: `/Users/oskarhane/.claude/plans/let-s-do-https-linear-app-neo4j-issue-cl-sorted-kite.md`

## Overview

Add a `--file` flag to `neo4j-cli credential dbms add` that imports a Neo4j Aura–exported credentials `.env`-style file and turns it into a stored dbms credential in one command. Users who provision an Aura instance via the console today receive a `.txt` file with the connection details; without this feature they must hand-copy each field into four separate flags. The flag is long-form only (no shorthand), supports flag overrides for any field, and lets `AURA_INSTANCENAME` supply `--name` when not explicitly passed.

## Goals

- Make a freshly-provisioned Aura instance usable from the CLI with a single command: `neo4j-cli credential dbms add --file aura-creds.txt`.
- Accept the **exact** file format Aura exports today, including comments, blank lines, and the `AURA_INSTANCEID` / `AURA_INSTANCENAME` keys.
- Preserve flag-based usage for users who don't have a file: every existing flag continues to work as before; existing commands keep working unchanged.
- Allow partial overrides — e.g. file supplies most fields, `--password` overrides the password from the file.
- Fail fast and clearly on malformed inputs (empty values, missing required fields, unreadable file).

## Non-Goals

- Stdin / pipe support (`cat creds.txt | … add` or `--file -`). User explicitly chose `--file <path>` only — see Open Questions.
- A new `import` subcommand (e.g. `credential dbms import`). Reuse the existing `add` leaf.
- Equivalent file support for `credential aura-client add` or `credential embed add`. Separate issues if wanted.
- Auto-discovering a `.env` / Aura file in the working tree (no walk-up). Explicit path only.
- `~` / environment-variable expansion inside the path. Shell handles it.
- Interactive prompts when fields are missing. CLI stays non-interactive.

## Requirements

### Functional Requirements

- **REQ-F-001:** Introduce a new `--file <path>` flag on `neo4j-cli credential dbms add` (long form only; no shorthand to avoid collision with the `-f` / `--format` shorthand registered on the parent).
- **REQ-F-002:** When `--file` is set, open the path via `cfg.Aura.Fs()` and parse it with `github.com/subosito/gotenv` (already a direct dependency used at `neo4j-cli/query/connect.go:315`).
- **REQ-F-003:** Map file keys to `DbmsCredential` fields as follows; unrecognised keys are silently ignored.

  | File key            | Target field      |
  | ------------------- | ----------------- |
  | `NEO4J_URI`         | `uri`             |
  | `NEO4J_USERNAME`    | `username`        |
  | `NEO4J_PASSWORD`    | `password`        |
  | `NEO4J_DATABASE`    | `database-name`   |
  | `AURA_INSTANCENAME` | `name` (fallback) |
  | `AURA_INSTANCEID`   | ignored           |

- **REQ-F-004:** Lift `cmd.MarkFlagRequired(...)` from `--name`, `--username`, `--password`, `--uri`. Their flag Usage strings remain unchanged so `--help` stays terse.
- **REQ-F-005:** Field-resolution order in `RunE`:
  1. Seed locals from `--file` (track per-key "was this key present?" separately from value).
  2. For each of `name` / `username` / `password` / `uri` / `database-name` / `embed-credential`: if `cmd.Flag(<name>).Changed`, overwrite the local AND mark `flagOverrode[K] = true`.
  3. If `databaseName == ""` AND `--database-name` not changed AND `NEO4J_DATABASE` not present in file → apply default `"neo4j"`.
  4. Validate required-ness (REQ-F-006 / REQ-F-007).
  5. Run the existing `Embed.Get` validation + `Dbms.Add(...)` + `Dbms.SetEmbed(...)` paths unchanged.
- **REQ-F-006:** If a required field (`name`, `username`, `password`, `uri`) is empty after merging because the file had the key but with an empty value AND no flag overrode it → return `clierr.NewUsageError("--file %q: %s has an empty value", path, K)`. Same rule applies to `database-name`: an empty `NEO4J_DATABASE=` with no `--database-name` flag override is an error (default `"neo4j"` is NOT applied — the user explicitly typed an empty value).
- **REQ-F-007:** If a required field is empty after merging because neither file nor flag supplied it → return `clierr.NewUsageError("--<flag> is required (provide via --file as %s, or pass --<flag>)", K)`. Surface only the first missing field (single-error-per-run; existing test infra asserts a single message).
- **REQ-F-008:** If `--file` points at a missing/unreadable path → return a wrapped clierr in the shape `credential dbms add: --file %q: <underlying error>` so the failing path is visible.
- **REQ-F-009:** If `--name` is not passed and the file contains a non-empty `AURA_INSTANCENAME`, use it as the credential name. If `--name` IS passed, it wins and `AURA_INSTANCENAME` is ignored entirely.
- **REQ-F-010:** Flags override file values whenever `cmd.Flag(<name>).Changed` is true. Specifically: `--password override` next to `--file creds.txt` stores `override`, regardless of what's in the file. Mirrors `query`'s `.env` < OS env < flag precedence at `neo4j-cli/query/connect.go:167-185`.
- **REQ-F-011:** `--embed-credential` continues to behave as before — validated against `Embed.Get(...)` before any persistence, applied via `Dbms.SetEmbed(...)` after `Dbms.Add` succeeds. The file MAY supply it (if a future Aura format adds the key), but the canonical Aura file does not include it; default behaviour is "not linked".

### Non-Functional Requirements

- **REQ-NF-001:** Compatible with the in-memory afero filesystem used in tests (`test/utils/testfs`). Don't introduce real-filesystem reads anywhere in the code path — use `cfg.Aura.Fs()`.
- **REQ-NF-002:** Cross-platform (Linux, macOS, Windows). CI runs on all three matrices per `.github/workflows/test.yml`. No `path/filepath` vs forward-slash assumptions baked into tests; use `filepath.Join` for any path constructed in tests.
- **REQ-NF-003:** No new third-party dependencies. `gotenv` is already in `go.mod` (line 48).
- **REQ-NF-004:** Skill bundle (`neo4j-cli/internal/skill/bundle/references/credential.md`) regenerates cleanly. Per CLAUDE.md "Cobra Help / Skill Bundle Rendering Notes", any `Long` / Usage edit on a credential leaf requires `go generate ./neo4j-cli/internal/skill/...` and `TestGenerator_RoundTrip` gates `make test`.
- **REQ-NF-005:** Changelog entry added via `changie new --projects neo4j-cli --kind Minor` (or hand-authored YAML at `.changes/unreleased/neo4j-cli-Minor-<ts>.yaml`) since this is a user-visible feature.
- **REQ-NF-006:** No secrets in logs or error messages. The `--file` open/parse error must not echo the file contents (only the path). Existing redaction patterns (e.g. `PrintableDbmsCredentials` omitting `password`) are unaffected by this change.

## Technical Considerations

- **Reuse target — `gotenv.Parse(io.Reader) map[string]string`** (`neo4j-cli/query/connect.go:315`). It already handles comments, blank lines, and quoted values. Avoid re-implementing dotenv parsing.
- **`cfg.Aura.Fs()`** is the afero `Fs` used everywhere; test code injects `testfs.GetTestFs(...)`. New helper `parseAuraEnvFile(fs afero.Fs, path string) (map[string]string, present map[string]bool, err error)` keeps the value/presence distinction explicit (gotenv collapses `KEY=` and missing-key both to `""` in the value map).
- **Why drop `MarkFlagRequired`** instead of using `cmd.MarkFlagsOneRequired(...)`: cobra has no built-in "this flag OR that flag" group for FOUR flags being satisfied by one file. Moving validation into `RunE` is simpler and gives more precise error messages.
- **Test seam pattern.** `dbmsTestHelper` (in `add_test.go`) currently creates the in-memory FS inside `executeCommand`. Add an optional `files map[string]string` field on the helper; populate it into `h.fs` immediately after `testfs.GetTestFs(...)` returns. Tests then reference paths like `/tmp/creds.txt` (memFS root) in their `command` strings.
- **Error-message stability.** Four existing test cases assert on cobra's auto-generated `required flag(s) "X" not set` text. Those messages disappear when `MarkFlagRequired` is removed. Update those four cases to match the new RunE-emitted message (REQ-F-007). No external consumer depends on the exact text.
- **`--database-name` default.** The current `"neo4j"` default kicks in via `cmd.Flags().StringVar(&databaseName, ..., "neo4j", ...)`. With the new resolution rules, the flag default still loads into the local — we need to distinguish "user passed --database-name with that value" from "default applied". Use `cmd.Flag("database-name").Changed`. The default itself doesn't need to change; the resolution logic just needs to consult `Changed` before treating the value as user-supplied.
- **Path expansion.** `~` and `$VAR` in `--file` paths are NOT expanded by the code. The shell handles `~`; users in restricted shells can substitute themselves. Documented in additions.md (out of scope here).
- **CRLF on Windows.** gotenv handles `\r\n` natively; no extra logic needed. Aura-exported files may have either line ending depending on the user's OS.

## Acceptance Criteria

- [ ] `neo4j-cli credential dbms add --file <path-to-aura.txt>` (no other flags) parses the file and stores a credential with name = `AURA_INSTANCENAME` and all four connection fields populated.
- [ ] `neo4j-cli credential dbms add --file <path> --name custom` stores the credential as `custom`, ignoring `AURA_INSTANCENAME`.
- [ ] `neo4j-cli credential dbms add --file <path> --password override` stores `override` as the password regardless of the file's `NEO4J_PASSWORD`.
- [ ] Running with `--file` against an empty file → usage error naming the first missing required field; no credential persisted.
- [ ] Running with `--file` against a file with `NEO4J_URI=` (empty value) and NO `--uri` flag → usage error `--file %q: NEO4J_URI has an empty value`; no credential persisted.
- [ ] Running with `--file` against the same file but WITH `--uri bolt://x` → success; flag overrides empty.
- [ ] Running with `--file` against a file with `NEO4J_DATABASE=` and no `--database-name` flag → usage error (default `"neo4j"` NOT applied silently).
- [ ] Running with `--file /nonexistent` → wrapped open error citing the path; no credential persisted.
- [ ] Running `--file` + `--embed-credential <missing>` → embed-validation error fires; no credential persisted (`Dbms.Add` not called).
- [ ] Running `--file` + `--embed-credential <existing>` → credential stored AND linked to embed.
- [ ] All flags work without `--file` exactly as before — backward compatibility preserved.
- [ ] Comments (`# ...`) and blank lines in the file are ignored without error (gotenv handles natively).
- [ ] `AURA_INSTANCEID` is read from the file and discarded without warning or error.
- [ ] All new and updated test cases pass under `make test` (Linux, macOS, Windows matrices).
- [ ] `make fmt-check`, `make lint`, `make generate-check` all clean.
- [ ] `TestGenerator_RoundTrip` passes after `go generate ./neo4j-cli/internal/skill/...`.
- [ ] Changelog entry added at `.changes/unreleased/neo4j-cli-Minor-<ts>.yaml`.

## Out of Scope

- Stdin / pipe support (`--file -` or auto-detected TTY).
- Mirroring this on `credential aura-client add` or `credential embed add`.
- Auto-discovery of a `.env` file in the cwd / parent dirs.
- `~` / `$VAR` expansion of the `--file` path.
- An `import` subcommand or batch import of multiple credentials.
- Updating the issue's `cat creds.txt | dbms add` example to literal stdin — issue text is the goal; chosen interface is `--file`.
- Skill bundle `additions.md` content for the new flag — will be addressed in implementation but content not specified here.

## Open Questions

None. All design questions resolved in the source plan.
