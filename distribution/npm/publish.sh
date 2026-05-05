#!/usr/bin/env bash
# distribution/npm/publish.sh — publishes @neo4j-labs/cli + 8 platform packages to npm.
#
# Inputs:
#   - GORELEASER_CURRENT_TAG (env, required) — version tag, e.g. "v0.2.0" or "v0.2.0-alpha.1".
#     Leading "v" is stripped. The script errors if unset/empty.
#   - dist/ (working dir) — populated by GoReleaser. Each platform archive must already be
#     extracted to a directory matching the goreleaser_dirname_template column in
#     distribution/platforms.tsv (with ${VERSION} substituted), containing the binary at the
#     top level (e.g. dist/neo4j-cli_0.2.0_Darwin_arm64/neo4j-cli).
#   - ~/.npmrc — must already be configured with a valid auth token by the caller (CI workflow).
#
# Ordering:
#   The 8 platform packages are published FIRST, then the wrapper @neo4j-labs/cli is published LAST.
#   This is mandatory: the wrapper's optionalDependencies reference the platform packages, and
#   `npm install @neo4j-labs/cli` returns 404 for any platform pkg that doesn't yet exist on the
#   registry. Publishing the wrapper before its platform deps would break first-time installs.
#
# Idempotency:
#   Before each `npm publish`, the script runs `npm view <name>@<version> version` and skips
#   the publish if the package+version already exists on the registry. This means a same-version
#   re-run after a partial-publish failure is safe — already-published packages are no-ops, and
#   the script only attempts the ones that didn't make it. There's no rollback step; failed
#   publishes are recovered by re-running the workflow at the same version.
#
# Dist-tag selection (REQ-F-013):
#   The script derives the npm dist-tag from the version string:
#     X.Y.Z              → (no flag, default `latest`)
#     X.Y.Z-alpha*       → --tag alpha
#     X.Y.Z-beta*        → --tag beta
#     X.Y.Z-rc*          → --tag rc
#     X.Y.Z-<other>      → --tag next  (catch-all for unrecognized prereleases)
#   This gates `npm i @neo4j-labs/cli` so it only ever resolves to a stable version; pre-releases
#   are opt-in via `npm i @neo4j-labs/cli@alpha` (etc).
#
# Recovery flow:
#   1. CI publish fails partway through (e.g. transient registry 5xx after platforms 1-3).
#   2. Maintainer triggers .github/workflows/publish-npm.yml manually with the same version.
#   3. The download-extract step recreates dist/. This script re-runs; platforms 1-3 hit the
#      idempotent skip path, platforms 4-8 publish, then the wrapper publishes. Workflow exits 0.
#   4. If a publish keeps failing for one specific package, debug from the CI logs — the
#      [skip]/[publish]/[ERROR] log lines tell you exactly which package and where.
#
# Flags:
#   --dry-run   Pass --dry-run through to `npm publish` (does NOT touch the registry, does
#               NOT update ~/.npmrc, but still renders templates and runs idempotency checks).
#               Used by `make npm-publish-dry`.

set -euo pipefail

# Resolve script + repo dirs (works from any cwd).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="${SCRIPT_DIR}/cli"
PLATFORM_TMPL_DIR="${SCRIPT_DIR}/cli-platform"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
PLATFORMS_TSV="${REPO_ROOT}/distribution/platforms.tsv"
GORELEASER_DIST="${REPO_ROOT}/dist"
WORK_ROOT="${REPO_ROOT}/dist/.npm-build"

# --- Args -----------------------------------------------------------------------------------
DRY_RUN=""
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN="--dry-run" ;;
    *) echo "ERROR: unknown argument: $arg" >&2; exit 2 ;;
  esac
done

# --- Version --------------------------------------------------------------------------------
if [ -z "${GORELEASER_CURRENT_TAG:-}" ]; then
  echo "ERROR: GORELEASER_CURRENT_TAG is unset; expected something like v0.2.0 or v0.2.0-alpha.1" >&2
  exit 1
fi
VERSION="${GORELEASER_CURRENT_TAG#v}"
echo "[publish-npm] version=${VERSION}"

# --- Dist-tag selection ---------------------------------------------------------------------
case "$VERSION" in
  *-alpha*) TAG=alpha ;;
  *-beta*)  TAG=beta ;;
  *-rc*)    TAG=rc ;;
  *-*)      TAG=next ;;
  *)        TAG="" ;;
esac
if [ -n "$TAG" ]; then
  TAG_FLAG="--tag $TAG"
  echo "[publish-npm] dist-tag=${TAG}"
else
  TAG_FLAG=""
  echo "[publish-npm] dist-tag=(default latest)"
fi

if [ -n "$DRY_RUN" ]; then
  echo "[publish-npm] DRY-RUN mode — npm publish will use --dry-run"
fi

# --- Sanity checks --------------------------------------------------------------------------
if [ ! -f "$PLATFORMS_TSV" ]; then
  echo "ERROR: platforms file not found: $PLATFORMS_TSV" >&2
  exit 1
