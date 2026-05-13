# PRD: Fix Parallel OAuth Token Refresh Race Condition

## Overview

When multiple `neo4j-cli aura` commands run concurrently (e.g. in a CI script), they all share a single `credentials.json` file. If the stored OAuth access token is expired, every parallel process detects that independently and each tries to atomically write a refreshed token back to disk. Because all processes use the same temp file path (`credentials.json.tmp`), their writes collide: one process renames the tmp file away while another is mid-write, producing a `rename .../credentials.json.tmp .../credentials.json: no such file or directory` panic.

This PRD covers the fix: serialize cross-process writes to the credentials file using an advisory lock, and add a disk re-read in the token-refresh path so a process that waited for the lock can skip a redundant HTTP call when the winner has already written a fresh token.

## Goals

- Eliminate the `rename ... no such file or directory` panic when parallel CLI invocations each trigger a token refresh.
- Reduce redundant OAuth HTTP calls: a process that waited for the lock should use the fresh token already on disk rather than making its own refresh request.
- Keep the fix transparent to all existing callers of `fileutils.WriteFile` — no call-site changes outside the credentials layer.
- Introduce no new module dependencies (use the existing direct `golang.org/x/sys` dependency).

## Non-Goals

- In-process goroutine synchronization (each CLI invocation is a separate OS process; a `sync.Mutex` would not help).
- Preventing all redundant OAuth HTTP calls (two processes that arrive at the expired-token check simultaneously will both proceed to HTTP; only the second waiter — held at the lock — benefits from the re-read optimization).
- Changing how tokens are validated or stored (format, expiry logic, etc.).

## Root Cause

