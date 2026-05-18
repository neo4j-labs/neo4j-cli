#!/usr/bin/env bash
# Builds and publishes platform-specific VS Code extension .vsix packages.
# Mirrors distribution/npm/publish.sh in structure and conventions.
#
# Usage:
#   publish.sh [--dry-run] <version>
#
# Env:
#   DIST_DIR   path to GoReleaser dist/ (default: ../../dist)
#   VSCE_PAT   VS Code Marketplace personal access token
#   OVSX_PAT   Open VSX personal access token
#
# Each target:
#   1. Copies the GoReleaser-built binary into bin/
#   2. Packages a platform-specific .vsix
#   3. Publishes to VS Code Marketplace (vsce)
#   4. Publishes to Open VSX (ovsx) — same .vsix artifact
#
set -euo pipefail

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=true
  shift
fi

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  echo "Usage: publish.sh [--dry-run] <version>" >&2
  exit 1
fi

# Strip leading 'v' for package.json version field
VERSION_CLEAN="${VERSION#v}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DIST_DIR="${DIST_DIR:-${SCRIPT_DIR}/../../dist}"

# VS Code platform target → GoReleaser archive dirname template → binary filename
# Format: <vscode-target>:<goreleaser-dirname>:<binary-name>
# Kept in sync with distribution/platforms.tsv
PLATFORMS=(
  "darwin-arm64:neo4j-cli_${VERSION}_Darwin_arm64:neo4j-cli"
  "darwin-x64:neo4j-cli_${VERSION}_Darwin_x86_64:neo4j-cli"
  "linux-arm64:neo4j-cli_${VERSION}_Linux_arm64:neo4j-cli"
  "linux-x64:neo4j-cli_${VERSION}_Linux_x86_64:neo4j-cli"
  "win32-arm64:neo4j-cli_${VERSION}_Windows_arm64:neo4j-cli.exe"
  "win32-x64:neo4j-cli_${VERSION}_Windows_x86_64:neo4j-cli.exe"
  "alpine-x64:neo4j-cli_${VERSION}_Linux_x86_64:neo4j-cli"
  "alpine-arm64:neo4j-cli_${VERSION}_Linux_arm64:neo4j-cli"
)

cd "$SCRIPT_DIR"
npm ci --silent

# Stamp version into package.json for this run (reverted after publish)
ORIGINAL_VERSION=$(node -p "require('./package.json').version")
node -e "
  const p = require('./package.json');
  p.version = '${VERSION_CLEAN}';
  require('fs').writeFileSync('./package.json', JSON.stringify(p, null, 2) + '\n');
"
trap 'node -e "const p=require(\"./package.json\"); p.version=\"'"$ORIGINAL_VERSION"'\"; require(\"fs\").writeFileSync(\"./package.json\", JSON.stringify(p,null,2)+\"\n\")"' EXIT

mkdir -p bin

for entry in "${PLATFORMS[@]}"; do
  IFS=: read -r vscode_target goreleaser_dir bin_name <<< "$entry"

  src_binary="${DIST_DIR}/${goreleaser_dir}/${bin_name}"
  dest_binary="bin/${bin_name}"
  vsix_file="neo4j-cli-${VERSION_CLEAN}-${vscode_target}.vsix"

  # Binary staging
  if [[ "$DRY_RUN" == "true" && ! -f "$src_binary" ]]; then
    echo "[stub]    ${vscode_target}: binary not found, using 1-byte placeholder"
    printf 'X' > "$dest_binary"
  else
    if [[ ! -f "$src_binary" ]]; then
      echo "[ERROR]   ${vscode_target}: binary not found at ${src_binary}" >&2
      exit 1
    fi
    cp "$src_binary" "$dest_binary"
    chmod +x "$dest_binary"
  fi

  if [[ "$DRY_RUN" == "true" ]]; then
    echo "[dry-run] would package ${vsix_file} --target ${vscode_target}"
    npx @vscode/vsce package --target "$vscode_target" --out "$vsix_file" --no-yarn 2>/dev/null || \
      echo "[dry-run] packaging skipped (install @vscode/vsce to test locally)"
  else
    echo "[package] ${vsix_file}"
    npx @vscode/vsce package --target "$vscode_target" --out "$vsix_file" --no-yarn

    echo "[vsce]    publishing ${vsix_file}"
    npx @vscode/vsce publish --packagePath "$vsix_file" --pat "${VSCE_PAT}"

    echo "[ovsx]    publishing ${vsix_file}"
    npx ovsx publish "$vsix_file" --pat "${OVSX_PAT}"
  fi

  rm -f "$dest_binary"
  echo "[done]    ${vscode_target}"
done

echo ""
echo "All platforms complete (dry-run=${DRY_RUN})"
