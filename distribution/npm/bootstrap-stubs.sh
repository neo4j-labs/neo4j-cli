#!/usr/bin/env bash
# Copyright (c) "Neo4j"
# Neo4j Sweden AB [http://neo4j.com]
#
# distribution/npm/bootstrap-stubs.sh — one-time helper to claim the 9 @neo4j-labs/* package
# names on the npm registry so npm Trusted Publisher can be configured per package in the
# npm UI (npm has no pending-publisher flow; the package must already exist before its
# Trusted Publisher source can be set).
#
# Design:
#   - Hardcoded VERSION=0.0.0-bootstrap.1 and dist-tag `bootstrap` so unqualified
#     `npm i @neo4j-labs/cli` never resolves to one of these stub publishes (the registry's
#     `latest` tag is unaffected; the stub only sits under the `bootstrap` tag).
#   - Each platform package's `bin/<file>` is a 1-byte placeholder (`#`); enough to satisfy
#     `npm publish` while shipping no executable code.
#   - Mirrors publish.sh's structure: same platforms.tsv, same template render, same
#     ordering (8 platform packages first, wrapper last), same `already_published` skip
#     pattern for idempotent re-runs.
#   - Refuses to run if `npm whoami` fails — the maintainer must `npm login --scope=@neo4j-labs`
#     first. There is no --dry-run flag; this script is intentionally local-run-only and a
#     dry-run path would defeat the purpose (the registry side effect IS the goal).
#
# Usage (one-time, by a maintainer with publish access on the @neo4j-labs scope):
#   npm login --scope=@neo4j-labs
#   NPM_OTP=123456 make npm-bootstrap   # or: NPM_OTP=123456 bash distribution/npm/bootstrap-stubs.sh
#   npm logout
#
# NPM_OTP is required if the npm account has 2FA on publishes (default for @neo4j-labs).
# A single OTP code is reused across all 9 publishes, which works because npm's OTP window
# is wide enough to cover the full run; if a code expires mid-run, re-invoke with a fresh
# code — already-published packages skip via the idempotency check.
#
# After running this, configure Trusted Publisher in the npm UI for each of the 9 packages
# (org `neo4j-labs`, repo `neo4j-cli`, workflow `publish-npm.yml`, environment blank).

set -euo pipefail

# Resolve script + repo dirs (works from any cwd).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="${SCRIPT_DIR}/cli"
PLATFORM_TMPL_DIR="${SCRIPT_DIR}/cli-platform"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
PLATFORMS_TSV="${REPO_ROOT}/distribution/platforms.tsv"
WORK_ROOT="${REPO_ROOT}/dist/.npm-bootstrap"

VERSION="0.0.0-bootstrap.1"
TAG_FLAG="--tag bootstrap"
OTP_FLAG=""
if [ -n "${NPM_OTP:-}" ]; then
  OTP_FLAG="--otp ${NPM_OTP}"
fi

echo "[bootstrap] version=${VERSION} tag=bootstrap"

# --- Auth check -----------------------------------------------------------------------------
if ! npm whoami >/dev/null 2>&1; then
  echo "[bootstrap] not logged in — run \"npm login --scope=@neo4j-labs\" first" >&2
  exit 1
fi
echo "[bootstrap] authenticated as $(npm whoami)"

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
  # shellcheck disable=SC2086 # we want word-splitting on TAG_FLAG and OTP_FLAG
  npm publish "$pkg_dir" --access public $TAG_FLAG $OTP_FLAG
}

# --- Build + publish 8 platform packages first ---------------------------------------------
mkdir -p "$WORK_ROOT"

# Read platforms.tsv (skip header).
while IFS=$'\t' read -r DIRNAME_TMPL NPM_OS NPM_CPU BIN_FILENAME; do
  # ${VERSION} substitution in the goreleaser dirname is intentionally ignored — bootstrap
  # never reads from dist/ (no real binary involved). Only NPM_OS / NPM_CPU / BIN_FILENAME
  # are consumed below.
  _="$DIRNAME_TMPL"
  PKG_NAME="@neo4j-labs/cli-${NPM_OS}-${NPM_CPU}"
  PKG_DIR="${WORK_ROOT}/cli-${NPM_OS}-${NPM_CPU}"

  rm -rf "$PKG_DIR"
  mkdir -p "${PKG_DIR}/bin"

  # 1-byte placeholder binary.
  printf '#' >"${PKG_DIR}/bin/${BIN_FILENAME}"
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

if [ -f "${DIST_DIR}/README.md" ]; then
  cp -p "${DIST_DIR}/README.md" "${WRAPPER_DIR}/README.md"
fi

sed -e "s/__VERSION__/${VERSION}/g" \
  "${DIST_DIR}/package.json.tmpl" >"${WRAPPER_DIR}/package.json"

publish_pkg "@neo4j-labs/cli" "$WRAPPER_DIR"

echo "[bootstrap] done."
