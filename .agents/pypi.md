# PyPI Distribution Notes

User-facing channel docs live in [`distribution/pypi/README.md`](../distribution/pypi/README.md). This file covers the workflow-side gotchas.

## Workflow shape

- `publish-pypi.yml` mirrors `publish-npm.yml`'s `workflow_run + workflow_dispatch` shape — see that file's comments and `.agents/deployment.md` "Release Workflow Notes" for cross-workflow gotchas (`workflows: ["release"]` matches the lowercase `name:`, cross-run `download-artifact` needs both `github-token` and `run-id`, `workflow_run` events have no `inputs.*`).
- `permissions: {}` works for jobs that download SAME-run artifacts via `actions/download-artifact@v4`. Cross-run downloads (workflow_run path consuming release.yml's artifacts) need workflow-level `actions: read`.

## go-to-wheel

- The `go-to-wheel` CLI ALWAYS cross-compiles from Go source — its argparse accepts only `go_dir`, no `--binary-path`. To wrap pre-built GoReleaser binaries into wheels (REQ-F-009 binary-parity invariant), bypass the CLI and call `go_to_wheel.build_wheel(binary_path=..., ...)` directly via an inline `python3 - <<'PY'` heredoc. The library function takes a binary path and a platform_tag; iterate over the six platforms and read each binary from `dist/neo4j-cli_<VERSION>_<TitleOS>_<arch>/<binary>`.
- `.github/workflows/requirements-build.txt` pins `go-to-wheel` with `--require-hashes` for supply-chain safety. Update the hash via `pip3 download --no-deps -d /tmp/x go-to-wheel` then `pip3 hash /tmp/x/<wheel>`.

## Version normalisation

- PEP 440 normalisation lives in a single shell helper at `.github/scripts/version-to-pep440.sh` (pure bash, no python/jq deps). Both auto and manual paths invoke it once. The Go ldflags `Version` stays the original GoReleaser tag (so `neo4j-cli --version` and the smoke-test grep keep matching); only the wheel filename + PyPI metadata use the PEP 440 form.

## Heredoc indentation

- YAML `run: |` block + bash heredoc gotcha: the heredoc terminator (`PY`) must be at the same indentation as the surrounding YAML block (which strips a fixed prefix). After strip, both the heredoc body and `PY` end up at column 0 — that IS valid for `<<'PY'`. Verify by round-tripping with PyYAML and `compile()`-ing the extracted python.
