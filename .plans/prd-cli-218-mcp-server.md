# PRD: CLI-218 — `neo4j-cli mcp` (stdio MCP server + Claude Desktop install)

Linear: [CLI-218](https://linear.app/neo4j/issue/CLI-218/mcp-server-that-matches-cli-capabilities-to-run-locally) (team `neo4j-cli-workinggroup`).
Source design: `/Users/oskarhane/.claude/plans/look-into-https-linear-app-neo4j-issue-c-cuddly-sparrow.md`.

## Overview

People using Claude Desktop (or any chat client that cannot run shell commands) have no access to `neo4j-cli`. This feature adds a `neo4j-cli mcp` command group with two halves:

1. **`mcp serve`** — a stdio MCP server exposing the CLI's capabilities through a deliberately tiny, context-frugal tool surface (5 tools, ~1.1k tokens of definitions) built on progressive disclosure rather than one-tool-per-command.
2. **`mcp install` / `mcp bundle`** — Bear-style installation: the CLI *generates* a `.mcpb` desktop extension on the fly, with its own absolute path baked in, so the end user double-clicks a file and never sees an MCP config.

The whole group ships behind the feature flag **`flag.mcp-server`** (default `false`), so it lands on `main` with full test coverage while the tool-surface curation is validated by measurement.

### Why the tool surface is the hard part

The CLI has **178 cobra commands / ~105 runnable leaves**. At the industry cost of 550–1400 tokens per MCP tool definition, a 1:1 mapping would consume **58k–147k tokens before the user types anything**. Measured alternatives from this codebase:

| Artifact | Size | ≈ tokens |
|---|---|---|
| `agent-context --format json` (full envelope) | 283 KB | ~75k |
| `agent-context --format toon` | 255 KB | ~67k |
| Top-level index only (12 trees, `key: short`) | 750 B | ~200 |
| `references/aura.md` (whole file) | 92 KB | ~25k |
| `references/docker.md` section for `docker create` | 5.6 KB | ~1.5k |
| Same leaf as an `agentcontext` JSON node | 7.0 KB | ~1.9k |

Two conclusions drive the design: **`agent-context` can never be a tool result** (75k tokens is worse than the problem), and **the generated markdown reference is smaller *and* richer than the equivalent JSON node**, because JSON pays for escaped newlines and repeated keys while the markdown carries real example invocations.

## Goals

- Expose `neo4j-cli` capability to MCP clients at **~1% of the token cost** of a naive 1:1 tool mapping.
- Make the discovery path converge reliably, addressing the two concerns raised on CLI-218: "if you start adding lots of tools I worry it adds too much context" (Liam) and "totally open would have a high failure rate" (Oskar).
- Give a non-technical Claude Desktop user a one-file, double-click install with **no MCP config exposed** and **no credentials typed**.
- Reuse existing subsystems rather than building parallel ones: the generated skill bundle for docs, `agentcontext` for routing/validation, `clierr` for errors, `clievents` for redaction, `flags.EnforceWriteGate` for write safety, and the `skill.AGENTS` catalog for agent detection.
- Turn the context-budget constraint into a **CI gate**, so it cannot erode.

## Non-Goals

- HTTP / streamable-HTTP transport. **stdio only.**
- A released `.mcpb` artifact, a Node launcher, binary bootstrapping, or any new distribution channel or CI publish job. The `.mcpb` is generated at runtime by the CLI.
- Install support for Cursor, VS Code, or Codex/ChatGPT-desktop in this PRD (the shared-catalog groundwork lands here; the additional config writers follow).
- Dedicated Cypher/schema tools (`get-schema`-style). Reachable via `neo4j_cli_run` at zero marginal definition cost.
- Coordination with `github.com/neo4j/mcp` — a separate project with a different goal (exposing Neo4j itself).
- Claude Cowork. Its VM is a NAT'd Linux guest (`vmIP` 172.16.10.3), so `localhost:7687` does not reach the user's database, a macOS binary is not executable in the guest, and anthropics/claude-code#42453 (local MCP tools disabled in Cowork) is closed as *not planned*.
- A skill `.zip` release asset for Claude Desktop's Skills upload.
- Making the flag default-`true` or removing it. Per `.agents/feature-flags.md`, GA deletes the flag and its gated branch in one PR — a separate change.

## Requirements

### Functional Requirements

#### Command group

- **REQ-F-001**: Add `neo4j-cli mcp` under `neo4j-cli/internal/subcommands/mcp/`, following the one-file-per-leaf layout: parent `mcp.go` (≤80 lines, `NewCmd(...)` + `AddCommand` per leaf) and one file per leaf.
- **REQ-F-002**: Leaves: `serve`, `tools`, `install`, `remove`, `list`, `check`, `bundle`. Every runnable leaf has a colocated `<action>_test.go`; shared helpers in `mcp_helpers_test.go`.
- **REQ-F-003**: The entire group is gated on `flag.mcp-server` (default `false`). When disabled the group is not registered on the root command, so `neo4j-cli --help`, `agent-context`, and the generated skill bundle are unchanged. **`flag.mcp-server` is the single gate for all MCP surface** — when it is on, every leaf including `serve` is listed normally in `mcp --help`. No leaf carries an additional `Hidden: true`.
- **REQ-F-004**: `flag.mcp-server` is added to the `clicfg.Registry` map in `common/clicfg/flags.go` with `Default: false`, `Owner`, `Gates`, `IntroducedIn`, `RemovalCondition`. It is the **first** production entry in that map (currently `map[string]Flag{}`).
- **REQ-F-005**: Overrides come free from the existing machinery — `NEO4J_CLI_FLAG_MCP_SERVER=1` (via `FlagNameToEnv`, bound per Registry entry in `clicfg.go:228`) and `neo4j-cli config set flag.mcp-server true`. `config get` and `config set` already resolve Registry keys (`clicfg.go:618` `ResolveConfigKey` → `config/get.go:67`, `config/set.go` `FlagScope`), so no new plumbing. `config list` deliberately does **not** enumerate flag keys — `clicfg.Config.Printable` walks only `Global`/`Aura` `ValidConfigKeys`, and a pre-existing test asserts the absence (`config/get_test.go:107`). Nor does tab-completion advertise them (`validGetArgs`). Both are correct per `.agents/feature-flags.md` ("unknown/removed keys are silent at runtime"): experimental flags stay unadvertised. Do not add plumbing to change this.

#### Tool surface

- **REQ-F-006**: `mcp serve` registers exactly **five** tools. The connector is named **"Neo4j CLI"** (not "Neo4j"), and every tool name is prefixed `neo4j_cli_`:

  | Tool | Annotations | Params |
  |---|---|---|
  | `neo4j_cli_list_targets` | `readOnlyHint`, `idempotentHint` | — |
  | `neo4j_cli_list_commands` | `readOnlyHint`, `idempotentHint` | `tree?` |
  | `neo4j_cli_read_docs` | `readOnlyHint`, `idempotentHint` | `command`, `offset?`, `max_chars?` |
  | `neo4j_cli_run` | `readOnlyHint` | `command`, `args?` |
  | `neo4j_cli_run_write` | `destructiveHint`, `openWorldHint` | `command`, `args?` |

- **REQ-F-007**: The server sets the MCP `instructions` field (sent once at `initialize`, inside the cached prompt prefix) carrying the 12-tree top-level index (measured 750 B) plus orientation rules: run `query :schema` before writing Cypher, prefer `neo4j_cli_read_docs` over guessing flags, and never pass `neo4j_cli_run_write` without user confirmation. Content that would otherwise inflate per-request tool descriptions goes here.
- **REQ-F-008**: `neo4j_cli_list_targets` fans out over `docker list`, `desktop dbms list`, `credential dbms list` and `aura instance list`, returning one table of reachable Neo4j targets. Failures of individual sources degrade to a noted omission rather than failing the tool.
- **REQ-F-009**: `neo4j_cli_list_commands` with no argument returns the 12 top-level trees; with `tree` returns every command in that tree as `use: short` lines. It is a projection of `agentcontext.BuildContext(root, version)` — no new data source. The `tree` enum is generated from the live tree at server start so a new top-level tree auto-surfaces.
- **REQ-F-010**: `neo4j_cli_read_docs` resolves `command` (a space-separated CLI path, e.g. `docker load`) against the embedded skill bundle (`neo4j-cli/internal/skill/bundle/references/<tree>.md`, exposed as `skill.Bundle fs.FS`) by matching the heading whose text is the full command path, returning until the next heading of level ≤ the matched level. This works because `common/skill/render/render.go:walkRender` already emits `## neo4j-cli docker load`-style headings.
- **REQ-F-011**: A tree name alone (`aura`) returns that file's `## Contents` TOC plus the tree's own prose, never its children — `renderReference` already prepends that TOC for files >100 lines, giving a ~3 KB menu for a 92 KB file.
- **REQ-F-012**: `neo4j_cli_read_docs` truncates at `max_chars` (default 6000, max 20000), ends truncated results with a `… truncated: N chars remain; call again with offset=<x>` line, and carries `next_offset` in `structuredContent`.
- **REQ-F-013**: `neo4j_cli_run` and `neo4j_cli_run_write` take `command` (validated CLI path) and `args` (remaining positionals/flags as separate array elements) as **separate** parameters, so `args` can never introduce a subcommand token and `command` can be validated before execution. `args` is capped at 64 items.
- **REQ-F-014**: `neo4j_cli_run` accepts only commands classified non-write. A write-annotated command returns a `usage_error` naming `neo4j_cli_run_write`. `neo4j_cli_run_write` accepts only write-classified commands and requires the write gate (REQ-F-020).
- **REQ-F-015**: A literal `--rw` in `args` is rejected with a `usage_error` pointing at `neo4j_cli_run_write`, so the policy layer cannot be bypassed. `--debug` in `args` is likewise rejected (its traces write to package-global `debugW` seams whose only setters are test-named).
- **REQ-F-016**: Flag long-names in `args` are validated against the resolved leaf's `agentcontext.Flag` list before execution, returning `usage_error` with a did-you-mean suggestion — cheaper than an exec and it teaches the model.
- **REQ-F-017**: `mcp tools` prints the registered tool manifest (`--format json|table|toon`) without starting a transport, so the surface is inspectable and gate-testable.

#### Execution model

- **REQ-F-018**: Commands execute **in-process** against a freshly built cobra tree per call, not by re-executing the binary. Rationale: the repo's rule is "secrets via process env, NEVER argv (world-readable via `/proc`)", and a model-supplied `--password` in `args` would land in a child's `/proc/<pid>/cmdline`. In-process also preserves `clicfg`, `clierr`, `confirm`, history, tee, telemetry and `flags.EnforceWriteGate` unchanged.
- **REQ-F-019**: Because `neo4j-cli/app` must import `mcp` to mount it, `mcp` cannot import `app`. A `func(*clicfg.Config) *cobra.Command` root factory is injected from `app.go` — the same injection pattern `agentcontext` uses for `version`.
- **REQ-F-020**: Write gating is three independent layers: (a) `mcp serve` without `--rw` rejects every `neo4j_cli_run_write` call before building a tree; (b) the policy table classifies the path; (c) the CLI's own `flags.EnforceWriteGate` fires inside `Execute()` off `cmd.Annotations["write"]`. Layer (c) is authoritative and must not be bypassed or reimplemented — a bug in the MCP layer must not be able to produce an unflagged write.
- **REQ-F-021**: `--format toon` is injected into executed commands when `args` contains no `--format`; `mcp serve --format json` changes that default. `format` is deliberately **not** a tool parameter. Do not rely on `output.ResolveOutput`'s agent-harness branch, which reads the *server process's* env.
- **REQ-F-022**: Each call builds a fresh `clicfg.NewConfig` and fresh root (cheap; avoids `viper` / `desktopclient.debugEnabled` / `clievents` global bleed), points `SetOut`/`SetErr` at bounded buffers, is serialised behind a mutex, and is wrapped in `recover()` producing an error the way `main.go:recoverPanic` does.

#### stdio safety

- **REQ-F-023**: After the transport captures stdout, the process `os.Stdout` is redirected to `os.Stderr` and `os.Stdin` to `os.DevNull`, so no stray direct read or write can corrupt or consume the JSON-RPC frame.
- **REQ-F-024**: Each per-call root gets `cmd.SetIn(bytes.NewReader(nil))` so `common/confirm` reads EOF and cleanly cancels rather than blocking.
- **REQ-F-025**: `update` and the interactive password-prompt leaves are excluded from the allowlist. This is not theoretical: `neo4j-cli/query/input.go:readPositionalOrStdin` falls through to `io.ReadAll(stdinReader())` (→ `os.Stdin`, `query/run.go:23`) when stdin is not a TTY and no positional was given, so `neo4j_cli_run("query")` with empty `args` would consume the protocol stream and hang the server. Same class: `dbconn/helpers.go:26` and the desktop create leaves call `term.ReadPassword(os.Stdin.Fd())`; `update/swap.go:296` sets `cmd.Stdin = os.Stdin`.

#### Policy / allowlist

- **REQ-F-026**: `mcp/allow.go` classifies every command path. Default-deny for anything unclassified.

  | Policy | Paths |
  |---|---|
  | `deny` | `update`, `mcp`, `completion`, `history clear`, `config set credential-storage` — swaps the running binary / recursive servers / silently downgrades keyring→plaintext |
  | `gated:allow-aura` | `aura instance create/resize/destroy/overwrite`, `aura agent *`, `customer-managed-key *` — **costs real money** |
  | `gated:allow-credential-write` | `credential * add/remove/use/set-embed` — mints and stores keys in the OS keyring |
  | `write` (→ `neo4j_cli_run_write` only) | everything carrying `Annotations["write"]="true"` (~83 leaves) |
  | `allow` | `dataset`, `docker`, `desktop`, `query`, `admin`, `config get/list`, `credential */list|get`, `skill`, `history list`, `agent-context`, all `aura … list/get` |

- **REQ-F-027**: The `gated:*` policies are opt-in via `mcp serve` flags (`--allow-aura`, `--allow-credential-write`) and the corresponding generated-manifest `user_config` booleans, both default `false`.

#### Errors and redaction

- **REQ-F-028**: Failures return `CallToolResult{isError: true}` — a *tool* error, not a JSON-RPC error (those are reserved for unknown tool / malformed params). Text content is `Error: <message> (exit N)` then `Suggestion: …` when present, then the redacted output tail.
- **REQ-F-029**: `structuredContent` carries `ce.BuildEnvelope()` verbatim (`code`, `exit_code`, `message`, `resource_type`, `resource_id`, `suggestion`, `tee_path`, `retryable`) plus `stdout`, `stderr` and redacted `argv`. This reuses an already-documented public contract — no new schema to version. Mirror `main.go`'s `errors.As(err, &ce)` with a `clierr.NewFatalError` fallback; never call `clierr.Render` (it writes to streams).
- **REQ-F-030**: `retryable` (true only for exit 7/8) is named in the tool descriptions so the model retries rate limits and stops retrying usage errors.
- **REQ-F-031**: **Every** tool result and error string passes through `clievents.RedactText` then `output.StripControl` before being returned. Under MCP these strings are copied into the model's context and uploaded, so the redaction obligation extends beyond today's telemetry/panic paths. Note the documented limitation that `RedactText` is shape-based and misses table cells — secret-minting leaves must have called `clievents.RegisterSecretValue` first (as `docker/create_core.go` and `instance/create_core.go` do).
- **REQ-F-032**: Returned output is truncated at a `--max-output-chars` bound (default ~8000) with `tee_path` surfaced so the remainder stays recoverable.

#### Install (Claude Desktop)

- **REQ-F-033**: Extend `skill.Agent` in `common/skill/agents.go` with two optional fields — `MCPConfig string` and `MCPFormat Format` — and add a `claude-desktop` entry. Capability is expressed by presence: `SkillsDir == ""` means "not a skill target", `MCPConfig == ""` means "not an MCP target". Reuse `--agent`, `DetectAgents`, `FindAgent`, `expandPath`, the `afero.Fs` seam, and the existing filter error semantics (`""` → all detected, unknown → `ErrUnknownAgent`, known-but-undetected → `ErrAgentNotDetected`). Do **not** create a parallel `mcphost`/`mcpclient` catalog.
- **REQ-F-034**: `agentNames()` filters on `SkillsDir != ""` and a new sibling filters on `MCPConfig != ""`. This is load-bearing: `agentNames()` is interpolated into `skill install`/`skill remove` `Long` text, which is rendered into the **committed** skill bundle — an unfiltered `claude-desktop` entry would both advertise skill support the app lacks and drift the bundle.
- **REQ-F-035**: `skill.Install`, `Remove`, `List` and `BuildInventory` apply the same `SkillsDir != ""` filter, so `skill list` gains no bogus `claude-desktop` row and existing row-count expectations hold.
- **REQ-F-036**: `expandPath` (or per-`GOOS` overrides on `Agent`) gains platform-specific path support for Claude Desktop — `~/Library/Application Support/Claude` on darwin, `%APPDATA%\Claude` on win32. Follow `common/clicfg/darwin.go`'s precedent and read the `$HOME` env var rather than `user.Current()`, so subprocess tests stay isolated.
- **REQ-F-037**: `mcp install` for `claude-desktop` **generates a `.mcpb` on the fly** — `manifest.json` + `icon.png` + `README.md`, zipped, no binary inside — writes it under the user cache dir, and opens it so Claude Desktop shows its own install UI. This is the verified Bear pattern (`~/Library/Application Support/Claude/Claude Extensions/local.mcpb.shiny-frog-ltd..bear/` contains only `manifest.json`, two icons and `README.md`, with `command: "${user_config.bearcli_path}"` pointing at an out-of-bundle CLI whose default is "the copy bundled inside the Bear.app **that generated this connector**").
- **REQ-F-038**: The generated manifest uses `manifest_version: "0.3"`, `name: "neo4j-cli"`, `display_name: "Neo4j CLI"`, `server.type: "binary"`, `entry_point` and `mcp_config.command` resolving to **this** binary's absolute path (`os.Executable()`), `args: ["mcp", "serve"]`, `tools_generated: false` with an explicit `tools[]` array carrying `title` plus `readOnlyHint`/`destructiveHint` per tool, `compatibility.platforms`, and `privacy_policies`.
- **REQ-F-038a**: The `icon` field is **omitted in v1**. Icons are optional per the MCPB docs, the repo currently ships no image assets of any kind, and third-party logo mirrors are not an acceptable provenance for a committed brand asset. Sourcing an official 512×512 Neo4j mark (via neo4j.com brand assets or design) is follow-up work; the manifest generator must therefore treat `icon` as optional rather than hard-coding a path.
- **REQ-F-039**: `user_config` carries a **selector, not secrets** — the sidecar `Claude Extensions Settings/<id>.json` is plaintext (verified). Fields: `neo4j_cli_path` (file, defaulted to this binary), `allow_writes` (boolean, **default false**), `neo4j_credential` (name of a stored dbms credential; empty = auto), `allowed_databases` (comma-separated allowlist), `result_row_limit` (number, default 500). **No `NEO4J_PASSWORD`.**
- **REQ-F-040**: `mcp bundle --out <path>` writes the same generated `.mcpb` to a path instead of opening it, for handing to a colleague who has no terminal.
- **REQ-F-041**: `mcp install` also supports writing `mcpServers."neo4j-cli"` into `~/Library/Application Support/Claude/claude_desktop_config.json` as an alternative to the `.mcpb` path. Any config write is a **surgical merge, never a rewrite**: read → `map[string]any` → set only that one key → atomic temp-file + rename preserving mode. That file holds `globalShortcut`, `coworkUserFilesPath` and a large `preferences` tree that must survive untouched.
- **REQ-F-042**: The server key is `neo4j-cli` everywhere, so it never collides with an existing `neo4j`-prefixed entry (this machine's config already holds a `neo4j-data-modeling` server — these configs are shared, populated space).
- **REQ-F-043**: `mcp list` shows one row per MCP-capable agent (Detected / Installed / InstalledVersion, the `AgentInstall` shape); `mcp remove` is idempotent and tolerant like `skill remove`; `mcp check` detects drift mirroring `common/skill/check.go`.

#### Credentials

- **REQ-F-044**: The happy path requires no typed credentials. `mcp serve` resolves connections through the existing `dbconn.ResolveConn` precedence (`--credential` selector → stored default dbms credential → `.env` walk-up → OS env → `neo4j://localhost:7687`). `initCredentialStorageDefault` (`app.go:82`) already sets `credential-storage: keyring` silently on new installs, so secrets live in Keychain / Credential Manager / libsecret.
- **REQ-F-045**: The "I have no database" flow works end to end: `neo4j_cli_list_targets` → `neo4j_cli_run_write("docker create", ["--name","claude","--wait"])`, which already generates a password, stores a dbms credential, and can suppress the password from output. `--no-print-password` is the default under MCP.
- **REQ-F-046**: When the credential store is unreadable (locked keyring, no D-Bus session), `mcp serve` returns an explicit actionable error rather than hanging or prompting.

### Non-Functional Requirements

- **REQ-NF-001**: Serialized tool definitions stay under a byte ceiling enforced by a golden test on `mcp tools --format json` — **4000 B (~1.1k tokens)**, ~1.8× headroom over the design's ~1.1k. Any future tool addition must argue past this gate.
- **REQ-NF-002**: The "load a dataset into a local Neo4j" task converges in **≤4 round trips and ≤2k tokens** from a cold session, and in 1 round trip when the command is already known.
- **REQ-NF-003**: SDK is `github.com/modelcontextprotocol/go-sdk` pinned to **v1.4.1 stable** — not v1.7.0-pre.1. This binary is signed, notarized and shipped through five channels; the design needs no 2026-07-28 spec feature and never emits `notifications/tools/list_changed` (whose own spec caveat is prompt-cache invalidation). Verify the transitive set stays pure Go before committing, since GoReleaser cross-builds linux/windows/darwin × amd64/arm64. **Verified 2026-08-04**: pure Go — adds `google/jsonschema-go` v0.4.2, `segmentio/encoding` v0.5.4, `segmentio/asm` v1.1.3 (Go SIMD assembly with pure-Go fallbacks, *not* cgo) and `yosida95/uritemplate/v3` v3.0.2; zero `CgoFiles` in the `./neo4j-cli` dep graph and `CGO_ENABLED=0 go build ./...` green on all six release targets. The SDK also forces six `golang.org/x` MVS upgrades (`mod`, `net`, `sync`, `term`, `text`, `tools` — the last to v0.41.0), all gates green with them.

  The pin **cannot land as a standalone commit** ahead of its first importer, correcting the task-003/task-004 split: `.goreleaser.yaml` runs `go mod tidy` in `before.hooks`, and tidy strips an unimported require line from both `go.mod` and `go.sum`. Worse, `go get` alone records only the go-sdk hashes in `go.sum` and none of the four transitive deps', so an unimported pin cannot even build the SDK. The `go.mod`/`go.sum` change therefore ships in the same commit as the first importing package (task-004). A blank-import placeholder was considered and rejected as a contrived import.
- **REQ-NF-004**: `make test`, `make fmt-check` and `make lint` all pass. Every new `.go` file carries the Neo4j copyright header (`make license-check`).
- **REQ-NF-005**: `go generate ./neo4j-cli/internal/skill/...` is run in the same commit as any cobra-tree change, so `TestGenerator_RoundTrip` and `make generate-check` stay green. With the flag default-`false` the tree is unchanged, so the bundle should **not** drift — a drift here is a signal the gating is wrong.
- **REQ-NF-006**: Every runnable leaf has a flush-left `Example:` with ≥2 `#`-commented invocations, `neo4j-cli` prefix, `--rw` on writes and ≥1 `--format json` on reads (`TestAllLeafCommands_HaveExamples`). `serve` must **not** be annotated `write:"true"` to sidestep the write gate — `EnforceWriteGate` would then demand `--rw` merely to *start* the server, since stdout is never a TTY under MCP, destroying the read-only default.
- **REQ-NF-007**: Casing: MCP tool names and JSON schema properties are snake_case, all prefixed `neo4j_cli_` (OUTPUT rule); `mcp` leaf names, aliases and flag long-names are kebab-case (INPUT rule); CLI tokens inside `command`/`args` values stay kebab-case. Both existing gates (`agentcontext/casing_input_gate_test.go`, `common/output/casing_gate_test.go`) continue to pass.
- **REQ-NF-008**: Per `.agents/feature-flags.md`, the flag ships with tests for **both** states while it lives, and CI exercises the flag-on path via `NEO4J_CLI_FLAG_MCP_SERVER=1`.
- **REQ-NF-009**: Tests never use `afero.NewOsFs()` in paths touching query or credentials (the dev machine has real credentials at `~/Library/Preferences/neo4j/cli/credentials.json`); use `testfs.GetTestFs(...)`. Seed `output.IsAgent` via `TestMain` where format resolution matters.
- **REQ-NF-010**: Handler tests use the SDK's `NewInMemoryTransports()` — no subprocess, no socket, no port binding.
- **REQ-NF-011**: A `changie` entry is added for the user-facing surface (`changie new --projects neo4j-cli --kind Minor --body …`), describing only the observable effect — the new `mcp` commands behind `flag.mcp-server` — not the internals.
- **REQ-NF-012**: No new distribution channel, CI publish job, GoReleaser artifact, npm/PyPI change, or website change.

## Technical Considerations

**Reuse map.** Nothing here invents a subsystem that already exists:

| Need | Existing thing |
|---|---|
| Command catalog, path resolution, flag validation, exit/error legend | `agentcontext.BuildContext` (pure, no I/O) — used *inside* the server, never as output |
| Command documentation | The committed, `//go:embed`ed skill bundle, sliced by its full-command-path headings |
| Error envelopes | `clierr.CLIError` + `BuildEnvelope()` + `clierr.Codes` |
| Secret scrubbing | `clievents.RedactText` / `RedactArgs` / `RegisterSecretValue` |
| Write safety | `flags.EnforceWriteGate` + `cmd.Annotations["write"]` (~83 leaves) |
| Agent detection + install/remove/list semantics | `common/skill/agents.go` + `installer.go` |
| Connection resolution | `dbconn.ResolveConn` |
| Output formats | `common/output.ResolveOutput`, `toon` |
| Feature gating | `clicfg.Registry` + `FlagSet.Enabled` + `FlagNameToEnv` |

**Known drift hazard.** `agentcontext` includes `completion` (it filters on `IsAvailableCommand()`, and cobra injects `completion` at `Execute()` time) while the bundle does not (`gen/main.go` runs pre-`Execute`). The catalog and the docs resolver therefore disagree by exactly one entry, which must be filtered explicitly and covered by a test.

**Shared-catalog risk.** Extending `skill.Agent` touches a subsystem with 11 live entries, generated help text and existing row-count expectations. Do the filtering refactor (REQ-F-034/035) as its **own commit** ahead of the MCP work so any skill-side regression is isolated and bisectable.

**Why not the alternatives** (recorded so they are not re-litigated):

| Rejected | Why |
|---|---|
| One tool per command (105 leaves) | 58k–147k tokens of definitions |
| Bare `run_neo4j_cli(args[])` (~80 tok) | No discovery path, no pre-exec validation, no policy boundary; `neo4j_cli_run` is this plus a validated `command`/`args` split for ~+500 tok across the surface |
| Returning the `agent-context` envelope as a tool result | 75k tokens measured |
| A hand-written MCP doc corpus | Guaranteed drift against a golden-tested generated artifact |
| Dynamic `tools/list_changed` growth | Invalidates prompt caching (the spec's own caveat) for no gain over progressive disclosure |
| Re-exec the binary per call | Puts model-supplied secrets on a child argv, loses typed `*clierr.CLIError`, forces stdout re-parsing; its only edge (crash isolation) is bought far cheaper by `recover()` |
| A single `run` tool instead of the read/write split | Annotations are static, so it must be `destructiveHint: true` unconditionally, training users to click through confirmations on read-only calls |
| Placing the server in `common/mcp/` | Forbidden: `common/*` cannot import `neo4j-cli/internal/*`, and the server needs `internal/skill` + `internal/subcommands/agentcontext` |
| Bundling binaries in the `.mcpb` | 16.4 MB raw / 9.0 MB gzip per platform → 45–72 MB for a chat extension; also breaks `neo4j-cli update` and inherits macOS quarantine |

## Acceptance Criteria

- [ ] `flag.mcp-server` exists in `clicfg.Registry` with `Default: false`; `neo4j-cli config set flag.mcp-server true` and `NEO4J_CLI_FLAG_MCP_SERVER=1` both enable the group.
- [ ] With the flag off: `neo4j-cli --help` shows no `mcp` group, `agent-context` output is unchanged, and `make generate-check` reports no bundle drift.
- [ ] With the flag on: `neo4j-cli mcp tools --format json` lists exactly the five `neo4j_cli_*` tools with correct annotations.
- [ ] Golden test fails if serialized tool definitions exceed 4000 B.
- [ ] Gate test walks the live cobra tree and fails on any command path not classified by the policy table.
- [ ] Gate test asserts every registered tool name matches `^neo4j_cli_[a-z0-9_]+$` and every schema property `^[a-z][a-z0-9_]*$`.
- [ ] `neo4j_cli_read_docs("docker load")` returns that leaf's prose, flag table and examples; `neo4j_cli_read_docs("aura")` returns the TOC only; neither exceeds `max_chars`; truncated results carry a working `offset` continuation.
- [ ] Every entry `neo4j_cli_list_commands` can emit resolves to a reference file, with `completion` explicitly filtered.
- [ ] `neo4j_cli_run("query")` with empty `args` returns a usage error and does **not** hang the server.
- [ ] `neo4j_cli_run` refuses a write-annotated command and names `neo4j_cli_run_write`; a literal `--rw` or `--debug` in `args` is rejected.
- [ ] `neo4j_cli_run_write` fails when `mcp serve` was started without `--rw`, and succeeds when it was.
- [ ] `aura instance create` and `credential dbms add` are refused unless their respective `--allow-*` flag is set.
- [ ] Redaction test: `docker create` driven through the MCP layer against the fake docker client produces a `CallToolResult` that does not contain the generated password.
- [ ] A failing command returns `isError: true` with `code`, `exit_code`, `message`, `suggestion`, `retryable` and `tee_path` in `structuredContent`.
- [ ] `mcp bundle --out X.mcpb` produces a valid zip containing `manifest.json` (no binary) whose `mcp_config.command` is this binary's absolute path; `npx @anthropic-ai/mcpb validate` accepts the manifest.
- [ ] `mcp install --agent claude-desktop` installs into Claude Desktop and the connector loads and lists its tools.
- [ ] Golden test: merging into a realistic `claude_desktop_config.json` preserves `preferences`, `globalShortcut` and a pre-existing `mcpServers` entry.
- [ ] `skill list` returns exactly the skill-capable agents (no `claude-desktop` row), and `agentNames()` output is byte-identical to before, so the committed bundle's "Supported agents:" line is unchanged.
- [ ] `mcp list` / `mcp check` / `mcp remove` behave per the `skill` analogues, including the unknown-agent and undetected-agent error semantics.
- [ ] End to end in Claude Desktop: "find my local Neo4j and load the movies dataset" completes in one chat turn, in ≤4 tool round trips.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check` all pass; CI exercises the flag-on path.
- [ ] A `changie` entry exists describing the observable surface.

## Out of Scope

- HTTP / streamable-HTTP transport, and anything needing an inbound listener or tunnel.
- A released `.mcpb` artifact, Node launcher, binary bootstrap/download, `publish-mcpb.yml`, `distribution/mcpb/`, or website/neo4j.sh changes.
- Install support for Cursor (`~/.cursor/mcp.json`), VS Code (`~/Library/Application Support/Code/User/mcp.json`), and Codex/ChatGPT-desktop (`~/.codex/config.toml`, text-block edit, no TOML dependency), plus `mcp link` deep links for VS Code and Cursor. The catalog groundwork lands here; the writers do not.
- Dedicated `neo4j_cli_cypher` / `neo4j_cli_schema` tools and a `--tools cypher,schema` opt-in.
- MCP `resources` (exposing `references/*.md` as URIs) and MCP `prompts` (`load-example-dataset`, `start-local-neo4j`).
- `elicitation/create` confirmation prompts on the write path.
- Serving the remote `neo4j-contrib/neo4j-skills` catalog through `neo4j_cli_read_docs`.
- A skill `.zip` release asset or a `neo4j-contrib/neo4j-skills` catalog entry.
- Claude Cowork support.
- Connectors Directory submission.
- Aura browser-based auth for chat users.
- Flipping `flag.mcp-server` to default-`true` or removing it.
- Any coordination with, or change to, `github.com/neo4j/mcp`.

## Resolved Questions

These were open in the design doc and are now settled. Recorded so they are not reopened during implementation.

1. **Can a `.mcpb`'s `mcp_config.command` point outside `${__dirname}`? → Yes.** Two independent confirmations. The MCPB manifest spec describes `server.type` as informational — the actual launch comes from `mcp_config` — and states no prohibition on absolute paths and no security constraint on executing files outside the bundle. And Bear ships exactly this pattern in production: its installed connector contains no binary, only `manifest.json` + icons + `README.md`, with `command: "${user_config.bearcli_path}"` defaulting to `/Applications/Bear.app/Contents/MacOS/bearcli`. REQ-F-037/038 stand. Task 1 still installs a generated bundle end to end as the empirical check, but this is no longer a design risk.
2. **Does a headless spawn hit a Keychain prompt on macOS? → No.** Measured directly: an item created the way `go-keyring` creates them (`security add-generic-password -U`, no `-T`) was read back via `security find-generic-password` under three conditions — inherited env, scrubbed env (`HOME` + `PATH` only, mimicking a GUI-app spawn), and no `HOME` at all — each returning exit 0 with no prompt and no hang. This matches the ACL model: the item's ACL identity is `/usr/bin/security` in both directions, so TTY and environment are irrelevant. Only the **Linux/libsecret** leg remains theoretical (needs `DBUS_SESSION_BUS_ADDRESS` and an unlocked keyring, and the existing first-run fallback only fires when the key is *absent*, so an established keyring-mode install would hard-fail rather than degrade). REQ-F-046's actionable-error requirement stands and is the mitigation.
3. **Tool-surface curation is owned by the issue owner**, to be decided from measurement rather than up front. REQ-NF-002 is the bar; if the five-tool surface misses it the response is a curation change, not an architecture change, and whether `neo4j_cli_list_targets` earns its ~200 tokens is the first thing to re-measure.
4. **`mcp serve` visibility → one flag, no extra hiding.** `flag.mcp-server` gates the entire group; when on, all leaves are listed normally. See REQ-F-003.
5. **Icon → omitted in v1.** See REQ-F-038a; the generator treats `icon` as optional and an official Neo4j mark is follow-up work.

## Open Questions

1. **Linux/libsecret behaviour in a headless spawn** — the one remaining unknown from (2) above. Not on the critical path for macOS-first delivery, and REQ-F-046 bounds the failure mode to an actionable error.
