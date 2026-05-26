# PRD: CLI-160 — Validate `docker create --version` against charset allowlist

## Overview

`neo4j-cli docker create --version <value>` flows directly into the docker
image tag at `neo4j-cli/internal/subcommands/docker/create.go:260-267`
(`"neo4j:" + version`, with `-enterprise` appended in the enterprise branch).
There is currently no CLI-side validation: malformed input surfaces as
Docker's generic "invalid reference format" rather than a pointed CLI error,
and any future loosening of Docker's reference parser (cross-version variance,
podman wrappers) would silently weaken the CLI.

Source: Oplane REQ-00065573 ("Image Tag Validation for Docker Pulls"),
parent CLI-159, threat model `df83039f-22b1-4874-9173-69d6bf248b69`. Graded
**Critical / PARTIALLY_IMPLEMENTED**. Practical impact is bounded by Docker's
reference parser (rejects `/`, `@`, second `:`, whitespace), so this is
defence-in-depth — but the cost is one regex + one validator + one test
table.

Linear:
[CLI-160](https://linear.app/neo4j/issue/CLI-160/docker-create-validate-version-against-charset-allowlist-before-image).

## Goals

- Reject malformed `--version` input at the CLI boundary with a clear error
  naming the expected format and the bad input; no Docker side effects
  before the check.
- Preserve correctness for every published `neo4j` tag the existing code
  already supports: `latest`, semver (`5`, `5.20`, `5.20.0`), calver
  (`2026.04`), and the redundant `-enterprise` suffix on a version (e.g.
  `5.20-enterprise`).
- Keep the validator small, single-purpose, and colocated with `create.go`
  — no new package, no new dependency.
- Mirror existing in-file patterns: `clierr.NewUsageError` error shape and
  the package-level precompiled-regex idiom already used elsewhere in the
  repo (e.g. `common/skill/installer.go:32`).

## Non-Goals

- Validating `--name`, `--bolt-port`, `--http-port`, `--password`, or any
  other `create` flag. Scope is `--version` only.
- Validating `--edition` (already enum-checked at `create.go:169-171`).
- Adding a `--registry` flag or otherwise loosening the hardcoded `neo4j`
  image name. Image name stays hardcoded.
- Changing Docker image-tag construction logic at `create.go:260-267` —
  only the input feeding into it.
- Cross-validating `--version` against `--edition` beyond the
  `-enterprise`-suffix strip described in REQ-F-004.
- Bundle-content changes: `--version` flag Long stays as-is, so no bundle
  regen is expected (will be verified via `make generate-check`).

## Requirements

### Functional Requirements

- **REQ-F-001**: A package-private validator `validateVersion(version string) (string, error)`
  MUST be added to `neo4j-cli/internal/subcommands/docker/create.go`,
  near the other `resolve*` / `expand*` helpers at the bottom of the
  file.
- **REQ-F-002**: The validator MUST use a package-level precompiled regex
  (`regexp.MustCompile`) named `versionPattern`, declared near the other
  package-level seams (`homeDirFn`, `clientFactory`, `randSource`,
  `listenerFactory`, `waitForBoltFn`). Pattern:
  `^[0-9]+(\.[0-9]+)*(-enterprise)?$|^latest$`.
- **REQ-F-003**: The validator MUST trim leading/trailing whitespace
  (`strings.TrimSpace`) before regex match. The trimmed value flows
  downstream unchanged in every other respect.
- **REQ-F-004**: After regex match, the validator MUST strip any trailing
  `-enterprise` suffix (`strings.TrimSuffix(trimmed, "-enterprise")`).
  Rationale: the existing image-construction block at `create.go:265`
  appends `-enterprise` when `--edition enterprise`, so a user passing
  `--version 5.20-enterprise --edition enterprise` would otherwise yield
  `neo4j:5.20-enterprise-enterprise` (unpublished tag, broken pull).
  Stripping during validate makes the suffix harmless in both editions:
  - `--version 5.20-enterprise --edition enterprise` → `neo4j:5.20-enterprise`
  - `--version 5.20-enterprise --edition community` → `neo4j:5.20`
- **REQ-F-005**: On regex miss, the validator MUST return a
  `clierr.NewUsageError` whose message names both the expected format
  (`"must match digits/dots with optional -enterprise suffix (e.g. 5.20, 5.20.0, 5.20-enterprise, latest)"`)
  and the unedited bad input (the value before TrimSpace, so the operator
  sees exactly what they passed). Mirrors the shape of the existing
  `--edition` usage error at `create.go:170`.
- **REQ-F-006**: The validator MUST be called from the `create` cobra
  `RunE` immediately after the `--edition` enum guard (around
  `create.go:171`) and BEFORE the `--env-out-file` / `--ephemeral` /
  volume / port pre-flights. Rationale: fail fast on bad input; do not
  burn listener probes or fs stat calls on a doomed invocation.
- **REQ-F-007**: The trimmed-and-stripped return value MUST be assigned
  back into the outer `version` variable so the existing image-construction
  block (`create.go:260-267`), the `LabelVersion` label (`create.go:309`),
  and the output row (`create.go:378`) all see the canonical form. No
  other lines change.
- **REQ-F-008**: On validation failure, `client.Run` MUST NOT be invoked —
  asserted by `fake.RunCalls` being empty in the failure test cases
  (mirrors the assertion shape of `TestCreate_InvalidEdition_ReturnsUsageError`
  at `create_test.go:341-346`).

### Non-Functional Requirements

- **REQ-NF-001**: `make test`, `make fmt-check`, and `make lint` MUST be
  clean (the AGENTS.md final-gate rule).
- **REQ-NF-002**: `make generate-check` MUST be clean — the change does
  not modify any flag Long, so no bundle regen is expected. If
  `TestGenerator_RoundTrip` fires, that is a signal to widen the change
  (it should not, but verify).
- **REQ-NF-003**: A changelog entry MUST be added via
  `changie new --projects neo4j-cli --kind Patch --body "docker create: validate --version against allowlist (CLI-160)"`
  (or hand-authored YAML under `.changes/unreleased/` per the AGENTS.md
  Changie Notes when changie is not installed locally).
- **REQ-NF-004**: No new `.go` file is needed — the validator lives inline
  in `create.go`, so the existing copyright header at the top of the file
  satisfies `make license-check`.
- **REQ-NF-005**: No new external Go dependency. `regexp` is in the
  standard library; `strings` and `clierr` are already imported.

## Technical Considerations

- **Why strip `-enterprise` instead of rejecting it.** Confirmed
  interactively before PRD generation: stripping makes the suffix
  harmless in both editions and matches operator intent ("I want
  enterprise 5.20"). Rejecting would force the operator to drop the
  suffix manually, hide a pre-existing image-construction interaction
  rather than fix it, and diverge from the Oplane spec which explicitly
  lists `5.20-enterprise` as an accept case.

- **Whitespace handling.** Confirmed interactively: silent trim. The
  trimmed value is what flows downstream, so an operator who passes
  `--version " 5.20 "` sees `5.20` in the docker run argv, the
  `org.neo4j.cli.version` label, the dbms credential metadata, and the
  rendered output row. No surprise asymmetry.

- **Calver coverage.** The existing image-construction comment at
  `create.go:257` already lists `2026.04` as a valid published tag. The
  proposed regex accepts arbitrary-length digit-dot sequences
  (`[0-9]+(\.[0-9]+)*`), so calver is covered without a special case.

- **Pre-existing `-community` suffix path.** Docker Hub does NOT publish
  `neo4j:<version>-community` tags (community is the default, no suffix).
  The proposed regex rejects `-community` as it would any unknown suffix;
  the resulting CLI error explicitly names the accepted shape so the
  operator can self-correct.

- **Where the regex lives.** Package level, precompiled once via
  `regexp.MustCompile`. Pattern (`common/skill/installer.go:32`,
  `common/analytics/events.go:75`). No regex compilation in a hot path.

- **Test seams already in place.** `create_test.go` has the
  `runCreate` / `runCreateWithOccupiedPorts` helpers, a stubbed
  `clientFactory` (`fakeDockerClient`), and the `stubListenerFactory`
  seam. The new tests do NOT need new seams — they reuse `runCreate`
  and assert via `fake.RunCalls` (empty on reject, populated with the
  expected image string on accept).

- **No fuzz target needed.** The validator is a single-line regex match
  with two pre/post string operations. A table-driven test with
  representative reject and accept cases is sufficient. Fuzz would add
  CI weight without finding anything the table doesn't.

## Acceptance Criteria

- [ ] `versionPattern` (precompiled regex) and `validateVersion(version string) (string, error)`
  exist in `neo4j-cli/internal/subcommands/docker/create.go`.
- [ ] `validateVersion` is called in `RunE` immediately after the
  `--edition` guard and before any other pre-flight (env-out-file,
  ephemeral, volume, port, name).
- [ ] On regex match, the function returns the input with TrimSpace and
  the `-enterprise` suffix stripped; the caller assigns the result
  back into the outer `version` variable.
- [ ] On regex miss, the function returns a `clierr.UsageError` whose
  message names both the expected format and the original (untrimmed)
  bad input; `client.Run` is NOT invoked.
- [ ] `TestCreate_VersionValidation` (table-driven, in
  `neo4j-cli/internal/subcommands/docker/create_test.go`) covers:
  - **reject**: `evilregistry.com/neo4j:latest`, `4.4:malicious`,
    `4.4@sha256:deadbeef`, `evil.com/neo4j@sha256:deadbeef`,
    `5.1.2$enterprise`, empty string, `5..20`, `5.20-community`.
  - **accept**: `latest` (default), `5`, `5.1.2`, `5.20-enterprise` +
    `--edition enterprise` (asserts image is `neo4j:5.20-enterprise`,
    not `neo4j:5.20-enterprise-enterprise`), `5.20-enterprise` +
    `--edition community` (asserts image is `neo4j:5.20`), `2026.04`,
    `  5.1.2  ` (asserts label payload + output row = `5.1.2`).
  - **integration**: at least one accept case asserts the docker run
    argv last token is `neo4j:<expected>` to confirm the validated
    value flows to `docker.io/library/neo4j` only.
- [ ] `TestCreate_InvalidEdition_ReturnsUsageError` is unchanged
  (regression guard for the existing `--edition` check).
- [ ] Manual smoke (not committed): `./bin/neo4j-cli docker create --name dev --version 'evil.com/neo4j' --rw`
  exits 2 with a usage error naming the expected format and the bad
  input; no stack trace; no docker side effect.
- [ ] Changelog entry exists under `.changes/unreleased/` with
  `kind: Patch`, project `neo4j-cli`, body referencing CLI-160.
- [ ] `make fmt-check`, `make lint`, `make test`, `make generate-check`
  are all clean.

## Out of Scope

- All other Oplane gaps tracked under CLI-159 (`--no-print-password`
  flag, docker stderr redaction, system bind-mount refuse/warn, TLS
  Bolt probe constant, multi-tenant name namespace help text, port
  TOCTOU). Each has or will have its own sub-issue.
- Adding a `--registry` flag, supporting non-`neo4j` images, or
  loosening the hardcoded image name.
- Rewriting the image-tag construction block at `create.go:260-267`.
- Fuzz-testing the validator.
- Changing the `--version` flag's help text or Long string (would
  trigger a bundle regen; deliberately avoided to keep the change
  surgical).

## Open Questions

None. The two design decisions that could have gone either way
(`-enterprise` suffix → strip vs reject; whitespace → trim vs reject)
were resolved interactively before PRD generation: **strip** the
suffix; **trim** whitespace silently.
