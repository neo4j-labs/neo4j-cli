# PRD: Fix install-script checksum verification on macOS Tahoe (Darwin 25)

## Overview

`distribution/installation-scripts/install-neo4j-cli.sh` fails on macOS Tahoe (Darwin 25) because Apple ships a BSD-flavoured `/sbin/sha256sum` (`sha256sum (Darwin) 1.0`) that rejects GNU long flags. The script's first branch (`command -v sha256sum && sha256sum --check --status`) matches Apple's binary, prints `usage: sha256sum [-bctwz] [files ...]`, and exits non-zero — install bombs out with `Checksum verification FAILED`.

Older macOS had no `sha256sum`, so the script fell through to `shasum`. New macOS finds Apple's `sha256sum` first and the `shasum` fallback is unreachable.

Fix: make the verification step use POSIX-compatible flags that work on GNU coreutils, BSD/Darwin, and Perl `shasum`. Add a shellcheck CI gate so future portability bugs surface in PR review instead of after release.

## Goals

- Restore `curl -sSfL https://neo4j.sh/install.sh | bash` on macOS Tahoe.
- Keep working on Linux (GNU coreutils), older macOS (`shasum`), and WSL.
- Catch future install-script portability regressions in CI via shellcheck.
- Ship as a `Patch` release — user-visible bug fix, no new functionality.

## Non-Goals

- **No PowerShell installer changes.** `install-neo4j-cli.ps1` uses `Get-FileHash` and is unaffected.
- **No npm / pypi distribution changes.** Those channels do not invoke this script.
- **No hard-fail on missing checksum tool.** When neither `sha256sum` nor `shasum` is found, keep the existing `warn` + continue behaviour (matches pragmatic minimal-Linux-container scenarios).
- **No GPG / Sigstore signing.** SHA256 from the same release stays the trust anchor.
- **No GNU-vs-BSD detection.** The fix uses flags supported by both, so detection is unnecessary.
- **No bats/macOS-runner smoke test.** Considered and deferred — shellcheck covers the syntax-and-portability class of bug; a live install smoke test is a larger scope.

## Requirements

### Functional Requirements

- **REQ-F-001** — Replace the checksum verification block at `distribution/installation-scripts/install-neo4j-cli.sh:110-119` with a variant that uses only flags supported by both GNU coreutils `sha256sum` and Apple's BSD `sha256sum (Darwin) 1.0`:
  - `-c` (short form of `--check`) — supported by both.
  - Explicit `-` to read the checksum line from stdin — required by BSD, accepted by GNU.
  - Drop `--status` (GNU-only, used to suppress per-file `OK` lines); redirect stdout to `/dev/null` instead.
- **REQ-F-002** — Final block shape:
  ```bash
  if command -v sha256sum &>/dev/null; then
    grep "${ARCHIVE}" "${CHECKSUM_FILE}" | sha256sum -c - >/dev/null \
      || error "Checksum verification FAILED for ${ARCHIVE}"
  elif command -v shasum &>/dev/null; then
    grep "${ARCHIVE}" "${CHECKSUM_FILE}" | shasum -a 256 -c - >/dev/null \
      || error "Checksum verification FAILED for ${ARCHIVE}"
  else
    warn "No sha256sum or shasum found — skipping checksum verification"
  fi
  ```
- **REQ-F-003** — Behaviour on tampered archive: a hash mismatch must still cause `error "Checksum verification FAILED for ${ARCHIVE}"` (`set -euo pipefail` + the `||` guard). The script must exit non-zero before reaching the extract step.
- **REQ-F-004** — Behaviour when neither tool is present: keep the current `warn "No sha256sum or shasum found — skipping checksum verification"` and continue.
- **REQ-F-005** — Add a shellcheck CI job that lints `distribution/installation-scripts/install-neo4j-cli.sh`. Use the `ludeeus/action-shellcheck` (or equivalent stable shellcheck-action) with default severity (`style`) and severity gate set to `warning` so style noise does not block PRs but real bugs do. Wire it into `.github/workflows/test.yml` (or a new `.github/workflows/shellcheck.yml`) so it runs on every PR touching the script or the workflow file.
- **REQ-F-006** — The shellcheck job must pass on the patched script (after REQ-F-001/002). Resolve any pre-existing warnings the linter surfaces by either fixing them or annotating a justified `# shellcheck disable=SCxxxx` comment.
- **REQ-F-007** — Add a `Patch` changie entry: `changie new --projects neo4j-cli --kind Patch --body "Fix install script checksum verification on macOS Tahoe (Darwin 25)"`. If `changie` is unavailable locally, hand-author the YAML at `.changes/unreleased/neo4j-cli-Patch-<YYYYMMDD>-<HHMMSS>.yaml` per AGENTS.md "Changie Notes".

### Non-Functional Requirements

