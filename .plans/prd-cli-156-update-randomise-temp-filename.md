# PRD: CLI-156 randomise temp filename on elevated swap path

## Overview

Oplane threat model (CLI-155, REQ-00065537) flagged the `neo4j-cli update` self-update flow. `Swap()` in `neo4j-cli/internal/subcommands/update/swap.go` writes the freshly-extracted replacement binary to a **fixed** path `<plan.tmpDir>/neo4j-cli.new`. On the elevated branch `plan.tmpDir == os.TempDir()` (typically `/tmp`), which is multi-user-writable. An attacker on the same host can pre-plant `/tmp/neo4j-cli.new` (regular file or symlink); the sticky bit blocks our best-effort `os.Remove`, then `O_EXCL` in `writeRegularFile` refuses to create — extract fails and `sudo install` is never invoked.

No arbitrary write primitive (O_EXCL prevents symlink-follow), but a clean DoS: any victim running `neo4j-cli update` is silently blocked.

Fix: per-invocation random hex suffix on `tmpNew`, mirroring the pattern `dirWritable` already uses at `swap.go:330-334`. Apply uniformly to both direct and elevated branches.

## Goals

- Close the multi-user `/tmp` pre-plant DoS on the elevated swap path.
- Apply random-suffix tmp naming uniformly to direct and elevated branches (one code path, no conditional).
- Preserve existing cleanup semantics — all `os.Remove(tmpNew)` calls must still target the correct file.

## Non-Goals

- Changing the swap algorithm, signing, checksum verification, or sudo elevation flow.
- Re-engineering temp-file creation to use `os.CreateTemp` (a slightly different API contract; the existing `O_EXCL` write path stays).
- Addressing other gaps from CLI-155 (those are tracked as separate sub-issues).
- User-visible CLI flag or output changes.
- Changelog entry (internal hardening, no user-visible behaviour change).

## Requirements

### Functional Requirements

- REQ-F-001: `Swap()` MUST construct `tmpNew` as `filepath.Join(plan.tmpDir, "neo4j-cli.new."+<random hex>)` where `<random hex>` is 16 hex chars derived from 8 bytes of `crypto/rand`.
- REQ-F-002: The random-suffix construction MUST be applied uniformly to both the direct (writable bin dir) and elevated (`os.TempDir()`) branches — the call site is single, prior to the existing extract/swap logic.
- REQ-F-003: All existing `os.Remove(tmpNew)` cleanup calls (`swap.go:444`, `:449`, `:462`, `:472`, `:478`) MUST still target the same `tmpNew` variable (no logic change beyond the name construction).
- REQ-F-004: On `rand.Read` failure, `Swap()` MUST return a wrapped error (`fmt.Errorf("swap: generate tmp suffix: %w", err)`) and perform no network I/O or extraction — fail-closed.
- REQ-F-005: The pre-extract best-effort `_ = os.Remove(tmpNew)` (currently `swap.go:439`) MUST be removed — the random suffix makes pre-existence effectively impossible, and `O_EXCL` in `writeRegularFile` already covers the theoretical case.
- REQ-F-006: File-level docstring (`swap.go:29-35`) and inline comment block (`swap.go:431-435`) MUST be updated to reflect the random suffix and reference the pre-plant DoS rationale.

### Non-Functional Requirements

- REQ-NF-001: No behavioural change visible to end users — same exit codes, same stderr narrative, same final binary at `currentBinaryPath`.
- REQ-NF-002: Use `crypto/rand`, not `math/rand`. Both packages are already imported transitively via the existing `dirWritable` helper.
- REQ-NF-003: Random suffix length 16 hex chars (8 raw bytes) — matches `dirWritable` convention exactly.
- REQ-NF-004: All existing tests must continue to pass with assertions updated to match the new tmp-name shape.

## Technical Considerations

### Code location

`neo4j-cli/internal/subcommands/update/swap.go`:

