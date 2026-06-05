# PRD: Command History Log (CLI-180)

## Overview

Add a local, append-only command history log to `neo4j-cli`. Every executed command is
recorded — with timestamp, the (redacted) command line, and metadata (CLI version, invoker
human/agent, default workspace, active credential name) — to a JSONL file alongside the
existing config. A new `neo4j-cli history` command lets a user **or agent** read (`list`) and
clear (`clear`) the log. The JSONL format is machine-readable so agents can distill / report on
prior activity; `history list` also renders a human-friendly view.

Linear: [CLI-180](https://linear.app/neo4j/issue/CLI-180/history-logs-we-should-store-that).
Today the repo has no history/logging — only Mixpanel telemetry and debug `slog`.

## Goals

- Persist a chronological log of executed commands for later review by users and agents.
- Capture useful provenance metadata: CLI version, invoker (human vs agent), target workspace,
  and the credential **name** used.
- Make the log both machine-readable (JSONL source of truth) and human-readable (`history list`).
- Be safe by default: redact secret flag values, restrict file permissions, bound file growth.
- Reuse existing infrastructure (config-path resolution, atomic file writes, arg redaction,
  TTY detection, the root command hook, cobra one-file-per-leaf layout).

## Non-Goals

- No exit code, duration, or per-command outcome capture (recording happens pre-execution).
- No remote/centralized logging, no integration with Mixpanel telemetry.
- No date/age-based rotation (cap is by entry count only).
- No masking of the `query` positional Cypher body — it is stored verbatim.
- No interactive TUI / fuzzy search / re-run-from-history UX (read + clear only for v1).
- No new dedicated per-OS "state" directory; the log lives under the existing config prefix.

## Requirements

### Functional Requirements

- REQ-F-001: Record one entry per executed command into a JSONL history file before the command
  runs, by extending the root `PersistentPreRunE` chain in `neo4j-cli/app/app.go`.
- REQ-F-002: The history file lives at
  `filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "history.jsonl")` (macOS
  `~/Library/Preferences/neo4j/cli/`, Linux `$XDG_CONFIG_HOME`/`~/.config/neo4j/cli/`, Windows
  `%LOCALAPPDATA%\neo4j\cli\`).
- REQ-F-003: Each entry is a single-line JSON object with fields: `time` (RFC3339, UTC),
  `command` (string), `invoker` (`"human"` | `"agent"`), `version` (string), and the
  `omitempty` fields `workspace` and `credential` (credential **name** only — never secrets).
- REQ-F-004: `command` = `"neo4j-cli " + clievents.RedactArgs(os.Args[1:])`, reusing
  `common/clievents/redact.go` so `--password`, `--client-secret`, `--api-key`, and
  `--instance-password` values are stored as `***`.
- REQ-F-005: Invoker is `human` when stdin is a TTY (`term.IsTerminal(int(os.Stdin.Fd()))`),
  otherwise `agent`. Detection goes through a package-level test seam (mirror
  `internal/quip/quip.go`). No env override in v1 — TTY heuristic only.
- REQ-F-006: Do **not** record the `history` subtree, `help`, or cobra completion
  (`__complete*`) commands.
- REQ-F-007: On each record, perform read-all → append → trim-to-limit → atomic rewrite via
  `fileutils.WriteFile` (mode `0600`) using `cfg.Aura.Fs()`. Trimming keeps the last
  `history-limit` entries.
- REQ-F-008: Recording is governed by global config: `history-enabled` (bool, default `true`)
  and `history-limit` (int, default `1000`). When `history-enabled=false` **or**
  `history-limit=0`, nothing is written.
- REQ-F-009: Add a `history` top-level command registered in `app.go` next to `config`/`update`.
- REQ-F-010: `history list` reads the JSONL file and, with no flags, prints the **last 20
  entries, newest first**; `--limit N` overrides the count; supports
  `--format json|table|toon` via the standard output flag. Default (table-ish) view renders the
  human form `[time] <command> {invoker:…, …}`.
- REQ-F-011: `history clear` empties the history file and requires a `--force` flag (no
  interactive confirmation prompt); without `--force` it errors with guidance.
- REQ-F-012: When reading, **skip** unparseable/corrupt JSONL lines and render the rest (do not
  error out).
- REQ-F-013: `history-enabled` and `history-limit` are settable/gettable via
  `neo4j-cli config set/get/list` (Global scope), with accessors `cfg.Global.HistoryEnabled()`
  and `cfg.Global.HistoryLimit()`.

### Non-Functional Requirements

- REQ-NF-001: History file is created with mode `0600`.
- REQ-NF-002: Recording must never crash a command nor block meaningfully; failures to write
  history are non-fatal (best-effort, consistent with the warn-and-continue style used elsewhere).
- REQ-NF-003: Tests use `testfs.GetTestFs(...)` (never `afero.NewOsFs()` / real FS) since
  recording goes through `cfg.Aura.Fs()`.
- REQ-NF-004: Cross-platform correctness (macOS/Linux/Windows) for the path and `0600` write;
  committed `.md`/golden files keep LF per repo `.gitattributes`.
- REQ-NF-005: Follow the one-file-per-leaf cobra layout; every leaf has a flush-left `Example:`
  satisfying `TestAllLeafCommands_HaveExamples` (≥2 invocations, comment lines, `--format json`
  on read invocations).

## Technical Considerations

- **Chokepoint**: `neo4j-cli/app/app.go` root `PersistentPreRunE` (≈`:74-83`) is the single
  cross-cutting hook already running version-check / skill-refresh for every command. Append a
  `history.Record(cfg, cmd)` step. Leaf commands do not override `PersistentPreRunE`, so the root
  hook fires for all of them.
- **New package layout** (`neo4j-cli/internal/subcommands/history/`):
  - `history.go` — `NewCmd(cfg)`, registers leaves (≤80 lines).
  - `store.go` — `Record`, `Load`, `Clear`, `path` helpers; imported by `app.go`.
  - `list.go` / `clear.go` — leaf commands.
  - `list_test.go`, `clear_test.go`, `store_test.go`, `helpers_test.go`.
- **Reuse**: `common/clievents/redact.go` (`RedactArgs`); `common/clicfg/fileutils/fileutils.go`
  (atomic `WriteFile`); `common/clicfg` (`ConfigPrefix`, `Aura.Fs()`); `internal/quip/quip.go`
  (TTY seam pattern); metadata accessors `cfg.Version`, `cfg.Aura.DefaultWorkspace()`,
  `cfg.Credentials.Aura.DefaultCredential`.
- **Config**: add `history-enabled` / `history-limit` to GlobalScope in
  `common/clicfg/clicfg.go` with defaults and Global accessors; they surface automatically via
  the `config` command.
- **Generated content gate**: adding a top-level command requires
  `go generate ./neo4j-cli/internal/skill/...` (else `TestGenerator_RoundTrip` fails on a
  `references/*.md` diff). Commit source + regenerated bundle together.
- **Changelog**: user-facing → `changie new --projects neo4j-cli --kind Minor --body "..."`.
- **Docs**: update `README.md` with the new command and a privacy note that commands are logged
  locally (and that `query` Cypher bodies are stored verbatim).
- **Privacy caveat**: secret *flag values* are redacted, but inline literals inside a `query`
  Cypher body are not — documented, by decision.

## Acceptance Criteria

- [ ] Running any non-excluded command appends a JSONL entry with `time`, redacted `command`,
      `invoker`, `version`, and (when set) `workspace` / `credential` name.
- [ ] `--password` / `--client-secret` / `--api-key` / `--instance-password` values appear as
      `***` in the stored `command`.
- [ ] History file is created mode `0600` at the config-prefix path on macOS/Linux/Windows.
- [ ] Invoker is `human` under a TTY and `agent` when stdin is piped/non-interactive (verified
      via the test seam).
- [ ] `history`, `help`, and `__complete*` invocations are not recorded.
- [ ] Exceeding `history-limit` trims the file to the last N entries; `history-limit=0` or
      `history-enabled=false` writes nothing.
- [ ] `neo4j-cli history list` shows the last 20 entries newest-first; `--limit N` and
      `--format json|table|toon` work; a corrupt line is skipped, not fatal.
- [ ] `neo4j-cli history clear` requires `--force`; with it the file is emptied; without it it
      errors with guidance.
- [ ] `neo4j-cli config set history-enabled false` disables subsequent recording;
      `config get`/`config list` show both keys.
- [ ] `make test`, `make fmt-check`, `make lint`, and `go generate ./neo4j-cli/internal/skill/...`
      (clean diff) all pass; changelog entry and README update included.

## Out of Scope

- Exit code / duration / outcome capture; `main.go` Execute() wrapping.
- Date/age-based rotation.
- Masking the `query` Cypher positional body.
- Env-var invoker override (e.g. `NEO4J_CLI_INVOKER`).
- Remote logging or telemetry integration.
- Re-run-from-history / interactive search UX.

## Open Questions

None — all decisions resolved (location: config prefix; cap: by count, default 1000; default:
on/opt-out; clear: `--force`; recording scope: all except history/help/completion; invoker: TTY
heuristic only; list default: last 20 newest-first; corrupt lines: skipped; Cypher body:
verbatim; no exitCode).
