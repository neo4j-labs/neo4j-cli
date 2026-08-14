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

# Behavioral tests for enforce-claude-verdict.sh
#
# Local run:
#   bats .github/scripts/tests/enforce-claude-verdict.bats
#
# Requirements:
#   bats-core >= 1.5.0  (brew install bats-core)
#   jq >= 1.6

bats_require_minimum_version 1.5.0

SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/enforce-claude-verdict.sh"
FIXTURES_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)/fixtures"

# ---------------------------------------------------------------------------
# setup / teardown
# ---------------------------------------------------------------------------

setup() {
  TEST_TMP="$(mktemp -d)"

  STUBS_DIR="${TEST_TMP}/stubs"
  mkdir -p "${STUBS_DIR}"

  # gh stub — emits the same shape as `gh api --paginate --slurp`:
  # an array of pages, [[{...},{...}],[{...}]].
  # Each test writes its response to GH_STUB_RESPONSE_FILE.
  cat >"${STUBS_DIR}/gh" <<'STUB'
#!/usr/bin/env bash
# Extract the API path from args to validate the call pattern
found=0
for arg in "$@"; do
  if [[ "$arg" == *"repos/"*"/issues/"*"/comments"* ]]; then
    found=1
    break
  fi
done
if [[ "$found" -eq 0 ]]; then
  echo "Unexpected gh call: $*" >&2
  exit 1
fi
if [[ -n "${GH_STUB_RESPONSE_FILE:-}" && -f "$GH_STUB_RESPONSE_FILE" ]]; then
  cat "$GH_STUB_RESPONSE_FILE"
  exit 0
fi
# Default empty response (no comments)
echo '[]'
exit 0
STUB
  chmod +x "${STUBS_DIR}/gh"

  export STUBS_DIR TEST_TMP
}

teardown() {
  rm -rf "${TEST_TMP}"
}

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# Write a gh response file with one page containing one claude[bot] comment.
# Args: body [created_at [login]]
write_gh_response() {
  local body="$1"
  local created_at="${2:-2024-01-01T00:00:00Z}"
  local login="${3:-claude[bot]}"
  local escaped
  escaped="$(printf '%s\n' "$body" | jq -Rs '.')"
  cat > "${TEST_TMP}/gh_response.json" <<EOF
[[
  {"user":{"login":"${login}"},"body":${escaped},"created_at":"${created_at}"}
]]
EOF
}

# Write a paginated response (two pages) with the claude[bot] comment on page 2.
# Args: body [created_at]
write_paginated_gh_response() {
  local body="$1"
  local created_at="${2:-2024-01-01T00:00:00Z}"
  local escaped
  escaped="$(printf '%s\n' "$body" | jq -Rs '.')"
  cat > "${TEST_TMP}/gh_response.json" <<EOF
[
  [{"user":{"login":"someone-else"},"body":"Regular comment","created_at":"2024-01-01T00:00:00Z"}],
  [{"user":{"login":"claude[bot]"},"body":${escaped},"created_at":"${created_at}"}]
]
EOF
}

# Write a response with two claude[bot] comments (different timestamps).
# Args: old_body old_created_at new_body new_created_at
write_dual_comment_response() {
  local old_body="$1"
  local old_ts="$2"
  local new_body="$3"
  local new_ts="$4"
  local old_escaped new_escaped
  old_escaped="$(printf '%s\n' "$old_body" | jq -Rs '.')"
  new_escaped="$(printf '%s\n' "$new_body" | jq -Rs '.')"
  cat > "${TEST_TMP}/gh_response.json" <<EOF
[[
  {"user":{"login":"claude[bot]"},"body":${old_escaped},"created_at":"${old_ts}"},
  {"user":{"login":"claude[bot]"},"body":${new_escaped},"created_at":"${new_ts}"}
]]
EOF
}

# Run the verdict script with standard env vars.
run_verdict() {
  PATH="${STUBS_DIR}:${PATH}" \
    REVIEW_KIND="${REVIEW_KIND:-Security review}" \
    PR_NUMBER="${PR_NUMBER:-123}" \
    GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-neo4j-labs/neo4j-cli}" \
    GH_TOKEN="${GH_TOKEN:-fake-token}" \
    VERDICT_FILE="${VERDICT_FILE:-${TEST_TMP}/verdict.txt}" \
    bash "${SCRIPT}" 2>&1
}

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

# case 1: verdict file 'pass' exits 0, logs tier 1
@test "case 1: verdict file pass" {
  echo "pass" > "${TEST_TMP}/verdict.txt"
  run run_verdict
  echo "$output"
  [ "$status" -eq 0 ]
  grep -qF "Tier 1 (verdict file)" <<< "$output"
}

# case 2: verdict file 'fail' exits 1
@test "case 2: verdict file fail" {
  echo "fail" > "${TEST_TMP}/verdict.txt"
  run run_verdict
  echo "$output"
  [ "$status" -eq 1 ]
}

# case 3: no verdict file, marker line without checkmark passes -- CLI-232 regression lock
@test "case 3: marker line without checkmark passes" {
  write_gh_response "**Security review:** no issues found"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 0 ]
}

# case 4: no verdict file, marker line with checkmark passes
@test "case 4: marker line with checkmark passes" {
  write_gh_response "**Security review:** ✅ no issues found"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 0 ]
}