- Lines 29-35: file-level docstring mentions fixed path; update narrative.
- Lines 67-91: imports (`crypto/rand`, `encoding/hex` already present; no new deps).
- Line 222-257: `planSwap` (untouched; still returns `plan.tmpDir`).
- Line 375-482: `Swap()` — the changed call site.
- Line 435: `tmpNew := filepath.Join(plan.tmpDir, "neo4j-cli.new")` → replaced by random-suffix construction added right after `planSwap` returns success (well above the network I/O so a `rand.Read` failure short-circuits before any download — fail-closed).
- Line 439: `_ = os.Remove(tmpNew)` → removed.

### Pattern reuse

`dirWritable` (lines 329-345) already does the exact thing we need:

```go
var randBytes [8]byte
if _, err := rand.Read(randBytes[:]); err != nil {
    return false, fmt.Errorf("dirWritable: generate probe name: %w", err)
}
probe := filepath.Join(dir, ".neo4j-cli-probe."+hex.EncodeToString(randBytes[:]))
```

We mirror this verbatim, swapping the dir, basename, and error prefix. No new helper function — three lines of inline code with clear intent.

### Test surface

`neo4j-cli/internal/subcommands/update/swap_test.go` — several assertions look up `tmpNew` by fixed name and must move to a glob/prefix match:

- Post-success cleanup checks: `:242-243`, `:307-308`, `:445-446`, `:1140-1141`, `:1210-1211`, `:1418-1419` — replace `os.Stat(filepath.Join(tmpDir, "neo4j-cli.new"))` with a `filepath.Glob(filepath.Join(tmpDir, "neo4j-cli.new.*"))` len-zero assertion.
- Elevated-branch argv assertions: `:1196-1199`, `:1384` — capture `counters.capturedCmd.Args[4]` (the src arg) and assert prefix `filepath.Join(elevTmpDir, "neo4j-cli.new.")` plus 16-hex-char suffix; assert `Args[5] == currentBinary`.

Direct-call tests of `extractTarGzEntry`/`writeRegularFile`/`elevatedSwap` (`:330`, `:364`, `:610`, `:621`, `:928`) are unchanged — they pass a chosen destination path into the leaf helper, not through `Swap()`.

### New test

`TestSwap_TmpName_RandomSuffixPerInvocation` — drives two back-to-back `Swap` calls against a shared `tempDirFn` with mocked http + sudo `runCommandFn`. Captures each call's src argv (`cmd.Args[4]`), asserts the two paths differ. Satisfies the ticket's acceptance ("two concurrent Swap calls with the same tempDirFn() do not collide"); "concurrent" here means "sharing the same tmpDir" not "racing goroutines" — sequential execution is sufficient since `rand.Read` collisions aren't ordering-dependent.

### Risk

Low. Two-line behavioural change in a well-tested helper. Existing tests cover all swap branches; the new assertion shape is mechanical. No public API change.

## Acceptance Criteria

- [ ] `Swap()` constructs `tmpNew` with a 16-hex-char per-invocation random suffix using `crypto/rand`.
- [ ] Random suffix applied uniformly to direct and elevated branches.
- [ ] `rand.Read` failure aborts before network I/O with a wrapped error.
- [ ] Stale `os.Remove(tmpNew)` pre-extract call removed.
- [ ] Docstring and inline comments updated to reflect new tmp-name shape and threat-mitigation rationale.
- [ ] All existing swap tests pass with updated assertions.
- [ ] New test `TestSwap_TmpName_RandomSuffixPerInvocation` proves two consecutive `Swap` calls under shared `tempDirFn` produce distinct `tmpNew` paths.
- [ ] `make test`, `make fmt-check`, `make lint` all pass.
- [ ] `golang-security` skill run as final gate (per user memory) — no new findings on swap path.

## Out of Scope

- Other CLI-155 sub-issues (--force warning, SECURITY.md — separate tickets).
- Switching to `os.CreateTemp`.
- Hardening the direct branch beyond uniform application of the random suffix (the direct branch lives in the running binary's dir, not multi-user-writable).
- Changelog entry — internal hardening, no user-visible behaviour change.

## Open Questions

None — direct + elevated both get the random suffix, no changelog entry.
