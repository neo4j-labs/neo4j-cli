# Security

This document describes how to report a security vulnerability in
`neo4j-cli` and the supply-chain trust root that `neo4j-cli update`
relies on. It is end-user-facing: it lists the trust links a user is
extending when they run `update`, the residual risks that are
accepted today, and the mitigations on the roadmap.

## Reporting a vulnerability

Please email **security@neo4j.com** with details of the issue.

Do **not** file a public GitHub issue for security reports. Public
issues are visible to everyone and would tip off attackers before a
fix is available. Email is the single reporting channel for this
project; we do not currently use GitHub Security Advisories.

When reporting, please include enough detail to reproduce the issue
(version of `neo4j-cli`, platform, exact command, observed behaviour)
so we can triage quickly.

## Supply-chain trust root for `neo4j-cli update`

`neo4j-cli update` downloads a replacement binary from GitHub
releases and atomically swaps it for the running one. When you run
`update`, you are extending trust to the following links:

1. **Go's TLS stack against the host root certificate store.** All
   HTTPS connections made by `update` use Go's standard `crypto/tls`
   verification against the operating-system root store. If the host
   root store is compromised, TLS verification is compromised.

2. **TLS certificates for the five-host GitHub allowlist.** `update`
   will only connect to the following hosts; any redirect or URL
   pointing elsewhere is rejected before a connection is opened:

   - `github.com`
   - `api.github.com`
   - `codeload.github.com`
   - `objects.githubusercontent.com`
   - `release-assets.githubusercontent.com`

   These are listed explicitly rather than as a wildcard so that a
   network operator can reason precisely about firewall posture.

3. **Integrity of GitHub release-assets storage for
   `neo4j-labs/neo4j-cli`.** The release archive and its
   `…_checksums.txt` manifest are both fetched from GitHub-hosted
   release-assets storage. Trust in the published artefacts reduces
   to trust in GitHub's storage of those assets for this repository.

4. **SHA256 `…_checksums.txt` manifest verified BEFORE archive
   extraction.** `update` computes the SHA256 of the downloaded
   archive and matches it against the entry in `…_checksums.txt`
   before any bytes are extracted. A mismatch aborts the update; no
   archive contents touch disk outside the temporary download.

If all four links hold, the binary that replaces the running
executable is the binary that was published from the
`neo4j-labs/neo4j-cli` release workflow.

## Accepted residual risks

The following risks are not mitigated by the current `update`
implementation:

- **GitHub Actions release-workflow compromise.** A malicious change
  to the release workflow (or to a workflow dependency) could publish
  a tampered archive whose checksum matches a tampered manifest. The
  checksum check above does not protect against this.

- **Release-assets storage compromise.** Both the archive and the
  checksums manifest are fetched from the same GitHub release-assets
  storage. An attacker with write access to a published release could
  replace both files coherently.

- **Single-channel substitution.** Because the archive and its
  checksum manifest travel over the same channel (GitHub release
  assets), an attacker who can substitute both files at once defeats
  the checksum check. There is no out-of-band channel today.

These are accepted today because the next-step mitigations listed
below are not yet implemented.

## Mitigation roadmap (not committed)

The following mitigations would address the residual risks above. They
are documented here for transparency. They are **not** committed
deliverables and may change priority without notice:

- **sigstore/cosign signed releases with verification in `update`.**
  Sign release archives at publication time and verify the signature
  in `update` against a project public key before extraction.

- **Reproducible builds.** Make the build process deterministic so
  that anyone with the source tree can rebuild a release archive and
  confirm it byte-matches the published artefact.

- **Out-of-band publication of release public keys.** Publish the
  verification public key(s) on a channel separate from GitHub
  release assets (e.g. the project website) so that a release-assets
  compromise cannot rewrite the trust anchor at the same time.

## Review cadence

This document is revisited periodically as the project evolves. There
is no fixed calendar cadence; updates are made when the supply-chain
posture changes (for example, when one of the roadmap mitigations
lands or when a new trust link is added).