# case 5: no verdict file, marker line with warning emoji fails
@test "case 5: marker line with warning emoji fails" {
  write_gh_response "**Security review:** ⚠️ 3 issue(s) flagged inline"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 1 ]
}

# case 6: no verdict file, no-security-relevant-changes passes
@test "case 6: no security-relevant changes passes" {
  write_gh_response "**Security review:** no security-relevant changes"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 0 ]
}

# case 7: analysis prose contains ⚠️ but marker line is clean — whole-body-grep defect fix
@test "case 7: prose contains warning emoji but marker line clean passes" {
  write_gh_response "$(printf 'Some analysis text mentioning ⚠️ issues.\n**Security review:** ✅ no issues found')"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 0 ]
}

# case 8: HTML comment 'pass' overrides garbled marker line
@test "case 8: HTML comment pass overrides garbled marker line" {
  write_gh_response "$(printf '**Security review:** some garbled nonsense here\n<!-- claude-verdict: pass -->')"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 0 ]
}

# case 9: HTML comment 'fail' overrides "no issues found"
@test "case 9: HTML comment fail overrides no issues found" {
  write_gh_response "$(printf '**Security review:** ✅ no issues found\n<!-- claude-verdict: fail -->')"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 1 ]
}

# case 10: no verdict file, no recognizable marker line, no HTML marker
@test "case 10: unparseable comment fails closed" {
  write_gh_response "This is a regular comment with no verdict information whatsoever"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 1 ]
}

# case 11: no claude[bot] comment at all
@test "case 11: no claude bot comment fails closed" {
  # Empty gh response (no comments at all)
  echo '[[{}]]' > "${TEST_TMP}/gh_response.json"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 1 ]
}

# case 12: marker comment only on page 2 of a paginated response
@test "case 12: comment on page 2 of paginated response passes" {
  write_paginated_gh_response "**Security review:** ✅ no issues found"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 0 ]
}

# case 13: CRLF body is handled correctly
@test "case 13: CRLF body passes" {
  body="$(printf 'summary text\r\n**Security review:** ✅ no issues found\r\n')"
  write_gh_response "$body"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 0 ]
}

# case 14: two claude[bot] summaries; newest is fail
@test "case 14: two summaries newest fail fails" {
  write_dual_comment_response \
    "**Security review:** ✅ no issues found" "2024-01-01T00:00:00Z" \
    "**Security review:** ⚠️ 2 issues found" "2024-01-02T00:00:00Z"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 1 ]
}

# case 15: mixed line with both emojis — fail-first ordering
@test "case 15: mixed checkmark and warning emoji fails (fail-first)" {
  write_gh_response "**Security review:** ✅ ⚠️ mixed"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 1 ]
}

# case 16: missing GH_TOKEN
@test "case 16: missing GH_TOKEN exits 1 and names the var" {
  run env -u GH_TOKEN \
    PATH="${STUBS_DIR}:${PATH}" \
    REVIEW_KIND="Security review" \
    PR_NUMBER="123" \
    GITHUB_REPOSITORY="neo4j-labs/neo4j-cli" \
    bash "${SCRIPT}" 2>&1
  echo "$output"
  [ "$status" -eq 1 ]
  grep -qF "GH_TOKEN" <<< "$output"
}

# case 17: missing PR_NUMBER
@test "case 17: missing PR_NUMBER exits 1 and names the var" {
  run env -u PR_NUMBER \
    PATH="${STUBS_DIR}:${PATH}" \
    REVIEW_KIND="Security review" \
    GITHUB_REPOSITORY="neo4j-labs/neo4j-cli" \
    GH_TOKEN="fake-token" \
    bash "${SCRIPT}" 2>&1
  echo "$output"
  [ "$status" -eq 1 ]
  grep -qF "PR_NUMBER" <<< "$output"
}

# case 18: Conventions review kind end-to-end
@test "case 18: Conventions review kind end-to-end passes" {
  write_gh_response "**Conventions review:** ✅ no issues found"
  REVIEW_KIND="Conventions review" \
    GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 0 ]
}

# VERDICT_BOT_LOGIN must match the identity the review step posts as. When the
# github_token escape hatch is used, comments come from github-actions[bot] and
# the default claude[bot] filter would fail-close on a review that really ran.
@test "VERDICT_BOT_LOGIN override trusts the matching author" {
  write_gh_response "**Security review:** ✅ no issues found" \
    "2024-01-01T00:00:00Z" "github-actions[bot]"
  VERDICT_BOT_LOGIN="github-actions[bot]" \
    GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 0 ]
}

@test "default bot login ignores a non-claude[bot] author (fail-closed)" {
  write_gh_response "**Security review:** ✅ no issues found" \
    "2024-01-01T00:00:00Z" "github-actions[bot]"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 1 ]
  [[ "$output" == *"claude[bot]"* ]]
}

# fixture replay: pr239-summary-no-checkmark.md verbatim through the gate
@test "fixture: pr239 summary without checkmark passes" {
  fixture_body="$(cat "${FIXTURES_DIR}/pr239-summary-no-checkmark.md")"
  write_gh_response "$fixture_body"
  GH_STUB_RESPONSE_FILE="${TEST_TMP}/gh_response.json" run run_verdict
  echo "$output"
  [ "$status" -eq 0 ]
}
