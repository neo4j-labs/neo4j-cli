# PRD: Control Neo4j Docker Instances

Linear: [CLI-123](https://linear.app/neo4j/issue/CLI-123/control-neo4j-docker-instances)
Source plan: `/Users/oskarhane/.claude/plans/i-d-like-to-look-dapper-valley.md`
Suggested branch: `oskar/cli-123-control-neo4j-docker-instances`

## Overview

`neo4j-cli` today manages cloud Neo4j (Aura) and connects to any reachable Neo4j via `query`, but it cannot spin up a local Neo4j. Users who want to evaluate, prototype, or run integration tests fall back to raw `docker run neo4j ...` with a stack of `-e`/`-p` flags.

CLI-123 adds a new top-level `docker` command tree that puts the local-Neo4j lifecycle (create / list / get / start / stop / delete) behind first-class commands, mirrors the Aura ergonomics where it makes sense (`--name`, `--wait`, `--rw`, `--format`), and drops the resulting dbms credential straight into `neo4j-cli query`. An `--ephemeral` mode runs the container with `docker run --rm`, persists nothing, and emits a `.env` file consumable by the existing `query --env <path>` flag.

The tree is implemented by shelling out to the `docker` CLI via `os/exec` — no Docker Go SDK dependency. Docker itself is the source of truth for managed state; we tag containers with `org.neo4j.cli.managed=true` plus a few metadata labels and discover them via `docker ps --filter label=...`. No new state file under `cli/`.

## Goals

- Add `neo4j-cli docker {create,list,get,start,stop,delete}` covering the full local-Neo4j lifecycle.
- Make `docker create` produce a usable Neo4j in one command: pulled image, running container, port-mapped Bolt/HTTP, generated password, stored dbms credential (or emitted `.env`), with optional `--wait` for Bolt readiness.
- Default to Neo4j Enterprise under evaluation license (`NEO4J_ACCEPT_LICENSE_AGREEMENT=eval`), with `--accept-license` to upgrade to full acceptance (`=yes`).
- Pre-flight port-conflict detection on host (7474, 7687) with a clear error pointing at `--bolt-port` / `--http-port`.
- Auto-suffix name collisions (`<name>-1`, `<name>-2`, …) against both Docker container names and existing dbms credentials; echo the chosen name.
- `--ephemeral` mode that runs `docker run --rm`, skips credential persistence, and emits a `.env` file (stdout or `--env-out-file <path>`) consumable by `neo4j-cli query --env <path>`.
- Reuse existing infrastructure: `flags.RegisterOutputFlag`, `flags.RegisterRwFlag`, `flags.RegisterWait`, `credentials.DbmsCredentials.Add/Remove`, the `clicfg` filesystem abstraction, and the neo4j-go-driver vendored by `query/` for Bolt readiness.
- Update all documentation surfaces (README.md, AGENTS.md, neo4j-cli skill description/additions, regenerated skill bundle, CONTRIBUTING.md, changelog entry).
- Ship a non-trivial test suite with a swappable docker client fake (no live docker required for unit tests).

## Non-Goals

- **No Kubernetes, Compose, Podman, or remote-Docker support.** Single local Docker daemon only.
- **No native (non-Docker) local Neo4j runner.** Future-flex via the `docker` name choice; not in v1.
- **No `docker logs` leaf in v1.** Deferred. Users fall back to `docker logs <name>` directly.
- **No `--wait-timeout` flag.** `--wait` uses a fixed 60-second deadline for v1.
- **No `--keep-credential` flag on `delete`.** Deleting the container always removes the linked dbms credential.
- **No memory / heap / cache size flags.** The issue explicitly excludes these.
- **No tenant / project / cloud-provider / region flags.** The issue explicitly excludes these (they're Aura concepts).
- **No persistent data-volume management.** Containers use the default volume strategy; advanced volume control is out of scope for v1.
- **No website (`gh-pages`) edits in this PR.** The site is prompt-driven and rolled out separately; flag in PR description for the next pass.
- **No changes to the aura standalone binary's command tree.** Docker is a neo4j-cli tree only; the aura standalone is no longer shipped.
- **No interactive `create` wizard.** All inputs are flags (or sensible defaults).

## Requirements

### Functional Requirements

#### Command tree layout

- **REQ-F-001:** New package `neo4j-cli/internal/subcommands/docker/` following the repo's one-file-per-leaf cobra layout (per AGENTS.md "Cobra Command Layout"):
  - `docker.go` — parent `NewCmd(cfg *clicfg.Config) *cobra.Command`; ≤80 lines; registers leaves via `cmd.AddCommand(...)`.
  - `create.go`, `list.go`, `get.go`, `start.go`, `stop.go`, `delete.go` — one leaf per file with a private constructor (`newCreateCmd`, …).
  - Colocated test files: `create_test.go`, `list_test.go`, `get_test.go`, `start_test.go`, `stop_test.go`, `delete_test.go`.
  - Shared helpers: `client.go` (docker exec wrapper + interface), `labels.go` (label key constants + `Inspect` → metadata struct), `bolt_ready.go` (Bolt readiness probe), `helpers_test.go` (fake docker client).
- **REQ-F-002:** Mount the tree at root in `neo4j-cli/app/app.go` via `cmd.AddCommand(docker.NewCmd(cfg))`, alongside `aura`, `credential`, `config`, `query`, `skill`, `update`, `agentcontext`.
- **REQ-F-003:** Every write leaf (`create`, `start`, `stop`, `delete`) carries `Annotations: map[string]string{"write": "true"}` so the root `--rw` gate fires.
- **REQ-F-004:** Every read leaf (`list`, `get`) registers `--format json|table|toon` via `flags.RegisterOutputFlag`.
- **REQ-F-005:** Every leaf populates a non-empty flush-left `Example:` block following the rules from `TestAllLeafCommands_HaveExamples` (≥3 invocations, `# comment` line + blank-line separator, `neo4j-cli` prefix, `--rw` on writes, ≥1 `--format json` on reads).

#### `neo4j-cli docker create`

- **REQ-F-010:** Flags:

  | Flag | Default | Notes |
  |---|---|---|
  | `--name <s>` | *required* | Container + dbms credential name. Auto-suffixed on collision (REQ-F-014). |
  | `--version <s>` | `latest` | Maps to Docker tag: `neo4j:<version>` (community) or `neo4j:<version>-enterprise`. |
  | `--edition <community\|enterprise>` | `enterprise` | Enterprise auto-passes `NEO4J_ACCEPT_LICENSE_AGREEMENT=eval` (REQ-F-012). |
  | `--accept-license` | `false` | Enterprise only. Upgrades env var from `eval` to `yes`. No-op for `--edition community`. |
  | `--bolt-port <int>` | `7687` | Pre-flight `net.Listen` conflict check (REQ-F-013). |
  | `--http-port <int>` | `7474` | Same. |
  | `--password <s>` | random 16-char | Mapped to `-e NEO4J_AUTH=neo4j/<password>`. Surfaced in the rendered `create` output (REQ-F-015). |
  | `--ephemeral` | `false` | `docker run --rm`; skip credential persistence; emit `.env` (REQ-F-016, REQ-F-017). |
  | `--env-out-file <path>` | unset | Only meaningful with `--ephemeral`; if set, write env file there and stay silent on stdout. |
  | `--wait` | `false` | Poll Bolt on `localhost:<bolt-port>` until handshake succeeds, 60 s timeout (REQ-F-018). |
  | `--no-store-credential` | `false` | Non-ephemeral path only; skip persisting a dbms credential. |

- **REQ-F-011:** `create` shells `docker run -d` with:
  - Container name = chosen name (after collision auto-suffix).
  - `-p <bolt-port>:7687 -p <http-port>:7474`.
  - `-e NEO4J_AUTH=neo4j/<password>`.
  - `-e NEO4J_ACCEPT_LICENSE_AGREEMENT=<eval|yes>` when `--edition enterprise` (REQ-F-012).
  - Labels: `org.neo4j.cli.managed=true`, `org.neo4j.cli.edition=<edition>`, `org.neo4j.cli.version=<version>`, `org.neo4j.cli.bolt-port=<port>`, `org.neo4j.cli.http-port=<port>`, `org.neo4j.cli.ephemeral=<bool>`.
  - Image: `neo4j:<version>` (community) or `neo4j:<version>-enterprise`.
  - `--rm` when `--ephemeral`.
- **REQ-F-012:** License env var:
  - `--edition enterprise` + no `--accept-license` → `-e NEO4J_ACCEPT_LICENSE_AGREEMENT=eval`.
  - `--edition enterprise` + `--accept-license` → `-e NEO4J_ACCEPT_LICENSE_AGREEMENT=yes`.
  - `--edition community` → no `NEO4J_ACCEPT_LICENSE_AGREEMENT` env var.
- **REQ-F-013:** Port-conflict pre-flight: before invoking `docker run`, try `net.Listen("tcp", fmt.Sprintf(":%d", port))` for both `--bolt-port` and `--http-port`. On bind failure return a `clierr.NewUsageError` naming the conflicting flag (e.g. `"port 7687 is in use on the host. Pass --bolt-port <other> to pick a free port."`).
- **REQ-F-014:** Name-collision handling: enumerate (a) `docker ps -a --format '{{.Names}}'` and (b) existing dbms credential names from `cfg.Aura.Credentials().Dbms.List()`. If `<name>` exists in either set, try `<name>-1`, `<name>-2`, … up to `<name>-99`; if all are taken, error out. On a successful suffix, write `info: name "dev" already in use; using "dev-1"` to stderr and use the chosen name everywhere downstream (container, credential, output).
- **REQ-F-015:** Password generation: if `--password` is unset, generate a 16-byte random secret using `crypto/rand` and encode as base64-URL-safe (no padding). Include the password as a column / JSON field in the rendered `create` output (no extra stderr echo).
- **REQ-F-016:** Credential persistence (non-ephemeral path): unless `--no-store-credential`, call `credentials.DbmsCredentials.Add(name, "neo4j", password, "neo4j", "neo4j://localhost:<bolt-port>")` after `docker run` succeeds. First credential becomes default automatically (existing behavior in `common/clicfg/credentials/dbms.go:39`).
- **REQ-F-017:** Ephemeral output: when `--ephemeral`, do **not** persist a dbms credential. Build a `.env` blob:
  ```
  # neo4j-cli docker — <name> @ <image>
  NEO4J_URI=neo4j://localhost:<bolt-port>
  NEO4J_USERNAME=neo4j
  NEO4J_PASSWORD=<password>
  NEO4J_DATABASE=neo4j
  ```
  - If `--env-out-file <path>` is set, write the blob there with `os.OpenFile(path, O_WRONLY|O_CREATE|O_TRUNC, 0600)` and stay silent on stdout (so the path can be piped).
  - Otherwise emit the blob to stdout.
  - Env-var names match what `neo4j-cli/query/connect.go:29-32` consumes; no change to `query`.
- **REQ-F-018:** `--wait` semantics: after `docker run`, poll `bolt://localhost:<bolt-port>` until a Bolt v5 handshake succeeds, with a fixed 60 s deadline. Reuse the neo4j-go-driver vendored by `query/` (preferred) — fall back to a raw `net.Dial` + handshake only if the import introduces a cycle. On timeout return `clierr.NewUsageError("container started but Bolt did not become ready within 60s; check 'docker logs <name>'")` but leave the container running (don't tear down).

#### `neo4j-cli docker list`

- **REQ-F-020:** Lists containers carrying `label=org.neo4j.cli.managed=true` via `docker ps -a --filter label=org.neo4j.cli.managed=true --format '{{json .}}'`.
- **REQ-F-021:** Columns / JSON fields: `name`, `status`, `edition`, `version`, `bolt-port`, `http-port`, `ephemeral`. (Status is Docker's reported state; the rest come from labels.)
- **REQ-F-022:** Empty result renders as an empty table / empty JSON array, no error.

#### `neo4j-cli docker get <name>`

- **REQ-F-030:** Single-container variant of `list`, scoped via `docker inspect <name>` (or `docker ps -a --filter name=^<name>$ --filter label=org.neo4j.cli.managed=true`).
- **REQ-F-031:** Fields include everything from `list`, plus `uri` (`neo4j://localhost:<bolt-port>`) and `image`.
- **REQ-F-032:** Unknown name → `clierr.NewUsageError("no managed container named %q (use 'neo4j-cli docker list' to see managed containers)", name)`. Containers that exist in Docker but lack the `org.neo4j.cli.managed=true` label are treated as unknown.

#### `neo4j-cli docker start <name>` / `docker stop <name>`

- **REQ-F-040:** `start` shells `docker start <name>`; `stop` shells `docker stop <name>`. Both accept exactly one positional name (`cobra.ExactArgs(1)`) and require `--rw`.
- **REQ-F-041:** Both support `--wait`. For `start --wait`, the readiness check from REQ-F-018 applies. For `stop --wait`, poll `docker inspect` until `.State.Running == false`, fixed 60 s timeout.
- **REQ-F-042:** Operating on an ephemeral container that has already been removed (Docker reports "No such container") → `clierr.NewUsageError("container %q is ephemeral and has already been removed; 'neo4j-cli docker create' to start a fresh one", name)`.
- **REQ-F-043:** Operating on a non-managed container (exists in Docker, no `org.neo4j.cli.managed=true` label) → unknown-name error (REQ-F-032), do **not** touch the container.

#### `neo4j-cli docker delete <name>`

- **REQ-F-050:** Shells `docker rm -f <name>` then calls `credentials.DbmsCredentials.Remove(name)` (best-effort — missing credential is not an error). Requires `--rw`.
- **REQ-F-051:** TTY confirmation prompt: when stdin is a TTY and `--force` is **not** set, prompt `Delete container <name> and its dbms credential? [y/N]` (default N). Reuse any existing TTY/prompt helper if one exists in `common/`; otherwise add a small helper colocated in `docker/`.
- **REQ-F-052:** When stdin is **not** a TTY (piped / scripted) and `--force` is not set, error out with `clierr.NewUsageError("non-TTY caller must pass --force to confirm deletion")`. Never proceed silently.
- **REQ-F-053:** Refuses to operate on non-managed containers (REQ-F-043 logic).
- **REQ-F-054:** No `--keep-credential` flag. The credential is always removed alongside the container.

#### Docker availability

- **REQ-F-060:** `client.go` calls `exec.LookPath("docker")` on the **first invocation of any docker subcommand** (not at root binding time) and caches the result. Missing docker → `clierr.NewUsageError("docker not found in PATH — install Docker Desktop (https://www.docker.com/products/docker-desktop/) or the docker CLI")`. Other neo4j-cli subtrees (`aura`, `query`, `credential`, …) must remain usable without docker installed.
- **REQ-F-061:** Docker exec errors surface to the user verbatim (stderr from `docker` is captured and included in the returned `clierr` message), so the user can act on them.

### Non-Functional Requirements

- **REQ-NF-001:** **No new third-party Go dependency.** Use `os/exec` + the already-vendored neo4j-go-driver (from `query/`) — do not pull in `github.com/docker/docker` or the Docker Go SDK. (Confirmed lightweight per AGENTS.md "Repo Layout Notes".)
- **REQ-NF-002:** **Cross-platform**: works on macOS, Linux, Windows (matches the CI matrix in AGENTS.md "Project Overview"). Port-conflict pre-flight uses `net.Listen("tcp", ":...")`. Path handling uses `filepath` (not raw `/`).
- **REQ-NF-003:** **Hermetic tests.** Define a `dockerClient` interface in `client.go` with a default `exec.Command`-based implementation and a fake in `helpers_test.go`. Unit tests must not require a running Docker daemon. (One optional `_smoke_test.go` with the `exec.LookPath("docker")` guard is acceptable, mirroring `neo4j-cli/query/query_bolt_smoke_test.go`.)
- **REQ-NF-004:** **Security**:
  - Password generation uses `crypto/rand` (REQ-F-015) — not `math/rand`.
  - Env-file writes use `0600` permissions (REQ-F-017).
  - The full `docker run` command line (including `NEO4J_AUTH=neo4j/<password>`) must **not** be echoed in verbose / debug output.
  - `--name` and `--env-out-file <path>` are validated against shell-injection vectors before being passed to `os/exec` (no `sh -c`; pass as discrete `Args`).
  - Run the `golang-security` skill as a final gate before merge (memory: "Security review gate").
- **REQ-NF-005:** **License headers.** Every new `.go` file starts with the Neo4j copyright header; `make license-check` is clean.
- **REQ-NF-006:** **Format / lint gates pass.** `make fmt-check` and `make lint` are clean. CI's golangci-lint v2 (with `gofmt` formatter) must be green.
- **REQ-NF-007:** **Skill bundle regeneration committed.** `go generate ./neo4j-cli/internal/skill/...` regenerates `bundle/SKILL.md` and `bundle/references/docker.md`; both are committed in the same PR. `TestGenerator_RoundTrip` must pass.
- **REQ-NF-008:** **Agent-context updated.** `neo4j-cli agent-context` reflects the new tree automatically (it walks the cobra tree at runtime — `agentcontext/build.go`). If `asyncFlag` / `exitCodes` / `errorCodes` constants need new entries, update them in the same PR.

### Documentation Requirements

- **REQ-DOC-001:** **README.md** — add a "Local Neo4j (Docker)" section near the existing Aura/query sections covering: `create` → `query --credential <name>` → `delete` flow, plus the `--ephemeral` + `--env-out-file` → `query --env <path>` flow.
- **REQ-DOC-002:** **AGENTS.md** (the `CLAUDE.md` symlink) — add a short subsection under "Architecture" pointing at `neo4j-cli/internal/subcommands/docker/` and noting Docker as the source-of-truth (label-based discovery, no separate state file).
- **REQ-DOC-003:** **`neo4j-cli/internal/skill/description.txt`** — extend the single-paragraph description so the skill triggers on "docker", "local neo4j", "start/stop a Neo4j container". Stay ≤1024 chars, third person (per AGENTS.md "Cobra Help / Skill Bundle Rendering Notes").
- **REQ-DOC-004:** **`neo4j-cli/internal/skill/additions.md`** — add a section describing the docker subtree with copy-paste examples, plus an entry covering `--ephemeral` + `--env-out-file` flowing into `query --env`.
- **REQ-DOC-005:** **Regenerated skill bundle** — `bundle/SKILL.md` and `bundle/references/docker.md` are produced by `go generate` and committed. Do **not** hand-edit.
- **REQ-DOC-006:** **CONTRIBUTING.md** — if a "Commands" or "Add a new command" subsection exists, list the docker tree alongside aura / query / credential.
- **REQ-DOC-007:** **Changelog** — `changie new --projects neo4j-cli --kind Minor --body "Add 'docker' command tree for managing local Neo4j containers"` (or hand-author a YAML under `.changes/unreleased/` per AGENTS.md "Changie Notes" if changie isn't installed).
- **REQ-DOC-008:** **Website** — out of scope for this PR; flag in PR description for the next prompt-driven website pass (`.github/prompts/website-update.md`).

## Technical Considerations

### Architecture & integration points

- **Cobra tree mount**: a single new line in `neo4j-cli/app/app.go` between the existing `query` and `skill` mounts (no other root edits).
- **Source of truth = Docker**. Listing/inspection uses `docker ps --filter label=org.neo4j.cli.managed=true`. No new state file; no migrations.
- **Credentials integration**: reuse `common/clicfg/credentials/dbms.go` (`Add` at line 25, `Remove` at line 46). First credential becomes default automatically.
- **Bolt readiness**: reuse the neo4j-go-driver already vendored by `query/`. The smoke-test in `neo4j-cli/query/query_bolt_smoke_test.go` is the closest existing pattern. If the import path causes a cycle, fall back to `net.Dial` + Bolt v5 hello frame.
- **TTY confirmation prompt**: needed for `delete`. Check `common/` for any existing TTY/prompt helper before adding a new one.
- **Output rendering**: reuse the shared output package used by credential / config commands (the one referenced by `dbms.PrintableDbmsCredentials.AsArray`). Confirm import path before wiring.
- **Agent-context**: walks the cobra tree at runtime (`agentcontext/build.go`) — picks up new commands automatically. Hand-coded `schemaVersion` / `exitCodes` / `errorCodes` / `asyncFlag` only need touching if we add new exit codes (none planned).

### Implementation order

1. `client.go` — `dockerClient` interface + `execClient` default + `lookupDocker` once.
2. `labels.go` — label key constants + `Container` metadata struct + `Inspect(name)`.
3. `docker.go` — parent `NewCmd`; mount in `app/app.go`.
4. `create.go` — by far the densest leaf; build incrementally (flags → run → labels → credential / env-out-file → wait).
5. `bolt_ready.go` — extract the readiness probe so it's reusable by `start`.
6. `list.go`, `get.go` — read leaves.
7. `start.go`, `stop.go`, `delete.go` — write leaves.
8. Test fakes + colocated tests as each leaf lands.
9. `go generate ./neo4j-cli/internal/skill/...` + commit regenerated bundle.
10. Docs (REQ-DOC-001 … REQ-DOC-008).
11. Final gates: `make fmt-check && make lint && make test && make license-check && make generate-check`.

### Risk areas

- **Bolt-driver import cycle** — `query/` imports the driver; this package would import it too. If a cycle pops up (e.g. via `clicfg`), fall back to a tiny `net.Dial` + Bolt v5 hello frame. Decide at implementation time.
- **TTY detection on Windows CI** — `term.IsTerminal(int(os.Stdin.Fd()))` is the standard pattern but worth a manual test on the windows runner. Non-TTY callers must pass `--force` (REQ-F-052), so the worst case is "Windows CI requires `--force`", which is acceptable.
- **`docker ps --format '{{json .}}'` output stability** — Docker's JSON output for `ps` is documented but field names differ slightly across Docker versions. Pin to the fields we actually need (`Names`, `Status`, `State`, `Labels`) and parse defensively.
- **Skill bundle drift** — adding a new command tree always requires bundle regen. AGENTS.md "Makefile Notes" calls this out explicitly. Run `go generate` once the cobra tree is final, then commit.
- **Image-pull latency on first `create`** — `docker run` will pull on demand; this can be slow. Don't add a progress spinner in v1; document in `--help` that the first `create` may take a minute.

## Acceptance Criteria

- [ ] `bin/neo4j-cli docker --help` shows six leaves: `create`, `list`, `get`, `start`, `stop`, `delete`. No `logs`. No `pause` / `resume`.
- [ ] `bin/neo4j-cli docker create --name dev --rw` (no other flags) produces a running enterprise container with `NEO4J_ACCEPT_LICENSE_AGREEMENT=eval`, ports 7474 + 7687 published, a generated 16-char password visible in the rendered output, and a dbms credential named `dev`.
- [ ] `bin/neo4j-cli docker create --name dev --rw` twice in a row produces a second container `dev-1` and writes `info: name "dev" already in use; using "dev-1"` to stderr.
- [ ] `bin/neo4j-cli docker create --name dev --edition enterprise --accept-license --rw` passes `NEO4J_ACCEPT_LICENSE_AGREEMENT=yes` to the container.
- [ ] `bin/neo4j-cli docker create --name dev --edition community --rw` passes no `NEO4J_ACCEPT_LICENSE_AGREEMENT` env var and pulls `neo4j:latest` (community tag).
- [ ] Port-conflict pre-flight: occupy 7687, run `create` → exits non-zero with a `clierr.UsageError` naming `--bolt-port`.
- [ ] `bin/neo4j-cli docker get dev --format json` returns JSON with `name`, `status`, `edition=enterprise`, `version=latest`, `bolt-port=7687`, `http-port=7474`, `ephemeral=false`, `uri=neo4j://localhost:7687`, `image=neo4j:enterprise`.
- [ ] `bin/neo4j-cli docker list --format table` includes the columns listed in REQ-F-021.
- [ ] `bin/neo4j-cli query --credential dev "RETURN 1"` returns `1`.
- [ ] `bin/neo4j-cli docker stop dev --rw --wait` exits cleanly; `docker ps -a` shows the container Exited.
- [ ] `bin/neo4j-cli docker start dev --rw --wait` returns the container to Running and Bolt is responsive.
- [ ] `bin/neo4j-cli docker delete dev --rw` (on a TTY) prompts `[y/N]`; on `y` removes the container AND the dbms credential.
- [ ] `printf 'y\n' | bin/neo4j-cli docker delete dev --rw` (non-TTY, no `--force`) exits non-zero with a clear usage error.
- [ ] `bin/neo4j-cli docker delete dev --rw --force` (any caller) deletes without prompting.
- [ ] `bin/neo4j-cli docker create --name tmp --ephemeral --env-out-file /tmp/n.env --rw --wait` writes `/tmp/n.env` with mode `0600`, header comment, and the four `NEO4J_*` env vars. No `tmp` dbms credential is created. The container has label `org.neo4j.cli.ephemeral=true`.
- [ ] `bin/neo4j-cli query --env /tmp/n.env "RETURN 1"` (using the file from the previous step) returns `1`.
- [ ] `bin/neo4j-cli docker stop tmp --rw` removes the ephemeral container (because `--rm`).
- [ ] With `docker` removed from PATH: `bin/neo4j-cli docker list --rw` errors out with the install hint; `bin/neo4j-cli aura instance list --rw` continues to work.
- [ ] `bin/neo4j-cli agent-context` includes the new `docker` subtree.
- [ ] Skill bundle: `neo4j-cli/internal/skill/bundle/references/docker.md` exists and lists every leaf with its Example block. `TestGenerator_RoundTrip` is green.
- [ ] `make fmt-check && make lint && make test && make license-check && make generate-check` all pass.
- [ ] `.changes/unreleased/` contains a `Minor` entry for CLI-123.
- [ ] README.md, AGENTS.md, skill `description.txt`, and skill `additions.md` are updated per REQ-DOC-001 … REQ-DOC-004.

## Out of Scope

- `docker logs` leaf (deferred — track as a follow-up Linear issue if desired).
- `--wait-timeout <dur>` flag (deferred; fixed 60 s for v1).
- Memory / heap / cache config (`--memory`, `dbms.memory.heap.*`) — explicitly excluded by the issue.
- Tenant / project / cloud-provider / region flags — explicitly excluded by the issue (Aura-only concepts).
- Persistent named volume / data-dir management.
- Compose / Kubernetes / Podman / remote Docker daemons.
- Native (non-Docker) local Neo4j runners.
- Website edits (`gh-pages` branch) — handled by the separate prompt-driven website update flow.
- Aura standalone binary edits — the aura standalone is no longer shipped.
- A `docker` group in `agent-context.errorCodes` / `exitCodes` unless implementation surfaces a genuinely new error class.

## Open Questions

None. All design questions were resolved in the locked source plan (`/Users/oskarhane/.claude/plans/i-d-like-to-look-dapper-valley.md`, "Decisions locked from review" section).