- **REQ-NF-001 (Portability)** — Verified against three environments before merge:
  1. Linux GNU coreutils `sha256sum` (CI ubuntu runner).
  2. macOS Tahoe with `/sbin/sha256sum` BSD variant (manual local run on the user's machine).
  3. macOS without `sha256sum`, falling through to `shasum` — simulate locally via `PATH=/usr/bin:/bin bash install-neo4j-cli.sh` (drops `/sbin/`).
- **REQ-NF-002 (Idempotent / quiet)** — On success, the script must print only `✔ Checksum OK` (existing `success` line). The previous `--status` was suppressing per-file `OK` output; the stdout redirect (`>/dev/null`) preserves that quiet-on-success contract.
- **REQ-NF-003 (No new dependencies)** — No additional runtime tools required. Existing `curl` / `tar` / `unzip` / `grep` / `sha256sum`-or-`shasum` set is unchanged.
- **REQ-NF-004 (CI cost)** — shellcheck job must complete in < 30s; do not add a macOS runner for this PR.

## Technical Considerations

### Why these specific flags

| Flag                 | GNU coreutils | BSD/Darwin `sha256sum` | macOS `shasum` (Perl) |
| -------------------- | ------------- | ---------------------- | --------------------- |
| `--check` (long)     | yes           | **no**                 | yes                   |
| `-c` (short)         | yes           | yes                    | yes                   |
| `--status`           | yes           | **no**                 | yes                   |
| `-s`                 | yes           | **no** (`-s` invalid)  | yes                   |
| Read stdin via `-`   | yes           | yes (required)         | yes                   |
| Output format on success | `<file>: OK` | `<file>: OK`         | `<file>: OK`          |

→ Lowest-common-denominator is `-c -` + stdout redirect.

### Why no GNU-vs-BSD detection

A user with Homebrew coreutils + `--with-default-names` would have `/usr/local/bin/sha256sum` (GNU) ahead of `/sbin/sha256sum` (BSD) on `PATH`. Both produce identical output on success; the `-c -` flag combination works for both. Detecting the variant adds branching for no behavioural difference.

### Why shellcheck (not bats / macOS smoke test)

shellcheck would have caught the original bug: SC2257 / SC2002-class warnings are not relevant here, but a `--check --status` on a BSD-only target is the kind of portability call the linter flags as a `warning` (e.g., SC2317 / SC2296 family). bats runs the script's logic but does not trigger Apple's `/sbin/sha256sum` unless run on a macOS Tahoe runner — costing a `macos-latest` job (~5x linux runner cost). A live install smoke test is in scope for a separate PRD if regressions recur.

### Files touched

- `distribution/installation-scripts/install-neo4j-cli.sh` — verification block at lines 110-119.
- `.github/workflows/shellcheck.yml` (new) **or** `.github/workflows/test.yml` (extended) — shellcheck job.
- `.changes/unreleased/neo4j-cli-Patch-*.yaml` — changie entry.

### Out-of-tree verification (manual, before tagging the PR ready)

```bash
# On macOS Tahoe (Darwin 25):
bash distribution/installation-scripts/install-neo4j-cli.sh
# Expect: ✔ Checksum OK ; Installed → /usr/local/bin/neo4j-cli

# Simulate "no sha256sum" path (pre-Tahoe macOS shape):
PATH=/usr/bin:/bin bash distribution/installation-scripts/install-neo4j-cli.sh
# Expect: shasum branch taken, ✔ Checksum OK
```

## Acceptance Criteria

- [ ] `curl -sSfL https://neo4j.sh/install.sh | VERSION=v0.1.0-alpha.10 bash` succeeds on macOS Tahoe (Darwin 25) and produces a working `/usr/local/bin/neo4j-cli`.
- [ ] Same command succeeds on linux/amd64 (verified in CI or local container).
- [ ] Same command succeeds with `PATH` scoped to `/usr/bin:/bin` (no `sha256sum` available, `shasum` path exercised).
- [ ] A tampered archive (intentionally corrupted in the temp dir between download and verify) causes the script to exit non-zero with the `Checksum verification FAILED` message, without proceeding to extract.
- [ ] shellcheck CI job runs on every PR and passes on `main`.
- [ ] `Patch` changie entry exists at `.changes/unreleased/neo4j-cli-Patch-*.yaml`.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check` all pass.

## Out of Scope

- PowerShell installer (`install-neo4j-cli.ps1`).
- npm / pypi installer channels.
- Hard-fail behaviour when no checksum tool is present.
- GPG / Sigstore signing.
- bats integration tests for the install script.
- macOS-runner end-to-end install smoke test.
- Refactoring the install script for readability beyond the targeted block.

## Open Questions

None — resolved:
- shellcheck severity gate: **`warning`** (REQ-F-005).
- shellcheck scope: **install script only**. `distribution/npm/` and `distribution/pypi/` shell scripts are out of scope; broader coverage is a follow-up if appetite exists.
