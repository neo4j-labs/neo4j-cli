# PRD: Install Neo4j Skills (curated catalog)

Linear: [CLI-102](https://linear.app/neo4j/issue/CLI-102/install-neo4j-skills)

## Overview

Extend `neo4j-cli skill` so it can install any skill from the curated `github.com/neo4j-contrib/neo4j-skills` catalog (29 skills today — Cypher, modeling, drivers, GraphRAG, GDS, Aura, etc.), not just the binary's own embedded SKILL.md. The catalog is downloaded over HTTPS on demand, cached locally, and gated on a single marketplace `version` field (no per-skill version comparison). The existing positional-as-agent UX is replaced with positional-as-skill-name and a `--agent` flag.

## Goals

- One unified `neo4j-cli skill` UX installs **either** the embedded self-skill **or** any curated catalog skill into any supported agent (Claude Code, Cursor, Windsurf, Codex, Gemini-CLI, etc.).
- HTTPS-based discovery and content fetch from `neo4j-contrib/neo4j-skills`; no binary-bloat embedding of the catalog.
- Cache the upstream `plugin.json` and gate refresh on its single `version` field — one HTTP GET answers "is anything stale?" for the whole catalog.
- Preserve the self-skill (`self`) as an immovable entry; never accidentally remove it during bulk operations.
- Keep the existing five-leaf shape (`install`, `remove`, `list`, `check`, `print`) plus one new leaf (`refresh`).

## Non-Goals

- Per-skill version metadata (upstream uses one bundle version).
- `--source <repo>` flag, persistent named sources, or any "marketplace" abstraction for arbitrary repos. MVP is fixed to `neo4j-contrib/neo4j-skills`.
- A `skills` (plural) sibling command — AGENTS.md mandates singular nouns.
- Soft-deprecation of `skill install <agent-name>` — hard break, called out in the changelog.
- Search UI (`skill search <term>`) — `skill list | grep` is enough for MVP.
- Per-binary skill catalogs for non-`neo4j-cli` binaries — only the neo4j-cli surface gets the catalog work in MVP.

## Requirements

### Functional Requirements

**Command surface**

- REQ-F-001: `neo4j-cli skill install` (no args) installs the **embedded self-skill** into every detected agent. Same shape as today's no-arg behaviour.
- REQ-F-002: `neo4j-cli skill install <skill-name>` installs the named **catalog skill** into every detected agent. Skill name is the upstream skill directory name (e.g. `neo4j-cypher-skill`).
- REQ-F-003: `neo4j-cli skill install --all` installs the self-skill **and** every skill listed in the cached `plugin.json` into every detected agent.
- REQ-F-004: `--agent <name>` flag scopes any install/remove operation to a single named agent (case-insensitive). Replaces today's positional `[agent]` arg.
- REQ-F-005: `--refresh` flag (on `install`, `list`, `check`) forces a network refresh of the cached `plugin.json` before the operation.
- REQ-F-006: `neo4j-cli skill remove <skill-name> [--agent X]` removes one named skill (catalog or self) from all detected agents or one. Idempotent — missing installs return success.
- REQ-F-007: `neo4j-cli skill remove --all` removes every **catalog** skill from all detected agents. Never touches the embedded self-skill (`neo4j-cli`). The omission is documented in `--help` and emitted as a note in table output.
- REQ-F-008: `neo4j-cli skill list` produces one row per `(skill × agent)` from cached catalog + self-skill, columns `skill, source, agent, detected, installed, installed_version, available_version, status`. Uses cache as-is unless `--refresh` is passed.
- REQ-F-009: `neo4j-cli skill check` exits non-zero if any installed skill's recorded `version:` frontmatter differs from the corresponding source version (binary version for self-skill, cached `plugin.json.version` for catalog skills). Output columns `skill, agent, installed_version, current_version, status` where status ∈ `ok`, `drift`, `unknown-version`.
- REQ-F-010: `neo4j-cli skill print [skill-name]` prints the SKILL.md for the named skill (catalog or self) to stdout. Defaults to the self-skill when no arg supplied (matches today's behaviour).
- REQ-F-011: `neo4j-cli skill refresh` forces a fresh download of `plugin.json` + the repo tarball if `version` changed. New leaf.
- REQ-F-012: **Hard break**: when `<skill-name>` positional doesn't match any known skill (catalog or self) but matches an agent name, error with `unknown skill: <name>; did you mean '--agent <name>'?`. No silent fall-through.

**Catalog mechanism**

- REQ-F-013: Upstream is hardcoded to `https://raw.githubusercontent.com/neo4j-contrib/neo4j-skills/main/.claude-plugin/plugin.json` for metadata and `https://codeload.github.com/neo4j-contrib/neo4j-skills/tar.gz/refs/heads/main` for content. URL constants live in `common/skill/catalog/source.go`.
- REQ-F-014: Cache lives under `os.UserCacheDir()/neo4j-cli/skill-catalog/`:
  ```
  plugin.json                    # latest fetched copy
  fetched-at                     # ISO8601 timestamp
  content/<version>/<skill>/     # extracted skill content, keyed by marketplace version
  ```
- REQ-F-015: Auto-refresh policy: `list`/`install`/`check` auto-fetch `plugin.json` when the cached copy is missing or older than **24h**. Tarball re-extraction fires only when `cached.version != upstream.version`. `--refresh` always forces a fresh `plugin.json` fetch + tarball check.
- REQ-F-016: Tarball extraction must allowlist only the skill subdirectories listed in `plugin.json.skills[]` (defence against path traversal, symlink, or absolute paths in the archive). Reject and abort the extract if any entry escapes the allowlist.
- REQ-F-017: Catalog install copies `content/<version>/<skill>/{SKILL.md, references/*}` into `<agent.SkillsDir>/<skill-name>/` and **injects** `version: <plugin.json.version>` into the SKILL.md YAML frontmatter — overwriting an existing `version:` line, or inserting one immediately before the closing `---` if upstream has none (today none have one).
- REQ-F-018: Network failure with no cache → fail with a clear error pointing the user at `neo4j-cli skill refresh` after restoring connectivity, exit non-zero.
- REQ-F-019: Network failure with cache → log a warning to stderr, fall back to cache, continue. Exit zero.

**Self-skill handling**

- REQ-F-020: Self-skill is identified by the canonical name `self` and shown as `source: embedded` in `skill list` output, always at the top. The binary name (`neo4j-cli`, from the `skillName` arg passed to `skill.NewCmd`) is accepted as an alias for back-compat.
- REQ-F-021: Self-skill is addressable as `self` (canonical) or as the binary name `neo4j-cli` (alias) in every command that takes a `<skill-name>` positional (`install`, `remove`, `print`). `neo4j-cli skill install` (no-arg) ≡ `neo4j-cli skill install self` ≡ `neo4j-cli skill install neo4j-cli`. Both names are reserved — catalog Lookup must reject any upstream skill called `self` or matching the binary name.
- REQ-F-022: `skill remove --all` never removes the self-skill.
- REQ-F-023: `skill remove self` (or `skill remove neo4j-cli`) is allowed and prints a hint: `Run 'neo4j-cli skill install' to reinstall.`
- REQ-F-024: `skill install --all` always includes the self-skill.

### Non-Functional Requirements

- REQ-NF-001: All new code follows the Cobra one-file-per-leaf layout from AGENTS.md. `refresh.go` is a new leaf file in `common/skill/`; tests colocate as `refresh_test.go`.
- REQ-NF-002: New `common/skill/catalog/` package mirrors existing test conventions: table-driven, colocated, `httptest`-based fixtures, no real network or real filesystem touched in unit tests. Test seam `userCacheDirFn` for cache-dir override.
- REQ-NF-003: All file writes go through `cfg.Aura.Fs()` (afero) for testability — including cache writes. Cache writes use mode 0600 for files / 0755 for dirs, matching existing installer.
- REQ-NF-004: Network calls use `net/http.DefaultClient` with a `User-Agent: neo4j-cli/<version>` header and a 30s overall timeout. No external HTTP library dependency.
- REQ-NF-005: Tarball stream-extracted via `archive/tar` + `compress/gzip` stdlib only — no third-party deps. Max-decompressed-size guard (e.g. 20 MB cap) to prevent zip-bomb DoS.
- REQ-NF-006: `make test && make fmt-check && make lint` must all pass. `make generate-check` must pass after `go generate ./neo4j-cli/internal/skill/...` is re-run to reflect the new flags in the bundled SKILL.md.
- REQ-NF-007: Changelog entry filed under `.changes/unreleased/neo4j-cli-Major-<YYYYMMDD>-<HHMMSS>.yaml` (this is a breaking change to the `skill install` positional shape).
- REQ-NF-008: `print` and `list` remain offline-capable when the cache is populated; only `install` / `refresh` / `--refresh` need network.
- REQ-NF-009: Windows/macOS/Linux path handling consistent with existing skill installer — use `filepath.Join` / `filepath.FromSlash` for FS writes, forward slashes for FS sources.
- REQ-NF-010: Output format obeys the global `--format` flag (`json` / `table` / `toon`) for every leaf, via `commonoutput.PrintBodyMap`. JSON shape is documented per-leaf and frozen as a contract.

## Technical Considerations

**Surface refactor** — `common/skill/installer.go` currently has a single `Install(filesystem, bundle, skillName, version, agentFilter)` function. It needs to gain a source concept. Cleanest shape:

```go
type Source struct {
    FS      fs.FS  // os.DirFS(cachedSkillDir) for catalog, embed.FS for self
    Version string // plugin.json version for catalog, binary version for self
}

Install(filesystem afero.Fs, src Source, skillName, agentFilter string) ([]*Agent, error)
```

The existing `copyBundleWithVersion` refactors to take a `Source` and inject (not just substitute) the version. `resolveTargets` is unchanged — agent resolution is orthogonal.

**Catalog API surface** (new `common/skill/catalog/` package):

```go
type Catalog struct {
    Version string
    Skills  []SkillEntry
    cacheRoot string
}
type SkillEntry struct {
    Name string  // "neo4j-cypher-skill"
    Path string  // "./neo4j-cypher-skill" from plugin.json
}

func Load(fs afero.Fs, cacheRoot string) (*Catalog, error)          // read-only, no network
func (c *Catalog) Refresh(ctx context.Context, fs afero.Fs) error    // fetches plugin.json + tarball if needed
func (c *Catalog) Lookup(name string) (*SkillEntry, fs.FS, error)    // returns the per-skill fs.FS rooted at content/<ver>/<name>/
func (c *Catalog) Stale(ttl time.Duration) bool                       // based on fetched-at timestamp
```

**Soft-fail vs hard-fail for cache** — `list`/`check` tolerate a missing cache (show only the self-skill with a hint to run `refresh`); `install <catalog-skill>` requires the cache and triggers an auto-refresh if missing.

**Self-skill name conflict** — the user-facing decision was to use `self` as the canonical self-skill name, with the binary name (`neo4j-cli`) as a back-compat alias. If upstream ever adds a skill called `self` or matching the binary name, we have a collision; the catalog Lookup must check both and fail closed (catalog cannot shadow self). One guard in `Catalog.Lookup` rejecting either name from the upstream side.

**Path-traversal hardening** — every tar entry must `filepath.Clean(path)`, reject if it starts with `..` or `/`, reject symlinks (`tar.TypeSymlink`/`TypeLink`), reject device/fifo nodes; only `tar.TypeReg` + `tar.TypeDir` allowed. Top-of-archive dir name (`neo4j-skills-<sha>/`) is stripped before allowlist check.

**Generated bundle drift** — `make generate-check` will fail if `--all`/`--agent`/`--refresh` flag additions don't make it into the regenerated `neo4j-cli/internal/skill/bundle/references/skill.md`. Plan includes the `go generate ./neo4j-cli/internal/skill/...` step in the task list explicitly.

**No driver / Aura code touched** — this is a pure addition under `common/skill/` + `common/skill/catalog/`. No Bolt, no Aura API, no credentials. Easy to keep isolated.

**Test seams added**:
- `userCacheDirFn func() (string, error)` package var in `common/skill/catalog/cache.go`
- `httpDoer interface { Do(*http.Request) (*http.Response, error) }` parameterised through `Catalog.Refresh` so tests inject an httptest server.
- `nowFn func() time.Time` for TTL test seam.

## Acceptance Criteria

- [ ] `neo4j-cli skill install` (no args) still installs the self-skill into all detected agents — back-compat preserved.
- [ ] `neo4j-cli skill install neo4j-cypher-skill` downloads + caches the upstream tarball on first run, writes SKILL.md + references/* into each detected agent's skills dir, with frontmatter `version: 1.0.0` (matching upstream `plugin.json`).
- [ ] `neo4j-cli skill install --all` installs self-skill + every entry in `plugin.json.skills[]`.
- [ ] `neo4j-cli skill install <name> --agent claude-code` installs into one agent only.
- [ ] `neo4j-cli skill install claude-code` (legacy form, no matching skill) exits non-zero with `unknown skill: claude-code; did you mean '--agent claude-code'?`.
- [ ] `neo4j-cli skill remove --all` removes every catalog skill from every detected agent but leaves the self-skill on disk.
- [ ] `neo4j-cli skill remove self` removes the self-skill and prints `Run 'neo4j-cli skill install' to reinstall.`. Same behaviour via the alias `neo4j-cli skill remove neo4j-cli`.
- [ ] `neo4j-cli skill install self` and `neo4j-cli skill print self` resolve to the embedded self-skill (canonical). Alias form `… install neo4j-cli` / `… print neo4j-cli` resolves to the same.
- [ ] A synthetic upstream `plugin.json` listing a skill named `self` or matching the binary name (`neo4j-cli`) is rejected by catalog Lookup.
- [ ] `neo4j-cli skill list` (with populated cache) lists the self-skill + every catalog entry × every detected agent, with `installed_version`, `available_version`, `status`.
- [ ] `neo4j-cli skill list` (cold cache) shows only the self-skill and a note pointing at `neo4j-cli skill refresh`.
- [ ] `neo4j-cli skill check` exits zero when all installed skills match their source version; exits non-zero when any drifts. Tested by hand-editing an installed SKILL.md's `version:` and re-running.
- [ ] `neo4j-cli skill print neo4j-cypher-skill` prints the cached SKILL.md for that skill to stdout. `neo4j-cli skill print` (no arg) still prints the self-skill.
- [ ] `neo4j-cli skill refresh` downloads `plugin.json` and (if version changed) the tarball; subsequent `list`/`install` use the refreshed cache without network.
- [ ] Cache auto-refreshes on `list`/`install`/`check` when cached `plugin.json` is older than 24h or missing.
- [ ] Tarball extract rejects any entry outside the `plugin.json.skills[]` allowlist (test: synthetic tarball with `../../../etc/passwd` entry must error and abort).
- [ ] All output leaves honour `--format json` / `--format table` / `--format toon`.
- [ ] Network failure with populated cache → warning to stderr + fall back to cache + exit zero (for read commands).
- [ ] Network failure with no cache on `install <catalog-skill>` → clear error pointing at `refresh`, exit non-zero.
- [ ] `make test`, `make fmt-check`, `make lint`, `make generate-check` all pass on Linux + macOS + Windows CI.
- [ ] Changelog entry filed under `.changes/unreleased/` with `kind: Major`, body describing the positional-arg break and the new catalog surface.
- [ ] `neo4j-cli/internal/skill/{additions.md,description.txt}` updated to mention the new multi-skill surface and `--all` / `--agent` / `--refresh` flags; `go generate ./neo4j-cli/internal/skill/...` re-run; bundle committed.

## Out of Scope

- `--source <repo>` flag or any pluggable source surface (deferred per locked decision 3).
- Persistent named sources (`skill source add ...`).
- Per-skill versions (upstream doesn't expose them).
- A `skills` (plural) command. Singular per AGENTS.md.
- `skill search <term>` leaf.
- Soft-deprecation / dual-meaning of the positional arg — hard break, no fall-through.
- Catalog support for non-`neo4j-cli` binaries (e.g. standalone aura). Their `skill` subtree continues to install only the embedded bundle.
- Auto-update of catalog skills (no `skill update --all` that re-downloads + re-installs everywhere). Users re-run `install` explicitly.
- Mirror / proxy / offline-installer story.
- Per-agent skill filtering by upstream (e.g. only install Claude-compatible skills into Claude Code) — every catalog skill is treated as agent-agnostic markdown.

## Open Questions

(none — all clarifications captured in the source plan and locked in user decisions)
