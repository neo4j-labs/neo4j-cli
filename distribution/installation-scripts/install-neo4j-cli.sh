#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────
#  neo4j-cli installer
#  https://github.com/neo4j-labs/neo4j-cli
# ─────────────────────────────────────────────

REPO="neo4j-labs/neo4j-cli"
BINARY_NAME="neo4j-cli"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

# ── Colours ──────────────────────────────────
if [ -t 1 ]; then
  BOLD="\033[1m"; GREEN="\033[32m"; RED="\033[31m"; YELLOW="\033[33m"; RESET="\033[0m"
else
  BOLD=""; GREEN=""; RED=""; YELLOW=""; RESET=""
fi

info()    { echo -e "${GREEN}▶${RESET} $*"; }
warn()    { echo -e "${YELLOW}⚠${RESET}  $*"; }
error()   { echo -e "${RED}✖${RESET}  $*" >&2; exit 1; }
success() { echo -e "${GREEN}${BOLD}✔${RESET}  $*"; }

# ── Detect OS ────────────────────────────────
detect_os() {
  case "$(uname -s)" in
    Darwin)  echo "Darwin" ;;
    Linux)   echo "Linux" ;;
    MINGW*|MSYS*|CYGWIN*) echo "Windows" ;;
    *) error "Unsupported OS: $(uname -s)" ;;
  esac
}

# ── Detect Arch ──────────────────────────────
detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)   echo "x86_64" ;;
    arm64|aarch64)  echo "arm64" ;;
    i386|i686)      echo "i386" ;;
    *) error "Unsupported architecture: $(uname -m)" ;;
  esac
}

# ── Resolve latest version via GitHub ────────
resolve_version() {
  local version
  version=$(curl -fsSL "https://github.com/${REPO}/releases/latest" \
    -o /dev/null -w "%{url_effective}" \
    | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+[^/]*$')
  
  if [ -z "$version" ]; then
    error "Could not determine latest release version. Set VERSION env var to override."
  fi
  echo "$version"
}

# ── Require a command ─────────────────────────
require() {
  command -v "$1" &>/dev/null || error "Required tool not found: $1"
}

# ─────────────────────────────────────────────
#  Main
# ─────────────────────────────────────────────

require curl
require tar

OS="$(detect_os)"
ARCH="$(detect_arch)"

# On macOS prefer ~/.local/bin (user-writable, no sudo needed).
# On other platforms fall back to /usr/local/bin.
# Both can be overridden by setting INSTALL_DIR before running the script.
if [ -z "${INSTALL_DIR:-}" ]; then
  if [ "$OS" = "Darwin" ]; then
    INSTALL_DIR="${HOME}/.local/bin"
  else
    INSTALL_DIR="/usr/local/bin"
  fi
fi

# Create the install directory if it doesn't exist (safe for user-owned dirs).
if [ ! -d "$INSTALL_DIR" ]; then
  mkdir -p "$INSTALL_DIR"
fi

# Allow version override via env var
VERSION="${VERSION:-}"
if [ -z "$VERSION" ]; then
  info "Resolving latest release…"
  VERSION="$(resolve_version)"
fi

# Strip leading 'v' for the filename (GoReleaser uses the bare version in filenames)
VERSION_NUM="${VERSION#v}"

# Build the archive filename
if [ "$OS" = "Windows" ]; then
  ARCHIVE="neo4j-cli_${VERSION_NUM}_${OS}_${ARCH}.zip"
  require unzip
else
  ARCHIVE="neo4j-cli_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
fi

CHECKSUM_FILE="neo4j-cli_${VERSION_NUM}_checksums.txt"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

info "Installing ${BOLD}neo4j-cli ${VERSION}${RESET} (${OS}/${ARCH})"
info "Download URL: ${BASE_URL}/${ARCHIVE}"

# ── Download ──────────────────────────────────
info "Downloading archive…"
curl -fsSL --progress-bar "${BASE_URL}/${ARCHIVE}" -o "${TMP_DIR}/${ARCHIVE}"

info "Downloading checksums…"
curl -fsSL "${BASE_URL}/${CHECKSUM_FILE}" -o "${TMP_DIR}/${CHECKSUM_FILE}"

# ── Verify checksum ───────────────────────────
info "Verifying checksum…"
cd "$TMP_DIR"

if command -v sha256sum &>/dev/null; then
  grep "${ARCHIVE}" "${CHECKSUM_FILE}" | sha256sum -c - >/dev/null \
    || error "Checksum verification FAILED for ${ARCHIVE}"
elif command -v shasum &>/dev/null; then
  grep "${ARCHIVE}" "${CHECKSUM_FILE}" | shasum -a 256 -c - >/dev/null \
    || error "Checksum verification FAILED for ${ARCHIVE}"
else
  warn "No sha256sum or shasum found — skipping checksum verification"
fi
success "Checksum OK"
cd - >/dev/null

# ── Extract ───────────────────────────────────
info "Extracting…"
if [ "$OS" = "Windows" ]; then
  unzip -q "${TMP_DIR}/${ARCHIVE}" -d "${TMP_DIR}/extracted"
else
  tar -xzf "${TMP_DIR}/${ARCHIVE}" -C "${TMP_DIR}"
fi

# ── Install ───────────────────────────────────
EXTRACTED_BINARY="${TMP_DIR}/${BINARY_NAME}"
if [ ! -f "$EXTRACTED_BINARY" ]; then
  # Some GoReleaser configs nest in a subdirectory
  EXTRACTED_BINARY="$(find "${TMP_DIR}" -type f -name "${BINARY_NAME}" | head -1)"
fi
[ -f "$EXTRACTED_BINARY" ] || error "Binary '${BINARY_NAME}' not found in archive"

chmod +x "$EXTRACTED_BINARY"

# Check if install dir is writable; if not, use sudo
if [ -w "$INSTALL_DIR" ]; then
  mv "$EXTRACTED_BINARY" "${INSTALL_DIR}/${BINARY_NAME}"
else
  info "Install dir ${INSTALL_DIR} requires elevated permissions, using sudo…"
  sudo mv "$EXTRACTED_BINARY" "${INSTALL_DIR}/${BINARY_NAME}"
fi

# ── Confirm ───────────────────────────────────
INSTALLED_PATH="${INSTALL_DIR}/${BINARY_NAME}"
success "Installed → ${BOLD}${INSTALLED_PATH}${RESET}"

echo ""
"$INSTALLED_PATH" --version 2>/dev/null || true

# ── PATH check ────────────────────────────────
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*)
    ;;
  *)
    echo ""
    warn "${INSTALL_DIR} is not in your PATH."
    # Suggest the right rc file and command for the running shell
    case "${SHELL:-}" in
      */fish)
        echo "  Add it permanently with:"
        echo "    fish_add_path ${INSTALL_DIR}"
        ;;
      */zsh)
        echo "  Add it to ~/.zshrc with:"
        echo "    echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.zshrc"
        echo "  Then reload: source ~/.zshrc"
        ;;
      */bash)
        echo "  Add it to ~/.bash_profile with:"
        echo "    echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.bash_profile"
        echo "  Then reload: source ~/.bash_profile"
        ;;
      *)
        echo "  Add it to your shell rc file with:"
        echo "    export PATH=\"${INSTALL_DIR}:\$PATH\""
        ;;
    esac
    ;;
esac