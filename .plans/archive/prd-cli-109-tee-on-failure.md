# PRD: Tee-on-failure (CLI-109)

## Overview

When a `neo4j-cli` command fails, persist the command's full emitted output
(stdout + stderr, combined) to a known file under the existing config dir, and
expose that file's path as `tee_path` in the structured error envelope so an
agent can read the captured output without re-running the command. Files are
redacted of secrets, rotated (keep last N per command type), and written
best-effort so tee never changes a command's behaviour or exit code.

This addresses the case where a failing command's output has scrolled away,
was filtered/truncated by the calling agent, or is expensive to reproduce (the
ticket's motivating example is a `query` that times out at 60s).

Linear: [CLI-109](https://linear.app/neo4j/issue/CLI-109/tee-on-failure).
Authoritative design source: the reviewed/approved plan at
`~/.claude/plans/look-into-this-one-frolicking-shell.md`.

## Goals

- On failure, save the command's full emitted stdout+stderr to a known,
  per-OS-stable path so agents/users can read it after the fact.
- Surface the saved path as `tee_path` in the JSON/toon error envelope, and as a
  `Full output saved: <path>` stderr line in all formats.
- Redact secrets (connection-string passwords, password/secret/token/api-key
  assignments, bearer tokens) from what is written.
- Rotate tee files — keep the last N (default 20) **per command type**.
- Be best-effort and config-gated: never crash, never alter exit code; on any
  tee error simply omit `tee_path`.
- Reuse existing infrastructure: config-path resolution, atomic file writes,
  the error-envelope builder, and the `history-*` config-key pattern.

## Non-Goals

- No reconstruction of pre-`--max-rows` / `--truncate-arrays-over` raw result
  sets — we capture the **emitted** (post-truncation) stream only. Pre-filter
  capture is an explicit follow-up.
- No tee on success — only on a non-zero exit / returned error.
- No tee on panic (the `recover` path in `main.go` is out of scope for v1).
- No date/age-based rotation (cap is by file count per command type).
- No new per-OS "state"/XDG-data directory — files live under the existing
  config prefix, consistent with `config.json` / `history.jsonl`.
- No separate stdout/stderr files — the two streams are combined into one log.
- No consolidation of the existing docker `redactString` onto the new text
  redactor (left as-is for now).

## Requirements

### Functional Requirements

- REQ-F-001: Capture the command's emitted output by wrapping the root command's
  out/err streams in `neo4j-cli/main.go` before `cmd.Execute()` —
  `cmd.SetOut(io.MultiWriter(os.Stdout, buf))` and
  `cmd.SetErr(io.MultiWriter(os.Stderr, buf))`, where `buf` is a single shared
  bounded buffer (stdout+stderr interleaved). Pass-through to the real streams is
  unchanged.
- REQ-F-002: The capture buffer is a bounded `LimitedBuffer` capping at ~5 MiB;
  once exceeded it keeps the **head** and appends a
  `\n[output truncated: exceeded N bytes]\n` footer. Only persisted on failure,
  so successful large streams cost only the bounded copy.
- REQ-F-003: Tee files live at
  `filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "tee")` (macOS
  `~/Library/Preferences/neo4j/cli/tee/`, Linux `$XDG_CONFIG_HOME`/`~/.config/neo4j/cli/tee/`,
  Windows `%LOCALAPPDATA%\neo4j\cli\tee\`).
- REQ-F-004: Filename is `<UTC-timestamp>_<command-slug>.log`, timestamp
  formatted `time.Now().UTC().Format("2006-01-02T15-04-05Z")` (filesystem-safe,
  lexicographically sortable), e.g. `2026-05-08T10-23-45Z_query.log`.
- REQ-F-005: The command-slug ("command type") is derived in `main.go` from the
  cobra tree: `matched, _, _ := cmd.Find(os.Args[1:])`, then
  `matched.CommandPath()` with the root binary name stripped and spaces replaced
  by `-` (e.g. `aura instance list` → `aura-instance-list`); fall back to
  `"root"` when no command matches.
- REQ-F-006: Tee writing happens only when `cmd.Execute()` returns a non-nil
  error, **after** the existing `confirm.ErrCancelled` exit-0 early return (a
  user cancellation is not a failure).
- REQ-F-007: Before writing, redact the captured bytes via a new
  `clievents.RedactText(string)` (see REQ-F-012). The write is atomic
  (`fileutils.WriteFileErr`, mode `0600`) using `cfg.Aura.Fs()`.
- REQ-F-008: Rotation — before writing the new file, list the tee dir for entries
  matching `*_<slug>.log`, sort by name (timestamp prefix sorts chronologically),
  and delete oldest until at most `tee-limit - 1` remain, so the new file is the
  freshest of `tee-limit` kept. Rotation is **per command type** (slug).
- REQ-F-009: Tee is governed by global config: `tee-enabled` (bool, default
  `true`) and `tee-limit` (int, default `20`). When `tee-enabled=false` **or**
  `tee-limit<=0`, nothing is written and `tee_path` is empty.
- REQ-F-010: Add `tee_path` to the error envelope — a `TeePath string` field on
  `clierr.CLIError` with a chainable `WithTeePath(string) *CLIError`, and a
  `TeePath string \`json:"tee_path,omitempty"\`` field on `EnvelopeBody`
  projected in `BuildEnvelope()`. Omitted when empty.
- REQ-F-011: `main.go` resolves the returned error to a `*clierr.CLIError`
  (`errors.As`, else `clierr.NewFatalError("%s", err.Error())`), sets the tee
  path via `WithTeePath` when non-empty, and passes that `*CLIError` to
  `clierr.Render`. `Render` additionally prints a `Full output saved: <path>`
  line to stderr (all formats) when `TeePath` is set.
- REQ-F-012: Add `clievents.RedactText(s string) string` as the text-level
  single source of truth for multi-line secret scrubbing, applying conservative
  regexes: (a) URI userinfo in free text
  `(?i)\b([a-z][a-z0-9+.-]*://[^\s:/@]+:)[^\s@]+@` → `${1}***@`; (b) key/value
  assignments `(?i)((?:password|passwd|pwd|secret|token|api[-_]?key|auth)\w*\s*[=:]\s*)(\S+)`
  → `${1}***`; (c) bearer headers `(?i)(bearer\s+)\S+` → `${1}***`.
- REQ-F-013: `tee-enabled` / `tee-limit` are settable/gettable via
  `neo4j-cli config set/get/list` (Global scope), with accessors
  `cfg.Global.TeeEnabled()` and `cfg.Global.TeeLimit()`, validated in `setGlobal`
  (bool / non-negative int) — mirroring `history-enabled` / `history-limit`.

### Non-Functional Requirements

- REQ-NF-001: Tee files are created with mode `0600` (via `fileutils.WriteFileErr`).
- REQ-NF-002: Tee writing must never crash a command nor change its exit code;
  all errors are swallowed and result in an empty `tee_path` (best-effort, same
  posture as `history.Record`).
- REQ-NF-003: Memory is bounded regardless of output size via the
  `LimitedBuffer` cap; tee logic adds no measurable latency to successful runs
  beyond the buffered byte copy.
- REQ-NF-004: Tests use `testfs.GetTestFs(...)` / `afero.MemMapFs` (never
  `afero.NewOsFs()` against the real home dir) for the `tee` package, since
  writes go through `cfg.Aura.Fs()`.
- REQ-NF-005: Cross-platform correctness (macOS/Linux/Windows) for the path,
  `0600` write, and the filesystem-safe timestamp format; committed golden/`.md`
  files keep LF per repo `.gitattributes`.
- REQ-NF-006: `RedactText` is conservative — no false positives on common
  non-secret text (e.g. `limit=10`, prose) while catching the listed secret
  shapes; verified by table-driven tests.

## Technical Considerations

- **Chokepoint**: `neo4j-cli/main.go` (lines ~109–132) is the single place that
  sets out/err, runs `cmd.Execute()`, intercepts `confirm.ErrCancelled`, resolves
  the render format, and calls `clierr.Render`. All tee orchestration (buffer
  wrap, slug, save, error wiring) lives here. The `clierr.Render` plain→fatal
  wrap stays as the fallback for the no-tee path.
- **New package** `common/tee/` (cobra-free, under `common/` so `main.go` can
  import it and so it's unit-testable; mirrors `history/store.go`):
  - `Dir() string`, `Save(cfg, commandSlug string, content []byte) (string, error)`,
    a `LimitedBuffer` `io.Writer`, plus rotation/slug helpers.
  - `tee_test.go` colocated.
- **Reuse**: `common/clicfg/fileutils` (`WriteFileErr`, atomic `0600`);
  `common/clicfg` (`ConfigPrefix`, `Aura.Fs()`); `common/clievents/redact.go`
  (extend with `RedactText`, reusing the existing secret vocab); rotation shape
  modeled on `history.Record` (`store.go:84-88`); `cobra.Command.Find` /
  `CommandPath()` for an accurate command-type slug.
- **Config**: add `tee-enabled` / `tee-limit` to `ValidConfigKeys`,
  `setDefaultValues`, `setGlobal` validation, and Global accessors in
  `common/clicfg/clicfg.go`; they surface automatically via the `config` command.
- **Generated content gate**: changing `ValidConfigKeys` alters the `config`
  command help text embedded in the skill bundle, so run
  `go generate ./neo4j-cli/internal/skill/...` (else `TestGenerator_RoundTrip`
  fails on a `references/config.md` diff). Commit source + regenerated bundle
  together.
- **Changelog**: user-facing → `changie new --projects neo4j-cli --kind Minor --body "..."`.
- **Capture caveat**: only output written through the cobra command's
  out/err writers is captured; code writing directly to `os.Stdout`/`os.Stderr`
  (bypassing `cmd`) would not be teed. Acceptable — the query/output rendering
  path uses `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`.

## Acceptance Criteria

- [ ] A failing command writes a redacted `<timestamp>_<slug>.log` under
      `<ConfigPrefix>/neo4j/cli/tee/` containing its emitted stdout+stderr.
- [ ] `--format json` (and `toon`) failure envelopes include `tee_path`; it is
      omitted when tee is disabled or nothing was captured.
- [ ] In all formats, a failing command prints `Full output saved: <path>` to
      stderr when a tee file was written.
- [ ] Secrets are redacted in the tee file: connection-string passwords
      (`neo4j://user:pw@host` → `neo4j://user:***@host`),
      `password=`/`secret=`/`token=`/`api-key=`/`auth=` values, and
      `Bearer <token>` are `***`; non-secret text (`limit=10`, prose) is intact.
- [ ] Rotation keeps at most `tee-limit` (default 20) files **per command slug**;
      a different command type has its own independent set.
- [ ] `tee-enabled=false` or `tee-limit=0` writes nothing and yields an empty
      `tee_path`.
- [ ] Tee files are mode `0600` and the path resolves correctly on
      macOS/Linux/Windows.
- [ ] A user-cancelled command (`confirm.ErrCancelled`, exit 0) writes no tee file.
- [ ] Tee failure (e.g. unwritable dir) never changes the command's exit code and
      simply omits `tee_path`.
- [ ] `neo4j-cli config set/get/list` handle `tee-enabled` / `tee-limit`;
      invalid values are rejected with a usage error.
- [ ] The capture buffer caps at ~5 MiB and appends the truncation footer when
      exceeded.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check`, and
      `go generate ./neo4j-cli/internal/skill/...` (clean diff) all pass; changelog
      entry included.

## Out of Scope

- Pre-truncation raw result-set capture for `query` (follow-up).
- Tee on success and tee on panic.
- Date/age-based rotation; per-stream (separate stdout/stderr) files.
- Migrating docker `redactString` onto `RedactText`.
- A new XDG/state data directory distinct from the config prefix.

## Open Questions

None — all decisions resolved (capture: emitted/post-truncation; location:
config prefix `tee/`; streams: combined; buffer cap: ~5 MiB head + footer;
defaults: `tee-enabled=true`, `tee-limit=20`; rotation: by-count per command
slug; redaction: new `RedactText`).
