# PRD: CLI-162 — Redact docker's own stderr in error messages

## Overview

`neo4j-cli/internal/subcommands/docker/client.go:141-152` wraps every
non-zero docker exit in a `clierr.UsageError`. The argv echo passes
through `redactArgs` (masks `AUTH=` / `PASSWORD=` env values), but
docker's own captured stderr (`msg`) is interpolated verbatim into the
user-facing error string:

```go
if err := cmd.Run(); err != nil {
    msg := strings.TrimSpace(stderr.String())
    if msg == "" {
        msg = err.Error()
    }
    return "", clierr.NewUsageError(
        "docker %s: %s",
        strings.Join(redactArgs(args), " "),
        msg, // not redacted
    )
}
```

Refactor `redactArgs` to delegate to a new `redactString(s string) string`
helper backed by a single regex, then apply `redactString` to BOTH the
argv echo AND the stderr blob.

Source: Oplane REQ-00065578 ("Stderr Redaction Coverage for Credential
Leakage"), parent CLI-159, threat model
`df83039f-22b1-4874-9173-69d6bf248b69`. Graded **High /
PARTIALLY_IMPLEMENTED**. Practical leak surface today is narrow —
docker's stderr on common errors (port conflict, name collision,
image-not-found) does not echo `-e VAR=value` argv — but third-party
wrappers (`alias docker=podman`, lazydocker) format errors differently,
and a future docker release could echo argv-with-env on failure. The
cost of closing the gap is tiny; the defence-in-depth value is real.

Linear:
[CLI-162](https://linear.app/neo4j/issue/CLI-162/docker-redact-dockers-own-stderr-in-error-messages-not-just-argv-echo).

## Goals

- Apply the same `AUTH=` / `PASSWORD=` redaction to docker's captured
  stderr that today only applies to the argv echo.
- Consolidate redaction into a single regex-backed helper
  (`redactString`) so the two surfaces (argv slice + stderr blob)
  share one source of truth — future tightening edits one regex, not
  two algorithms.
- Preserve every current `redactArgs` test case verbatim (no
  observable change for the argv-echo path).
- Keep the change surgical: one helper file (`client.go`), one test
  file (`client_test.go`), one changelog entry. No new files. No
  bundle regen (no flag-Long change).

## Non-Goals

- Redacting credentials embedded in filesystem paths (e.g.
  `/tmp/aws_secret_abcd1234.txt`). `neo4j-cli` does not construct such
  paths today; speculative redaction would be churn without a use
  case. Explicitly out of scope per the ticket.
- Adding `SECURITY.md` regex-review documentation — tracked
  separately in CLI-158.
- Touching other Oplane gaps from the parent CLI-159 scan
  (`--no-print-password` flag, system bind-mount refuse/warn, TLS
  Bolt probe constant, port TOCTOU, multi-tenant name namespace help
  text). Each has or will have its own sub-issue.
- Changing argv-construction at `exec.CommandContext` — the argv
  passed to exec is untouched; redaction is only applied to the
  user-facing error string (existing invariant, preserved).
- Renaming `redactArgs` — keeps blame and call-site stability;
  internal delegation only.

## Requirements

### Functional Requirements

- **REQ-F-001**: A package-level precompiled regex named
  `sensitiveAssignmentRe` MUST be added to
  `neo4j-cli/internal/subcommands/docker/client.go`. Pattern (Go RE2):
  `(?i)(\w*(?:AUTH|PASSWORD)\w*\s*=\s*)(\S+)`. Declared near the
  existing `lookPathFn` / `ErrNotFound` package-level seams.
- **REQ-F-002**: A package-private helper
  `redactString(s string) string` MUST be added to `client.go`,
  implementation:
  `return sensitiveAssignmentRe.ReplaceAllString(s, "${1}<redacted>")`.
  Documented inline as the single source of truth for credential
  redaction across argv echoes and stderr blobs.
- **REQ-F-003**: The existing `redactArgs(args []string) []string`
  body MUST be rewritten to delegate to `redactString` per element.
  The nil/empty short-circuit (returns nil for nil input, empty slice
  for empty input) MUST be preserved. The non-mutation contract MUST
  be preserved (existing test asserts via `inCopy` snapshot).
- **REQ-F-004**: At `client.go:151` the `msg` interpolation MUST be
  changed from `msg` to `redactString(msg)`. The argv-echo branch
  (`strings.Join(redactArgs(args), " ")`) MUST be unchanged in shape;
  observable output remains identical for every input that didn't
  previously match the LHS-substring check.
- **REQ-F-005**: A new table-driven test
  `TestRedactString` MUST be added to
  `neo4j-cli/internal/subcommands/docker/client_test.go` covering the
  Oplane verification subset:
  - Single-line stderr with `NEO4J_AUTH=neo4j/hunter2` → value
    replaced with `<redacted>`, surrounding text intact.
  - Multi-line blob with two `PASSWORD=` mentions on separate lines
    → both replaced (asserts `ReplaceAllString` walks all matches,
    not just the first).
  - Unicode value: `NEO4J_AUTH=neo4j/密码1234` → value replaced
    even though it's non-ASCII (asserts `\S` matches non-whitespace
    runes via RE2).
  - Mixed-case LHS with spaces around `=`: `Neo4j_Auth = secret xyz`
    → value replaced; documentary case for case-insensitive flag and
    `\s*=\s*` tolerance.
  - Operational-error string with no sensitive assignment (e.g.
    `"Cannot connect to the Docker daemon at unix:///var/run/docker.sock"`)
    → returned verbatim, no false-positive redaction.
  - Empty string → returned as empty string (no panic, no
    allocation surprise).
