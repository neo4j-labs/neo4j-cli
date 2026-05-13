# Windows CI Gotchas

## Path separators

- Path-separator bugs in `expandPath`-style helpers are Windows-only. Catalog entries keep forward slashes (portable convention); helpers MUST wrap any post-substitution path through `filepath.FromSlash` (or build via `filepath.Join`) so the whole path is OS-native. A `ReplaceAll(path, "$XDG_CONFIG_HOME", xdg)` where `xdg` came from `os.Getenv` produces mixed separators on Windows (`C:\…\.config/opencode`) — fix at the helper, not the catalog.
- Test expected values that hard-code separators bake in OS assumptions. Build expected values with `filepath.Join` / `filepath.FromSlash` rather than literals when asserting cross-OS path output. MemMapFs marker paths in detection tests must also be built OS-natively so they match what the (post-fix) helper looks up.

## Line endings

- Committed `.md` / golden / bundle files MUST be pinned to LF via `.gitattributes` — Windows runners have `core.autocrlf=true` by default and will rewrite to CRLF on checkout. The renderer (`common/skill/render`) and `make generate-check` both assume LF; a CRLF checkout breaks byte-equal golden tests AND `git diff --exit-code`. The repo-root `.gitattributes` covers `common/skill/render/testdata/**`, `**/internal/skill/bundle/**`, `**/internal/skill/additions.md`, `**/internal/skill/description.txt`. `common/skill/bundles_test.go::TestCommittedBundlesAndTestdataAreLF` is the assertion that catches a weakened/removed attribute.
