# Copyright (c) "Neo4j"
# Neo4j Sweden AB [https://neo4j.com]
# This file is part of Neo4j.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Behavioral tests for the NEO4J_CLI_AUTO_INSTALL_SKILL feature in install-neo4j-cli.sh
#
# Local run:
#   bats distribution/installation-scripts/tests/install-neo4j-cli.bats
#
# Requirements:
#   bats-core >= 1.5.0  (brew install bats-core / apt-get install bats)

bats_require_minimum_version 1.5.0

SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/install-neo4j-cli.sh"

# ---------------------------------------------------------------------------
# setup / teardown
# ---------------------------------------------------------------------------

setup() {
  # Create an isolated temp directory for each test
  TEST_TMP="$(mktemp -d)"

  # Directory where the "binary" will be installed
  INSTALL_DIR="${TEST_TMP}/bin"
  mkdir -p "${INSTALL_DIR}"

  # File where the stub records invocations
  STUB_CALLS="${TEST_TMP}/stub_calls.txt"
  touch "${STUB_CALLS}"

  # Create a stubs directory and prepend it to PATH so every external command
  # the installer calls is intercepted.
  STUBS_DIR="${TEST_TMP}/stubs"
  mkdir -p "${STUBS_DIR}"

  # ── curl stub ─────────────────────────────────────────────────────────────
  # Parses the -o flag to find the output file path and creates a suitable
  # placeholder file. For the checksums file it writes a fake sha256 line so
  # the subsequent `grep | sha256sum -c` pipeline succeeds.
  #
  # The installer requests two files in this order:
  #   1. neo4j-cli_<ver>_<OS>_<arch>.tar.gz   (archive)
  #   2. neo4j-cli_<ver>_checksums.txt          (checksums)
  cat >"${STUBS_DIR}/curl" <<'EOF'
#!/usr/bin/env bash
outfile=""
args=("$@")
for ((i=0; i<${#args[@]}; i++)); do
  if [[ "${args[$i]}" == "-o" ]]; then
    outfile="${args[$((i+1))]}"
  fi
done

if [[ -n "$outfile" ]]; then
  if [[ "$outfile" == *checksums.txt ]]; then
    # Write a fake sha256 line so that `grep <archive> checksums.txt | sha256sum -c`
    # succeeds. The uname stub returns Linux/x86_64, so the archive name matches.
    # The sha256sum stub always exits 0, so the hash value itself does not matter.
    base="${outfile%_checksums.txt}"
    archive_name="${base##*/}_Linux_x86_64.tar.gz"
    echo "aabbccdd11223344aabbccdd11223344aabbccdd11223344aabbccdd11223344  ${archive_name}" >"$outfile"
  else
    touch "$outfile"
  fi
fi
exit 0
EOF
  chmod +x "${STUBS_DIR}/curl"

  # ── sha256sum stub ────────────────────────────────────────────────────────
  # Reads stdin (the grep output piped to it) and always exits 0.
  cat >"${STUBS_DIR}/sha256sum" <<'EOF'
#!/usr/bin/env bash
cat >/dev/null   # consume stdin
exit 0
EOF
  chmod +x "${STUBS_DIR}/sha256sum"

  # ── shasum stub ───────────────────────────────────────────────────────────
  cat >"${STUBS_DIR}/shasum" <<'EOF'
#!/usr/bin/env bash
cat >/dev/null   # consume stdin
exit 0
EOF
  chmod +x "${STUBS_DIR}/shasum"

  # ── tar stub ──────────────────────────────────────────────────────────────
  # The installer calls: tar -xzf <archive> -C <dir>
  # We create a fake neo4j-cli binary in the target directory that records
  # invocations (STUB_CALLS is exported so it is visible to child processes).
  # NEO4J_CLI_STUB_EXIT controls the exit code for skill subcommands.
  cat >"${STUBS_DIR}/tar" <<'STUB'
#!/usr/bin/env bash
target_dir=""
args=("$@")
for ((i=0; i<${#args[@]}; i++)); do
  if [[ "${args[$i]}" == "-C" ]]; then
    target_dir="${args[$((i+1))]}"
  fi
done
if [[ -n "$target_dir" ]]; then
  # Write a neo4j-cli stub that records invocations via the STUB_CALLS env var
  cat >"${target_dir}/neo4j-cli" <<'NEOSTUB'
#!/usr/bin/env bash
if [[ -n "${STUB_CALLS:-}" ]]; then
  echo "$*" >> "${STUB_CALLS}"
fi
if [[ "$1" == "skill" ]]; then
  exit "${NEO4J_CLI_STUB_EXIT:-0}"
fi
exit 0
NEOSTUB
  chmod +x "${target_dir}/neo4j-cli"
fi
exit 0
STUB
  chmod +x "${STUBS_DIR}/tar"

  # ── sudo stub ─────────────────────────────────────────────────────────────
  # The install dir is writable in tests so sudo should not be reached, but
  # stub it for safety.
  cat >"${STUBS_DIR}/sudo" <<'EOF'
#!/usr/bin/env bash
"$@"
EOF
  chmod +x "${STUBS_DIR}/sudo"

  # ── uname stub ────────────────────────────────────────────────────────────
  # Returns deterministic values so the OS/arch detection branches are stable.
  cat >"${STUBS_DIR}/uname" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == "-s" ]]; then echo "Linux"; else echo "x86_64"; fi
EOF
  chmod +x "${STUBS_DIR}/uname"

  # ── neo4j-cli stub (in INSTALL_DIR) ───────────────────────────────────────
  # The installer places the binary at "${INSTALL_DIR}/neo4j-cli" by mv-ing
  # the tar-extracted binary (which the tar stub creates with recording logic).
  # We also pre-populate INSTALL_DIR so that if mv fails for any reason the
  # test still has a stub present. Both stubs use the STUB_CALLS env var.
  cat >"${INSTALL_DIR}/neo4j-cli" <<'STUB'
#!/usr/bin/env bash
if [[ -n "${STUB_CALLS:-}" ]]; then
  echo "$*" >> "${STUB_CALLS}"
fi
if [[ "$1" == "skill" ]]; then
  exit "${NEO4J_CLI_STUB_EXIT:-0}"
fi
exit 0
STUB
  chmod +x "${INSTALL_DIR}/neo4j-cli"

  # Export for use inside run_installer
  export INSTALL_DIR STUB_CALLS STUBS_DIR TEST_TMP
}

teardown() {
  rm -rf "${TEST_TMP}"
}

# ---------------------------------------------------------------------------
# Helper: runs the installer with the stub PATH prepended.
# All heavy lifting (curl, tar, sha256sum) is intercepted by stubs in
# STUBS_DIR. The binary "placement" step finds the stub pre-seeded in
# INSTALL_DIR (the mv merely overwrites it with the tar-stub output, which is
# also a do-nothing script named neo4j-cli).
# ---------------------------------------------------------------------------
run_installer() {
  local stub_path="${STUBS_DIR}:${INSTALL_DIR}:${PATH}"
  PATH="${stub_path}" \
    VERSION="v9.9.9" \
    INSTALL_DIR="${INSTALL_DIR}" \
    STUB_CALLS="${STUB_CALLS}" \
    NEO4J_CLI_STUB_EXIT="${NEO4J_CLI_STUB_EXIT:-0}" \
    bash "${SCRIPT}" 2>&1
}

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

# Test 1: When NEO4J_CLI_AUTO_INSTALL_SKILL=1, `skill install --rw` is called.
@test "NEO4J_CLI_AUTO_INSTALL_SKILL=1: skill install --rw is invoked" {
  NEO4J_CLI_AUTO_INSTALL_SKILL=1 run run_installer

  # The installer must succeed
  [ "$status" -eq 0 ]

  # The stub call file must contain the skill install invocation
  run grep -F "skill install --rw" "${STUB_CALLS}"
  [ "$status" -eq 0 ]
}

# Test 2a: Env var unset → no skill install.
@test "NEO4J_CLI_AUTO_INSTALL_SKILL unset: skill install is not called" {
  unset NEO4J_CLI_AUTO_INSTALL_SKILL
  run run_installer

  [ "$status" -eq 0 ]

  # STUB_CALLS should not contain skill install
  if grep -qF "skill install" "${STUB_CALLS}"; then
    echo "FAIL: skill install was called but should not have been"
    false
  fi
}

# Test 2b: Env var =0 → no skill install.
@test "NEO4J_CLI_AUTO_INSTALL_SKILL=0: skill install is not called" {
  NEO4J_CLI_AUTO_INSTALL_SKILL=0 run run_installer

  [ "$status" -eq 0 ]

  if grep -qF "skill install" "${STUB_CALLS}"; then
    echo "FAIL: skill install was called but should not have been"
    false
  fi
}

# Test: install output signposts the feedback/issues URL.
@test "install output contains the feedback/issues URL" {
  unset NEO4J_CLI_AUTO_INSTALL_SKILL
  run run_installer

  [ "$status" -eq 0 ]

  [[ "$output" == *"neo4j-labs/neo4j-cli/issues"* ]]
}

# Test 3: =1 but skill install exits non-zero → installer still exits 0.
@test "NEO4J_CLI_AUTO_INSTALL_SKILL=1 and skill install fails: installer still exits 0" {
  # NEO4J_CLI_STUB_EXIT=1 makes the stub exit 1 for skill subcommands.
  NEO4J_CLI_AUTO_INSTALL_SKILL=1 NEO4J_CLI_STUB_EXIT=1 run run_installer

  # Install script must exit 0 even though skill install returned 1
  [ "$status" -eq 0 ]

  # skill install was attempted
  run grep -F "skill install --rw" "${STUB_CALLS}"
  [ "$status" -eq 0 ]
}