- **REQ-F-006**: The existing `TestRedactArgs` table cases MUST
  continue to pass without modification — proves the regex
  implementation is behaviour-compatible with the previous
  LHS-substring algorithm for every documented argv shape (auth env
  masked, license env preserved, lowercase auth masked, no-LHS
  `=oddly-shaped` preserved, label assignments preserved, empty,
  nil).
- **REQ-F-007**: The existing comment block on `redactArgs` (lines
  156-160) MUST be updated to note that it now delegates to
  `redactString` and that the LHS match is regex-driven and
  case-insensitive (the previous `strings.ToUpper` LHS-folding is
  no longer present in the code). The `redactString` helper MUST
  itself carry an explanatory comment describing the regex shape
  and noting the multi-match property used for stderr blobs.

### Non-Functional Requirements

- **REQ-NF-001**: `make test`, `make fmt-check`, and `make lint`
  MUST be clean — the AGENTS.md final-gate rule.
- **REQ-NF-002**: `make generate-check` MUST be clean. The change
  does not modify any flag Long or cobra-tree Short string, so no
  bundle regen is expected. If `TestGenerator_RoundTrip` fires, that
  is a signal to widen the change (it should not).
- **REQ-NF-003**: A changelog entry MUST be added via
  `changie new --projects neo4j-cli --kind Patch --body "docker: redact NEO4J_AUTH/PASSWORD values in error messages from docker stderr (CLI-162)"`
  (or hand-authored YAML under `.changes/unreleased/` per the
  AGENTS.md Changie Notes when changie is not installed locally).
- **REQ-NF-004**: No new `.go` file is needed — both the regex and
  the helpers live inline in `client.go`, so the existing copyright
  header at the top of the file satisfies `make license-check`.
- **REQ-NF-005**: No new external Go dependency. `regexp` is in the
  standard library; `strings`, `bufio`, `bytes`, `clierr` are already
  imported.
- **REQ-NF-006**: Redaction MUST be allocation-conservative on the
  happy path (no docker error). The redaction code is only reached
  on `cmd.Run` failure, so per-call overhead is irrelevant; the
  regex is compiled once at package init via `regexp.MustCompile` —
  no compile-on-call cost on the error path either.

## Technical Considerations

- **Why regex over the existing per-element LHS-substring scan.**
  The stderr-blob case needs to find sensitive assignments embedded
  inside an arbitrary multi-line text (e.g. `"Error 1: NEO4J_AUTH=x\nError 2: ..."`),
  not just at the start of a slice element. A single regex over the
  full string handles both surfaces uniformly. The previous
  `IndexByte('=')` + `strings.ToUpper` LHS check only works when the
  whole element is the assignment.

- **Behaviour-compat with `TestRedactArgs`.** Walked each existing
  case against the new regex before drafting the PRD:
  - `NEO4J_AUTH=neo4j/hunter2` — matches, value masked. ✓
  - `NEO4J_ACCEPT_LICENSE_AGREEMENT=eval` — LHS has no AUTH or
    PASSWORD substring, no match. ✓
  - `MY_PASSWORD=hunter2` — matches via `PASSWORD`. ✓
  - `neo4j_auth=neo4j/x` — `(?i)` flag handles lowercase. ✓
  - `=oddly-shaped` — `\w*(?:AUTH|PASSWORD)\w*` requires AUTH or
    PASSWORD in the LHS; no LHS letters → no match. ✓
  - `org.neo4j.cli.managed=true` — LHS has no AUTH/PASSWORD, no
    match. ✓
  - Empty slice / nil — short-circuited in `redactArgs` before the
    regex is consulted; semantics preserved. ✓