**Shared temp file path + no inter-process lock.** `fileutils.writeFileAtomic` always names the temp file `<path>.tmp` (hardcoded at [fileutils.go:65](common/clicfg/fileutils/fileutils.go#L65)). The sequence that produces the error:

1. Processes A and B both call `getToken` ([token.go:20-78](neo4j-cli/aura/internal/api/token.go#L20-L78)); both see `HasValidAccessToken() == false`.
2. Both make the OAuth HTTP request and receive a fresh `(accessToken, expiresIn)`.
3. Both call `UpdateAccessToken` → `onUpdate` → `save` → `fileutils.WriteFile`.
4. Both open `credentials.json.tmp` with `O_CREATE|O_WRONLY|O_TRUNC`. One truncates the other's data.
5. Both call `fs.Rename("credentials.json.tmp", "credentials.json")`.
6. Process A's rename succeeds; the `.tmp` file is now gone.
7. Process B's rename fails: `no such file or directory`.

## Requirements

### Functional Requirements

- REQ-F-001: `fileutils.WriteFile` must acquire an exclusive advisory lock on a sibling lock file (`<path>.lock`) before opening the temp file, and release it after the rename (or after cleanup on failure).
- REQ-F-002: The lock acquisition must be cross-platform: `unix.Flock(LOCK_EX)` on Linux/macOS, `windows.LockFileEx` on Windows. Build-tag-separated implementation files in `common/clicfg/fileutils/`.
- REQ-F-003: The lock file (`credentials.json.lock`) must be created if absent and left in place on disk after the operation (never deleted mid-run, to avoid a race between lock creation and acquisition).
- REQ-F-004: `getToken` must re-read `credentials.json` from disk after the in-memory `HasValidAccessToken()` check fails, before making the OAuth HTTP request. If the fresh disk state shows a valid token for the credential, `getToken` must return that token without making an HTTP call.
- REQ-F-005: The re-read in `getToken` must be a best-effort path: if the disk read or parse fails, `getToken` falls through to the existing HTTP refresh path (no new error surface).

### Non-Functional Requirements

- REQ-NF-001: Lock acquisition must be blocking (not spin-polling) so parallel processes queue efficiently without busy-waiting.
- REQ-NF-002: Lock acquisition timeout: no hard cap; rely on OS scheduler. Document that a stale lock (process killed while holding it) is released automatically by the OS when the file descriptor is closed.
- REQ-NF-003: All existing `fileutils_test.go` tests must continue to pass. The `afero.MemMapFs` used in tests does not support `Flock`; the lock implementation must short-circuit gracefully when `Flock` returns `ENOSYS` or when the underlying fs is not an `OsFs`.
- REQ-NF-004: No new entries in `go.mod` — use only `golang.org/x/sys/unix` (Unix) and `golang.org/x/sys/windows` (Windows), both already direct dependencies.

## Technical Considerations

### Lock file implementation

Create two build-tagged files in `common/clicfg/fileutils/`:

- `lock_unix.go` (`//go:build !windows`) — opens `<path>.lock` with `O_CREATE|O_RDWR`, calls `unix.Flock(fd, unix.LOCK_EX)`, returns the `*os.File` and a release func.
- `lock_windows.go` (`//go:build windows`) — opens `<path>.lock`, calls `windows.LockFileEx` with `LOCKFILE_EXCLUSIVE_LOCK`.

`writeFileAtomic` calls `acquireLock(path)`, defers `release()`, then proceeds with the existing open/write/sync/close/rename sequence.

### MemMapFs / test compatibility

`afero.MemMapFs` does not implement `Fd()` and returns an error-implementing non-nil `File`. The lock helpers must check whether the underlying `afero.Fs` is `*afero.OsFs` (or wraps one); if not, skip locking. Alternatively, open the lock file via `os.OpenFile` directly (bypassing afero) since the lock file is a coordination artifact, not application data.

The simplest approach: `acquireLock` receives the *real* path string (already available as `path` in `writeFileAtomic`), opens the lock file via `os.OpenFile` (not afero), and calls the OS locking syscall on the resulting `*os.File`. This means lock files are always on the real filesystem even when afero wraps a MemMapFs — which is correct for cross-process coordination.

### Re-read in getToken

Add a package-private helper `reloadAuraCredential(fs afero.Fs, filePath, name string) (*AuraCredential, bool)` in `common/clicfg/credentials/` that reads and parses `credentials.json` from disk and returns the named credential. Call it in `getToken` between the `HasValidAccessToken()` guard and the HTTP request block:

```go
func getToken(credential *credentials.AuraCredential, cfg *clicfg.Config) (string, error) {
    if credential.HasValidAccessToken() {
        return credential.AccessToken, nil
    }
    // Re-read from disk; another parallel process may have already refreshed.
    if fresh, ok := credentials.ReloadAuraCredential(cfg.Aura.Fs(), cfg.Aura.CredentialsFilePath(), credential.Name); ok && fresh.HasValidAccessToken() {
        cfg.Credentials.Aura.SyncFromDisk(fresh) // update in-memory state
        return fresh.AccessToken, nil
    }
    // ... existing HTTP refresh path ...
}
```

The `SyncFromDisk` call (or equivalent) updates the in-memory `AuraCredential` so subsequent calls within the same process also see the fresh token.

### Relationship to existing save() flow

The lock is held only during `fileutils.WriteFile`. The `onUpdate → save()` call chain is unchanged. Because `save()` calls `fileutils.WriteFile`, all credential mutations (add, remove, set-default, update-token) automatically benefit from the lock with no additional call-site changes.

### Windows atomic rename

`os.Rename` on Windows uses `MoveFileEx(MOVEFILE_REPLACE_EXISTING)`, which is atomic for same-volume moves. Combined with `LockFileEx`, the Windows path is safe.

## Acceptance Criteria

- [ ] `make test` passes on all platforms (Linux, macOS, Windows).
- [ ] `make lint` and `make fmt-check` are clean.
- [ ] A new test in `fileutils_test.go` (or a colocated `race_test.go`) verifies that two goroutines calling `WriteFile` on the same path concurrently produce a valid final file (no panic, last write wins, content is valid JSON).
- [ ] A new test verifies `getToken` returns the on-disk token without an HTTP call when another "process" (simulated by writing a fresh credential to disk before calling `getToken`) has already refreshed it.
- [ ] Running `make build && bin/neo4j-cli aura instance list & bin/neo4j-cli aura instance list & wait` with an expired token no longer panics.
- [ ] No new entries appear in `go.mod` / `go.sum` beyond `golang.org/x/sys` sub-packages already present.
- [ ] `make license-check` passes (new `.go` files carry the Neo4j copyright header).

## Out of Scope

- Locking around reads (only writes need serialization for this bug).
- Migrating existing lock-less tests to use a real filesystem — MemMapFs tests remain in place; the lock bypasses afero for the lock file itself.
- Rate-limiting or deduplicating concurrent OAuth HTTP calls at the network level.
- Any change to token expiry logic, token format, or OAuth credential model.

## Open Questions

- Should `SyncFromDisk` (updating in-memory state from a fresh disk read) be exposed at the `Credentials` level or only at the `AuraCredentials` level? The re-read optimization is currently only needed for Aura tokens, but Dbms/Embed credentials could theoretically benefit from a similar pattern in the future.
- Should the lock file path be configurable (e.g. for testing with a custom prefix), or is hardcoding `<path>.lock` sufficient?
- The re-read helper needs access to `cfg.Aura.Fs()` and the credentials file path. Verify these are already exposed on `Config`/`AuraConfig`, or add accessors as needed.