fi
if [ ! -f "${DIST_DIR}/package.json.tmpl" ]; then
  echo "ERROR: wrapper template not found: ${DIST_DIR}/package.json.tmpl" >&2
  exit 1
fi
if [ ! -f "${PLATFORM_TMPL_DIR}/package.json.tmpl" ]; then
  echo "ERROR: platform template not found: ${PLATFORM_TMPL_DIR}/package.json.tmpl" >&2
  exit 1
fi
if [ ! -f "${DIST_DIR}/bin/neo4j-cli.js" ]; then
  echo "ERROR: wrapper bin shim not found: ${DIST_DIR}/bin/neo4j-cli.js" >&2
  exit 1
fi

# --- Helper: check whether <pkg>@<ver> is already on the registry ---------------------------
# Echoes "yes" if published, "no" otherwise. Tolerates `npm view` returning non-zero on 404.
already_published() {
  local pkg="$1"
  local ver="$2"
  local found
  found="$(npm view "${pkg}@${ver}" version 2>/dev/null || true)"
  if [ -n "$found" ]; then
    echo "yes"
  else
    echo "no"
  fi
}

# --- Helper: publish one rendered package directory ----------------------------------------
publish_pkg() {
  local pkg_name="$1"
  local pkg_dir="$2"
  local published
  published="$(already_published "$pkg_name" "$VERSION")"
  if [ "$published" = "yes" ]; then
    echo "[skip] ${pkg_name}@${VERSION} already published"
    return 0
  fi
  echo "[publish] ${pkg_name}@${VERSION}"
  # shellcheck disable=SC2086 # we want word-splitting on TAG_FLAG and DRY_RUN
  npm publish "$pkg_dir" --access public $TAG_FLAG $DRY_RUN
}

# --- Build + publish 8 platform packages first ---------------------------------------------
mkdir -p "$WORK_ROOT"

# Read platforms.tsv (skip header).
while IFS=$'\t' read -r DIRNAME_TMPL NPM_OS NPM_CPU BIN_FILENAME; do
  # Substitute ${VERSION} in the goreleaser dirname.
  GORELEASER_DIRNAME="${DIRNAME_TMPL//\$\{VERSION\}/$VERSION}"
  SRC_BIN="${GORELEASER_DIST}/${GORELEASER_DIRNAME}/${BIN_FILENAME}"
  PKG_NAME="@neo4j-labs/cli-${NPM_OS}-${NPM_CPU}"
  PKG_DIR="${WORK_ROOT}/cli-${NPM_OS}-${NPM_CPU}"

  rm -rf "$PKG_DIR"
  mkdir -p "${PKG_DIR}/bin"

  if [ ! -f "$SRC_BIN" ]; then
    if [ -n "$DRY_RUN" ]; then
      # In dry-run we tolerate missing binaries: maintainer's `make snapshot` only builds
      # the current platform, so 7/8 binaries are absent locally. Stub them so template
      # rendering + ordering checks still execute end-to-end.
      echo "[dry-run-stub] ${SRC_BIN} missing — writing 1-byte placeholder"
      printf '#' >"${PKG_DIR}/bin/${BIN_FILENAME}"
    else
      echo "ERROR: expected binary not found: $SRC_BIN" >&2
      echo "       (did GoReleaser run? is the dist/ artifact extracted at the right layout?)" >&2
      exit 1
    fi
  else
    # Copy binary preserving exec bit. macOS cp -p / GNU cp -p both preserve mode.
    cp -p "$SRC_BIN" "${PKG_DIR}/bin/${BIN_FILENAME}"
  fi
  chmod +x "${PKG_DIR}/bin/${BIN_FILENAME}"

  # Render package.json.
  sed \
    -e "s/__VERSION__/${VERSION}/g" \
    -e "s/__OS__/${NPM_OS}/g" \
    -e "s/__CPU__/${NPM_CPU}/g" \
    -e "s/__BIN_NAME__/${BIN_FILENAME}/g" \
    "${PLATFORM_TMPL_DIR}/package.json.tmpl" >"${PKG_DIR}/package.json"

  publish_pkg "$PKG_NAME" "$PKG_DIR"
done < <(tail -n +2 "$PLATFORMS_TSV")

# --- Build + publish wrapper LAST ----------------------------------------------------------
WRAPPER_DIR="${WORK_ROOT}/cli"
rm -rf "$WRAPPER_DIR"
mkdir -p "${WRAPPER_DIR}/bin"
cp -p "${DIST_DIR}/bin/neo4j-cli.js" "${WRAPPER_DIR}/bin/neo4j-cli.js"
chmod +x "${WRAPPER_DIR}/bin/neo4j-cli.js"

# Ship the user-facing README if it exists (task-009 will add it; this script tolerates absence).
if [ -f "${DIST_DIR}/README.md" ]; then
  cp -p "${DIST_DIR}/README.md" "${WRAPPER_DIR}/README.md"
fi

sed -e "s/__VERSION__/${VERSION}/g" \
  "${DIST_DIR}/package.json.tmpl" >"${WRAPPER_DIR}/package.json"

publish_pkg "@neo4j-labs/cli" "$WRAPPER_DIR"

echo "[publish-npm] done."