- **Regex coverage notes (from Oplane advice).**
  - `\w*(?:AUTH|PASSWORD)\w*` — LHS substring contains AUTH or
    PASSWORD anywhere; covers `NEO4J_AUTH`, `MYSQL_PASSWORD`,
    `aws_secret_password`.
  - `\s*=\s*` — tolerates spaces around `=` (stderr blobs from
    third-party tools may emit `KEY = value`).
  - `(\S+)` — value capture is greedy non-whitespace; Unicode-safe
    because RE2 `\S` matches any non-whitespace rune, not just
    ASCII.
  - `ReplaceAllString` walks every non-overlapping match, so
    multi-error stderr blobs are fully covered.

- **Where the helper lives.** Same file (`client.go`) as the only
  call site. Adding a separate `redact.go` file would split blame
  for a 10-line helper and force a second copyright header for no
  win.

- **Argv passed to `exec` is untouched.** Existing invariant: only
  the user-facing error string is redacted. Verified by reading
  `c.run` — `args` flows into `exec.CommandContext` BEFORE the
  redaction code path. Preserved.

- **Inline integration assertion vs end-to-end fake-exec test.**
  Skipping end-to-end. The existing pattern in `client_test.go` is
  helper-level table tests (`TestRedactArgs`,
  `TestClassifyInspectError`, `TestAltRuntimeHint_*`); composing
  `redactArgs(args)` + `redactString(msg)` at the call site is
  trivially correct by construction and would need a fake `exec.Cmd`
  seam that doesn't exist today. Adding one for a one-line
  composition would be more risk than the test catches.

- **Test seams already in place.** None needed. `redactString` is a
  pure function; the new test is identical in shape to the existing
  `TestRedactArgs` table.

## Acceptance Criteria

- [ ] `sensitiveAssignmentRe` (precompiled regex) and
  `redactString(s string) string` exist in
  `neo4j-cli/internal/subcommands/docker/client.go`.
- [ ] `redactArgs` body delegates per-element to `redactString`,
  preserves the nil/empty short-circuit, and preserves the
  non-mutation contract.
- [ ] At `client.go:151` the `msg` interpolation is
  `redactString(msg)` (not bare `msg`).
- [ ] `TestRedactString` exists in
  `neo4j-cli/internal/subcommands/docker/client_test.go` covering
  the single-line, multi-line, Unicode, mixed-case-with-spaces,
  operational-error-no-match, and empty-string cases.
- [ ] `TestRedactArgs` table cases pass without modification.
- [ ] `TestClassifyInspectError` and the `TestAltRuntimeHint_*` /
  `TestResolve_DockerMissing_*` tests remain unchanged
  (regression guard for the surrounding helpers).
- [ ] Changelog entry exists under `.changes/unreleased/` with
  `kind: Patch`, project `neo4j-cli`, body referencing CLI-162.
- [ ] `make fmt-check`, `make lint`, `make test`,
  `make generate-check` are all clean.
- [ ] Manual smoke (not committed): a deliberately invalid
  `bin/neo4j-cli docker create ... -e NEO4J_AUTH=neo4j/secret` that
  docker rejects post-arg-parse — confirm the resulting error
  string masks `NEO4J_AUTH=<redacted>` in BOTH the argv echo AND
  any docker-stderr mention. Skip if no docker daemon available;
  table tests cover the behaviour.

## Out of Scope

- Credentials embedded in filesystem paths (no such code paths
  today).
- `SECURITY.md` regex-review documentation — CLI-158.
- All other Oplane gaps from CLI-159 — separately tracked.
- Renaming `redactArgs` or moving it to its own file.
- Adding a fake-`exec.Cmd` seam to drive an end-to-end stderr-blob
  redaction assertion (composition is trivial; new seam not worth
  it).
- Changing the `redactArgs` external signature — kept as-is for
  call-site stability.

## Open Questions

None. Implementation shape, regex pattern, and test surface are all
specified in the Oplane advice excerpt embedded in the Linear ticket
and confirmed against existing repo patterns
(`prd-cli-160-docker-create-version-validation.md` is the sibling
PRD from the same Oplane scan).
