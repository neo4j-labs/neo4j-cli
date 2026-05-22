# PRD: CLI-158 — Add `SECURITY.md` documenting supply-chain trust root for `neo4j-cli update`

## Overview

`neo4j-cli update` is the highest-blast-radius code path in the binary: it
downloads a replacement binary and atomically swaps it for the running one,
optionally via `sudo`. The defences are already in place — host pinning to a
five-host GitHub allowlist (`neo4j-cli/internal/subcommands/update/swap.go`
~line 115), SHA256 verification of `…_checksums.txt` **before** archive
extraction (~line 410), tar/zip-slip guards (~line 795), strict sudo argv
validation (~line 301), and a doc-comment contract that GitHub tokens MUST
NOT appear in error messages (`release.go` ~line 201). **None of this surfaces
to end users.** A user installing the CLI has no way to learn the trust root
of `update` or its accepted residual risks.

This PRD adds a user-facing `SECURITY.md` at repo root closing that
communication gap (REQ-00065544 from the Oplane parent threat model
[`d8cadbc9-…`]), wires a README link from the Installation section, and
adds a one-sentence pointer to `update --help` Long text. No defences
change.

Source: Oplane REQ-00065544, parent threat model on reviewed commit
`9829cec`. Severity Info / transparency.

Linear:
[CLI-158](https://linear.app/neo4j/issue/CLI-158/docs-add-securitymd-documenting-supply-chain-trust-root-for-neo4j-cli),
parent [CLI-155](https://linear.app/neo4j/issue/CLI-155/oplane-scan-for-neo4j-cli-update-action-items).

## Goals

- Document the supply-chain trust root of `neo4j-cli update` in a single
  flush-left `SECURITY.md` at repo root — five sections (trust root,
  residual risks, mitigation roadmap, review cadence, reporting channel).
- Make the document discoverable: link from `README.md` "Installation" and
  from `neo4j-cli update --help` Long text.
- Advertise `security@neo4j.com` as the vulnerability reporting channel
  (confirmed interactively).
- Keep the document end-user-facing — no Oplane / internal threat-model
  URLs, no brittle file:line anchors.

## Non-Goals

- Implementing any of the mitigation-roadmap items (sigstore/cosign
  signing, reproducible builds, out-of-band key publication). Each would
  be a separate engineering project.
- Fixing the other two Oplane gaps under CLI-155 (fixed `/tmp` filename
  on the elevated path; missing stderr warning on `--force` override).
  Those are tracked as their own sub-issues.
- Adding redirect-chain validation (the "honourable mention" in the parent
  threat model).
- Adding `SECURITY.md` to `.github/` (repo-root placement is the standard
  GitHub-recognised location).
- Introducing an Issue Template or PR Template for security reports —
  email channel only per interactive decision.
- Changing any code in `swap.go` / `release.go` / `update.go` beyond the
  one-sentence Long-text extension on `update`.

## Requirements

### Functional Requirements

- **REQ-F-001**: A new file MUST exist at repo root: `SECURITY.md`. Flush
  left, ≤120 lines, GitHub-flavoured Markdown, no leading whitespace on
  headings.

- **REQ-F-002**: `SECURITY.md` MUST contain exactly the following H2
  sections, in this order:
  1. `## Reporting a vulnerability` — names `security@neo4j.com` and
     instructs reporters NOT to file public GitHub issues for security
     reports.
  2. `## Supply-chain trust root for \`neo4j-cli update\`` — enumerates
     the four trust links: Go TLS verification against the host system
     root store; TLS certificates for the five-host allowlist (`github.com`,
     `api.github.com`, `codeload.github.com`, `objects.githubusercontent.com`,
     `release-assets.githubusercontent.com`); integrity of GitHub
     release-assets storage for `neo4j-labs/neo4j-cli`; SHA256
     `…_checksums.txt` manifest verified **before** archive extraction.
  3. `## Accepted residual risks` — three items: GitHub Actions
     release-workflow compromise; release-assets storage compromise;
     single-channel substitution (archive + checksums shipped on the
     same channel). Closes with one sentence stating these are accepted
     because next-step mitigations are not yet implemented.
  4. `## Mitigation roadmap (not committed)` — three bullets:
     sigstore/cosign signed releases with verification in `update`;
     reproducible builds; out-of-band publication of release public
     keys. Section header MUST include the `(not committed)` qualifier
     so the doc cannot be read as a delivery commitment.
  5. `## Review cadence` — single short paragraph stating the policy is
     revisited periodically; deliberately vague (no calendar cadence,
     no retrospective binding) per interactive decision.

- **REQ-F-003**: `SECURITY.md` MUST NOT contain file:line anchors
  pointing into `neo4j-cli/internal/subcommands/update/` (would drift;
  is not user-actionable). Refer to the `update` command by name only.

- **REQ-F-004**: `SECURITY.md` MUST NOT reference Oplane, internal
  threat-model IDs, or any internal review tooling — file is end-user-facing.

- **REQ-F-005**: `README.md` "Installation" section MUST gain a single
  trailing line linking to `SECURITY.md` via a relative path
  (`./SECURITY.md`). The line MUST be placed AFTER the existing
  `#### Alternatives` bullet list (current location ~line 30) and MUST
  NOT introduce a new top-level heading. Suggested wording:
  `**Security:** how \`neo4j-cli update\` validates downloads and our vulnerability reporting channel are documented in [SECURITY.md](./SECURITY.md).`

- **REQ-F-006**: `neo4j-cli/internal/subcommands/update/update.go` `Long`
  field at lines 96-101 MUST gain one trailing sentence pointing to
  `SECURITY.md`. Suggested wording: append
  `" See SECURITY.md in the repo for the supply-chain trust root and accepted residual risks."`
  to the existing trailing string segment. No other text in the `Long`
  string may change.

- **REQ-F-007**: After the `update.go` edit, `go generate ./neo4j-cli/internal/skill/...`
  MUST be run and the resulting bundle changes
  (`neo4j-cli/internal/skill/bundle/references/update.md` at minimum)
  MUST be committed in the same commit as the source-side edit.
  Rationale: `TestGenerator_RoundTrip` fails CI otherwise; `make
  generate-check` mirrors the gate locally.

- **REQ-F-008**: A changelog entry MUST be added via
  `changie new --projects neo4j-cli --kind Patch --body "docs: add SECURITY.md documenting update trust root (CLI-158)"`
  (or hand-authored YAML under `.changes/unreleased/` per the AGENTS.md
  Changie Notes when changie is not installed locally). Rationale: the
  `update --help` Long-text change is user-visible.

### Non-Functional Requirements

- **REQ-NF-001**: `make test`, `make fmt-check`, `make lint`, and
  `make license-check` MUST all be clean (AGENTS.md final-gate rule).
  `TestGenerator_RoundTrip` is the load-bearing gate for this PRD — it
  proves the bundle regen in REQ-F-007 was actually done.

- **REQ-NF-002**: `make generate-check` MUST be clean after the bundle
  regen. If it fires on anything other than `bundle/references/update.md`
  (e.g. `SKILL.md`), the change has accidentally touched a global help
  surface — investigate before committing.

- **REQ-NF-003**: `SECURITY.md` MUST start with the Neo4j copyright
  header IF and only if `make license-check` requires it for `.md`
  files. Inspect `Makefile` `license-check` target and `addlicense`
  invocation to confirm scope. (Current observation: license check
  globs over `.go` files; `.md` is unlikely to need a header, but
  verify before pushing — running `make license-check` after the new
  file is added is sufficient.)

- **REQ-NF-004**: `SECURITY.md` MUST be LF-pinned via `.gitattributes`
  IF the repo's existing `.gitattributes` pins other `.md` files to
  LF. Inspect existing rules before adding a new one — Windows CI golden
  comparisons hinge on this. Likely no change needed if the existing
  `*.md` rule already covers repo-root files.

- **REQ-NF-005**: No new Go dependency. No new test file. No change to
  `swap.go`, `release.go`, or any file outside the four named in
  Acceptance Criteria.

## Technical Considerations

- **Why `SECURITY.md` at repo root, not under `.github/`.** Both
  locations are GitHub-recognised, but repo root is the more discoverable
  default and matches the README convention of linking with a relative
  `./SECURITY.md` path. `.github/` placement would also work but would
  bury the file from contributors who navigate the source tree.

- **Why no GitHub Security Advisories channel.** Confirmed interactively:
  email `security@neo4j.com` is the single channel. GHSA would split
  reports across two systems; one Neo4j-owned inbox is the right
  surface for this project.

- **Why no file:line anchors.** Confirmed interactively: anchors drift
  fast (the surrounding files churn for unrelated reasons), and the
  reader isn't going to act on a file:line — they want to know the
  trust root. Refer to commands by name (`neo4j-cli update`), let the
  code be the source of truth for "exactly where".

- **Why a vague review cadence.** Confirmed interactively: a calendar
  cadence (quarterly) or release-tied cadence (minor-release retro)
  would create a binding the project may not honour. Keep the
  commitment soft: "revisited periodically".

- **Why a Patch changelog entry.** AGENTS.md says "PRs require a
  changelog entry only for user-facing changes". The new file alone
  would not require an entry, but the `update --help` Long text changes
  for end users, so a `Patch` entry is appropriate. Confirmed
  interactively.

- **Bundle regen scope.** The `Long`-text change is on the leaf
  `update` command, so the affected reference doc is
  `bundle/references/update.md`. Root-level `SKILL.md` is only touched
  if a globally bound flag changes; this change does not touch any
  flag. If `make generate-check` flags anything outside `update.md`,
  treat it as a signal that something larger has been touched (or that
  there is pre-existing drift) and investigate before merging.

- **Why mention sigstore/cosign even though it's not implemented.**
  Transparency about the mitigation roadmap is the whole point of the
  doc; suppressing the next-step mitigations would let readers assume
  the residual risks are accepted permanently. The `(not committed)`
  qualifier on the section heading prevents over-reading.

- **Host-allowlist enumeration.** Listing all five hosts (rather than
  the abstract `*.githubusercontent.com`) is deliberate: the wildcard
  form would understate `api.github.com` and `github.com`, and reading
  the explicit list lets a network operator reason about firewall
  posture.

## Acceptance Criteria

- [ ] `SECURITY.md` exists at repo root with the five H2 sections from
  REQ-F-002, in order, advertising `security@neo4j.com` as the reporting
  channel.
- [ ] `SECURITY.md` contains no Oplane references, no internal
  threat-model IDs, no file:line anchors.
- [ ] `SECURITY.md` is ≤120 lines and flush-left (no leading whitespace
  on H1/H2 headings).
- [ ] `README.md` "Installation" section has a trailing line linking to
  `./SECURITY.md` placed AFTER the `#### Alternatives` bullet list. No
  new top-level heading.
- [ ] `neo4j-cli/internal/subcommands/update/update.go` `Long` (lines
  96-101) has one new trailing sentence pointing to `SECURITY.md`. No
  other text in `Long` changes.
- [ ] `neo4j-cli/internal/skill/bundle/references/update.md` reflects
  the new `Long` text (regenerated via `go generate ./neo4j-cli/internal/skill/...`).
- [ ] A changelog entry exists under `.changes/unreleased/` with
  `project: neo4j-cli`, `kind: Patch`, body referencing CLI-158.
- [ ] `make fmt-check`, `make lint`, `make test`, `make generate-check`,
  and `make license-check` are all clean.
- [ ] Manual sanity: `./bin/neo4j-cli update --help | grep -i security`
  prints the new sentence; `cat SECURITY.md | wc -l` is under 120;
  `grep -c SECURITY.md README.md` returns ≥1.

## Out of Scope

- Implementation of sigstore/cosign signed releases.
- Implementation of reproducible builds.
- Out-of-band publication of release public keys.
- The other two CLI-155 sub-issue gaps: randomising the elevated-path
  temp filename in `swap.go` (~line 435); adding a stderr warning when
  `--force` overrides install-method detection in `update.go` (~lines
  385-399). Each has or will have its own Linear sub-issue.
- Custom `CheckRedirect` policy on the `http.Client` used for downloads
  (the parent threat model's "honourable mention").
- Adding `SECURITY.md` to `.github/` in addition to repo root.
- Adding a security section to `CONTRIBUTING.md` (`SECURITY.md` is the
  canonical location; cross-linking from CONTRIBUTING is fine but not
  required).
- Adding a GitHub Security Advisories channel as a secondary report
  path.
- Any change to the website content at `gh-pages/` (the public install
  site does not currently mention security and is prompt-driven; out
  of scope here).

## Open Questions

None. All design decisions were resolved interactively before PRD
generation:

- Reporting channel → `security@neo4j.com` only.
- Changelog entry → yes, `Patch` kind.
- File:line anchors in `SECURITY.md` → drop entirely.
- Review-cadence wording → keep vague (no calendar cadence).
