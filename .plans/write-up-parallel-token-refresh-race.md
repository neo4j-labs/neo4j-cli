# Parallel OAuth Token Refresh Race Condition — Investigation & Fix

## The Report

Running several read-only `neo4j-cli aura` commands in parallel would occasionally fail with:

```
rename .../credentials.json.tmp .../credentials.json: no such file or directory
```

Running the same commands sequentially worked fine. The error only surfaced when a cached OAuth access token happened to be expired at the moment multiple processes started.

---

## Root Cause

Every `neo4j-cli aura` command is a separate OS process. Each process loads `credentials.json` into memory at startup, makes its API calls, and — when it needs a fresh OAuth token — writes the updated token back to disk.

The write path was:

```
UpdateAccessToken() → onUpdate() → save() → fileutils.WriteFile()
```

`WriteFile` uses an atomic temp-file-then-rename pattern:

```
open credentials.json.tmp (O_CREATE|O_WRONLY|O_TRUNC)
write → sync → close
rename credentials.json.tmp → credentials.json
```

The problem: **all processes share the same temp file path** (`credentials.json.tmp`). When two processes race to refresh an expired token the sequence becomes:

| Process A | Process B |
|-----------|-----------|
| Detects expired token | Detects expired token |
| HTTP refresh → gets token | HTTP refresh → gets token |
| Opens `credentials.json.tmp` (O_TRUNC) | |
| Writes token A | |
| | Opens `credentials.json.tmp` (O_TRUNC) — **truncates A's data** |
| | Writes token B |
| `rename .tmp → credentials.json` ✓ — **tmp is now gone** | |
| | `rename .tmp → credentials.json` ✗ — **no such file** → panic |

There was no inter-process lock anywhere in the write path.

---

## Fix

The fix has two parts: **serialize writes with a file lock** and **avoid redundant HTTP calls with a pre-lock disk re-read**.

### Part 1 — Lock → read → merge → write

`credentials.save()` now wraps every write in a `gofslock` exclusive lock on a sidecar `credentials.json.lock` file:

```
acquire exclusive lock on credentials.json.lock (10 s timeout)
  read current on-disk state
  merge in-memory state onto disk state
  write atomically (temp + rename) while holding the lock
release lock
```

Using a **sidecar lock file** (not the JSON file itself) avoids any risk of the lock mechanism truncating or corrupting application data. The OS releases the lock automatically when the file descriptor is closed, so a process killed mid-operation never leaves a permanent lock.

`fileutils.WriteFile` itself is unchanged — the temp-plus-rename pattern is preserved for atomicity, and the lock sits one level higher at the credentials layer where it belongs.

#### Merge rules

Wrapping the write in a lock means we can also read the current disk state first and incorporate any changes made by parallel processes:

- **In-memory is authoritative for the credential list.** If a credential was added or removed in this process, that is the intended final state. Disk-only entries (added by a parallel process) are not preserved — for a CLI, concurrent credential management is not a realistic scenario.
- **For Aura access tokens, whichever copy has the later expiry wins.** If a parallel process already wrote a fresher token, it is kept rather than overwritten with a stale in-memory value.
- **An explicitly cleared token is never restored from disk.** When a 401 or 403 response causes `ClearAccessToken` to set the token to `""`, the merge does not replace it with the non-empty value still on disk.

### Part 2 — Pre-lock disk re-read in `getToken`

Before making an OAuth HTTP request, `getToken` now re-reads `credentials.json` from disk:

```go
func getToken(credential *AuraCredential, cfg *Config) (string, error) {
    if credential.HasValidAccessToken() {
        return credential.AccessToken, nil  // fast path: in-memory is fresh
    }

    // Re-read disk; a parallel process may have already refreshed.
    if fresh, ok := credentials.ReloadAuraCredential(...); ok && fresh.HasValidAccessToken() {
        cfg.Credentials.Aura.SyncCredential(fresh)
        return fresh.AccessToken, nil       // use the on-disk token
    }

    // Slow path: make the HTTP request.
    ...
}
```

This is best-effort and lock-free. If the disk read fails or the on-disk token is also expired, execution falls through to the HTTP call unchanged. The benefit: a process that was queued behind the lock holder can skip its own network round-trip when it finds the fresh token already written.

---

## Package choice: `gofslock`

The initial implementation used raw `unix.Flock` / `windows.LockFileEx` from `golang.org/x/sys` with custom build-tagged files. This was replaced with [`github.com/danjacques/gofslock`](https://github.com/danjacques/gofslock) for three reasons:

1. **Cross-platform without boilerplate.** `gofslock` handles the `flock(2)` vs `LockFileEx` split internally; no build-tagged files needed in this repo.
2. **`WithBlocking` + `Blocker` pattern.** The `Blocker` callback is called between retry attempts, making a timeout straightforward to implement without a goroutine or context:

   ```go
   deadline := time.Now().Add(10 * time.Second)
   blocker := func() error {
       if time.Now().After(deadline) {
           return fmt.Errorf("timed out after 10s")
       }
       time.Sleep(5 * time.Millisecond)
       return nil
   }
   fslock.WithBlocking(lockPath, blocker, func() error { ... })
   ```

3. **No new transitive dependencies.** `golang.org/x/sys` was already a direct dependency; `gofslock` brings nothing new to `go.sum`.

---

## Files changed

| File | Change |
|------|--------|
| `common/clicfg/fileutils/lock_unix.go` | **Deleted** — replaced by gofslock |
| `common/clicfg/fileutils/lock_windows.go` | **Deleted** — replaced by gofslock |
| `common/clicfg/fileutils/fileutils.go` | Reverted to original (no lock in `writeFileAtomic`) |
| `common/clicfg/credentials/credentials.go` | `save()` rewritten with lock + merge; `readDisk()` added; `FilePath()` accessor added |
| `common/clicfg/credentials/merge.go` | **New** — `mergeCredentialsFile` and `mergeAura` |
| `common/clicfg/credentials/reload.go` | **New** — `ReloadAuraCredential` and `SyncCredential` |
| `common/clicfg/credentials/reload_test.go` | **New** — tests for reload helpers |
| `neo4j-cli/aura/internal/api/token.go` | Pre-lock disk re-read added before HTTP call |
| `neo4j-cli/aura/internal/api/token_test.go` | **New** — `TestGetToken_DiskReread` |
| `go.mod` / `go.sum` | `gofslock` added |
| `.gitignore` | `*.lock` added (advisory lock files from `make test`) |
