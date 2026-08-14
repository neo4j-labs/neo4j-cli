#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C.UTF-8

# ============================================================
# enforce-claude-verdict.sh
#
# Shared verdict gate for Claude review workflows (security
# and conventions). Fail-closed: a missing or unparseable
# verdict fails the check so an early Claude exit or tool
# denial cannot silently pass.
#
# Fallback tiers (most exact to most tolerant):
#   1. VERDICT_FILE (default /tmp/claude-verdict.txt)
#   2. HTML comment <!-- claude-verdict: pass|fail -->
#   3. Marker-line phrase match (single-line only)
#   4. Fail-closed (::error:: + exit 1)
#
# Both workflows have observed Claude post the mandated
# summary comment but skip the verdict-file write (PR #239,
# PR #132). The summary carries the review-kind marker line
# and may include the HTML comment; this script falls
# through until it reaches a verdict or exits 1.
#
# Env vars (required): REVIEW_KIND, PR_NUMBER,
#                      GITHUB_REPOSITORY, GH_TOKEN
# Env var (optional):  VERDICT_FILE
#                      (default /tmp/claude-verdict.txt)
# ============================================================

# --- Input validation ---
for var in REVIEW_KIND PR_NUMBER GITHUB_REPOSITORY GH_TOKEN; do
  if [ -z "${!var:-}" ]; then
    echo "::error::Required env var ${var} is unset or empty"
    exit 1
  fi
done

for cmd in gh jq; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "::error::Required command '${cmd}' not found on PATH"
    exit 1
  fi
done

: "${VERDICT_FILE:=/tmp/claude-verdict.txt}"

verdict=""

# --- Tier 1: verdict file ---
if [ -s "$VERDICT_FILE" ]; then
  verdict="$(tr -d '[:space:]' < "$VERDICT_FILE")"
  echo "Tier 1 (verdict file): verdict='${verdict}'"
fi

# --- Tiers 2-4: comment fetch and analysis ---
if [ -z "$verdict" ]; then
  # Fetch raw API JSON separately so a gh failure produces a clear message
  # instead of a bare set -e abort (REQ-NF-005).
  api_json=""
  if ! api_json="$(gh api "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments?per_page=100" \
                    --paginate --slurp)"; then
    echo "::error::Failed to fetch PR comments from GitHub API"
    exit 1
  fi

  # Extract the latest claude[bot] body containing the kind marker.
  # --slurp + --paginate yields [[page1...],[page2...]] hence .[][] to flatten.
  # --arg passes the marker to jq; never shell-interpolate REVIEW_KIND into a
  # jq filter string (quoting/injection hazard).
  body=""
  if ! body="$(printf '%s\n' "$api_json" | jq -r --arg marker "**${REVIEW_KIND}:**" '
    [ .[][] | select(.user.login == "claude[bot]"
                     and (.body | contains($marker))) ]
    | sort_by(.created_at) | last | .body // ""
  ')"; then
    echo "::error::Failed to parse PR comments with jq"
    exit 1
  fi

  if [ -z "$body" ]; then
    echo "::error::No claude[bot] comment with marker '**${REVIEW_KIND}:**' found"
    exit 1
  fi

  # CR-strip once; share across tier 2 and tier 3.
  # A comment body is attacker-influenceable, so use printf (not echo),
  # never eval, and never word-split (REQ-F-009).
  clean_body="$(printf '%s\n' "$body" | tr -d '\r')"

  # --- Tier 2: machine-readable HTML comment ---
  # grep -iE with hardcoded regex is safe here (not built from user data).
  # grep returns 1 on no-match, which with set -euo pipefail aborts a $()
  # subshell on bash 3.2 (macOS runner). || true keeps the assignment safe.
  tier2_line="$(printf '%s\n' "$clean_body" \
    | grep -iE '<!--[[:space:]]*claude-verdict:[[:space:]]*(pass|fail)[[:space:]]*-->' \
    | tail -1 || true)"
  if [ -n "$tier2_line" ]; then
    # grep -oE also returns 1 when the pattern is absent. The pipeline is inside
    # a $() subshell, so || true prevents silent set -e abort on bash 3.2.
    verdict="$(printf '%s\n' "$tier2_line" \
      | tr '[:upper:]' '[:lower:]' \
      | grep -oE 'pass|fail' | head -1 || true)"
    echo "Tier 2 (HTML comment): verdict='${verdict}'"
  fi

  # --- Tier 3: marker-line phrase match ---
  if [ -z "$verdict" ]; then
    # grep -F treats REVIEW_KIND as a literal (its ** are not regex meta-
    # characters under -F). Then check the line actually starts with the
    # marker via parameter-expansion prefix removal (no regex from the
    # body, no eval, no word-splitting).
    marker_prefix="**${REVIEW_KIND}:**"
    last_marker_line=""
    while IFS= read -r line; do
      # Remove leading whitespace
      trimmed="${line#"${line%%[![:space:]]*}"}"
      # Check prefix: if removing the prefix changes the value, it was present
      rest="${trimmed#"$marker_prefix"}"
      if [ "$rest" != "$trimmed" ]; then
        last_marker_line="$line"
      fi
    done < <(printf '%s\n' "$clean_body" | grep -F "$marker_prefix")

    if [ -n "$last_marker_line" ]; then
      # Strip leading whitespace for classification (single line only —
      # the whole-body grep is the defect this fixes).
      trimmed="${last_marker_line#"${last_marker_line%%[![:space:]]*}"}"
      # Fail indicators tested first
      if printf '%s\n' "$trimmed" | grep -qF '⚠'; then
        verdict="fail"
        echo "Tier 3 (marker line): verdict='fail' (contains ⚠)"
      elif printf '%s\n' "$trimmed" | grep -qiF 'flagged inline'; then
        verdict="fail"
        echo "Tier 3 (marker line): verdict='fail' (contains 'flagged inline')"
      elif printf '%s\n' "$trimmed" | grep -qiF 'no issues found'; then
        verdict="pass"
        echo "Tier 3 (marker line): verdict='pass' (contains 'no issues found')"
      elif printf '%s\n' "$trimmed" | grep -qiF 'no security-relevant changes'; then
        verdict="pass"
        echo "Tier 3 (marker line): verdict='pass' (contains 'no security-relevant changes')"
      elif printf '%s\n' "$trimmed" | grep -qiF 'no convention-relevant changes'; then
        verdict="pass"
        echo "Tier 3 (marker line): verdict='pass' (contains 'no convention-relevant changes')"
      elif printf '%s\n' "$trimmed" | grep -qF '✅'; then
        verdict="pass"
        echo "Tier 3 (marker line): verdict='pass' (contains ✅)"
      else
        echo "Tier 3 (marker line): unclassified — '${trimmed}'"
        unclassified_line="$trimmed"
      fi
    else
      echo "Tier 3 (marker line): no line starting with marker found"
    fi
  fi
fi

# --- Tier 4: fail-closed ---
if [ -z "$verdict" ]; then
  if [ -z "${body:-}" ]; then
    echo "::error::No claude[bot] comment with marker '**${REVIEW_KIND}:**' found"
  elif [ -n "${unclassified_line:-}" ]; then
    echo "::error::Marker line did not yield a known verdict: ${unclassified_line}"
  else
    echo "::error::No line starting with marker '**${REVIEW_KIND}:**' in claude[bot] comment"
  fi
  exit 1
fi

# --- Final verdict ---
if [ "$verdict" = "pass" ]; then
  exit 0
fi
echo "::error::Claude ${REVIEW_KIND} did not produce a pass verdict (verdict='${verdict}')"
exit 1
