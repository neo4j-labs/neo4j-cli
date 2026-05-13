# Hermetic Test Notes

Gotchas for keeping tests deterministic and isolated from the host machine.

## Path & env

- For `~` / `$XDG_CONFIG_HOME` expansion tests, use `t.Setenv("HOME", "...")` and `t.Setenv("XDG_CONFIG_HOME", "")` — Go's `os.Getenv` returns `""` for both unset and set-to-empty, and `t.Setenv` auto-restores after the test.
- Use `afero.DirExists` (not `Exists`) for "is the agent installed?" checks — files at the marker path shouldn't count as detected.

## Output assertions

- `go-pretty/v6/table` upper-cases header text by default — compare against `strings.ToLower(...)` for header columns, exact case for body cells.

## Config wiring

- Lightweight cobra command tests can wire `clicfg.NewConfig(testfs.GetTestFs(...), version)` directly without `testutils.NewAuraTestHelper` — the latter pulls in API mocking and credential setup that `skill` doesn't need.

## Repo-walking gate tests

- For gate tests that auto-discover content (e.g. `common/skill/bundles_test.go` walking every `<bin>/internal/skill/bundle/SKILL.md`), resolve repo root via `runtime.Caller(0)` then `filepath.Walk` from there. Suffix-match paths after `filepath.ToSlash` so Windows runs match. Prune `.git`, `node_modules`, `bin`, `.changes` to keep the walk fast.

## File perms

- `os.OpenFile(..., 0o644)` mode bits are masked by umask on create — if downstream readers (e.g. a docker container running as a different uid) need a specific perm, follow up with `os.Chmod`. Same for `t.TempDir()` (creates with 0700); a read-only bind mount into a container needs `os.Chmod(dir, 0o755)` for the in-container user to traverse.

## TTY seams (query package)

- Package-level seams (e.g. `stdinIsTTY` at `neo4j-cli/query/run.go`, `stdoutIsTerminal` at `neo4j-cli/query/output.go`) are `var <name> = func(...) ...` declarations production fills with the real impl. `TestMain` in `testseam_test.go` seeds the seam to the most-common assertion (TTY=true) so legacy tests stay green; tests that need the other branch use a `withX(t, val)` helper that swaps and registers `t.Cleanup` to restore.

## httptest cancellation propagation

- `httptest.NewServer` server-side `r.Context().Done()` propagation from a closed client connection is best-effort and timing-dependent. When testing client ctx-cancellation paths, ALWAYS:
  - Guard the handler with a short safety timeout: `select { case <-r.Context().Done(): case <-time.After(2*time.Second): }`
  - Wrap the test-side wait on `errCh` in a `select` with a 5s fallback `t.Fatal`.
- Without these, a propagation miss hangs until `-test.timeout` (default 10m), looking like an infinite loop. Symptom: `pkill -QUIT <pid>` shows the handler's goroutine blocked on `chanrecv` at `<-r.Context().Done()` while the client side has long returned.

## Update-check e2e

- Tier-1 e2e for `update check` uses the `e2e_seams` build tag + `test/e2e/release_fixture` to cover both `channel: stable` and `channel: pre-release` deterministically (two scenarios, sequential restarts). A sibling `Update e2e (schema-only live smoke)` step pipes real-api.github.com output through `check_json --schema-only` as a calendar-immune contract canary.

## Cobra completion injection

- Cobra lazily injects its built-in `completion` subcommand (and four children) on the FIRST `Execute()` call via `InitDefaultCompletionCmd`. Tests that walk the live cobra tree AND a post-execute artifact (e.g. `agent-context` JSON, skill bundle) must build ONE tree, run `Execute()`, then walk THAT same instance — building a fresh `app.NewCmd` for the walk and a separate one for Execute yields a phantom diff of `[completion, completion bash, completion fish, completion powershell, completion zsh]`.
