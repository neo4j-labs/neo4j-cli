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
#   1. VERDICT_FILE (default claude-verdict.txt, workspace-relative)
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
# When Tier 1 finds no verdict file, the comment fetch is
# retried: the review agent has been observed to exit 0
# before its summary comment lands, and on a re-push the
# newest marker comment can still be the previous push's.
# The fetch repeats up to POLL_ATTEMPTS times, sleeping
# POLL_INTERVAL_SECONDS between fetches, until a marker
# comment acceptable for the current head appears (a hit on
# the first fetch adds no delay).
#
# Env vars (required): REVIEW_KIND, PR_NUMBER,
#                      GITHUB_REPOSITORY, GH_TOKEN
# Env var (optional):  MODE — "detect" runs the same tier
#                      cascade and writes exactly one line,
#                      verdict=pass|fail|none, to $GITHUB_OUTPUT
#                      (none = no tier produced a verdict),
#                      exiting 0 for all three verdict outcomes.
#                      Infrastructure failures still exit 1:
#                      missing required env vars, missing gh/jq,
#                      or a gh api failure. Any other value (or
#                      unset) keeps the fail-closed enforce
#                      semantics. In detect mode an unset
#                      GITHUB_OUTPUT falls back to /dev/stdout
#                      (local runs); enforce mode never touches
#                      GITHUB_OUTPUT.
# Env var (optional):  HEAD_SHA — when non-empty, a marker
#                      comment is accepted only if its body
#                      carries <!-- claude-head-sha: <sha> -->
#                      with sha equal to HEAD_SHA
#                      (case-insensitive hex match). A missing
#                      or mismatched sha counts as absent.
#                      Empty/unset skips the check (legacy).
# Env var (optional):  POLL_ATTEMPTS (default 6) — maximum
#                      number of comment fetches when Tier 1
#                      finds no verdict file.
# Env var (optional):  POLL_INTERVAL_SECONDS (default 15) —
#                      sleep between fetches.
# Env var (optional):  VERDICT_FILE
#                      (default claude-verdict.txt, relative
#                      to the workspace root). Workspace-relative
#                      on purpose: the review agent's sandbox
#                      auto-approves writes only inside the
#                      session working directory — /tmp is
#                      outside it and gets denied.
# Env var (optional):  VERDICT_BOT_LOGIN
#                      (default claude[bot]) — must match the
#                      identity the review step posts as
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

# MODE=detect reports the verdict instead of enforcing it. Any other value
# (including unset) keeps the fail-closed enforce semantics unchanged.
detect_mode=0
if [ "${MODE:-}" = "detect" ]; then
  detect_mode=1
  # GitHub Actions provides GITHUB_OUTPUT as a file path; fall back to
  # stdout for local runs so the verdict line is still observable.
  : "${GITHUB_OUTPUT:=/dev/stdout}"
fi

: "${VERDICT_FILE:=claude-verdict.txt}"

# Author whose comment carries the verdict. Defaults to the Claude GitHub App.
# Only override when the review step authenticates with a different identity
# (see the `github_token` note in the review workflows) — a wider value lets a
# third party author a comment the gate will trust.
: "${VERDICT_BOT_LOGIN:=claude[bot]}"

# Optional head-sha binding for comment verdicts; empty disables the check.
: "${HEAD_SHA:=}"

# Comment-fetch polling knobs (see header). Only used when Tier 1 is empty.
: "${POLL_ATTEMPTS:=6}"
: "${POLL_INTERVAL_SECONDS:=15}"

verdict=""

# --- Tier 1: verdict file ---
if [ -s "$VERDICT_FILE" ]; then
  verdict="$(tr -d '[:space:]' < "$VERDICT_FILE")"
  echo "Tier 1 (verdict file): verdict='${verdict}'"
fi

# --- Tiers 2-4: comment fetch (polled) and analysis ---
if [ -z "$verdict" ]; then
  body=""
  attempt=1
  while true; do
    # Fetch raw API JSON separately so a gh failure produces a clear message
    # instead of a bare set -e abort (REQ-NF-005).
    api_json=""
    if ! api_json="$(gh api "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments?per_page=100" \
                      --paginate --slurp)"; then
      echo "::error::Failed to fetch PR comments from GitHub API"
      exit 1
    fi

    # Extract the latest review-bot body containing the kind marker.
    # --slurp + --paginate yields [[page1...],[page2...]] hence .[][] to flatten.
    # --arg passes the marker to jq; never shell-interpolate REVIEW_KIND into a
    # jq filter string (quoting/injection hazard).
    if ! body="$(printf '%s\n' "$api_json" | jq -r --arg marker "**${REVIEW_KIND}:**" \
                                                --arg login "$VERDICT_BOT_LOGIN" '
      [ .[][] | select(.user.login == $login
                       and (.body | contains($marker))) ]
      | sort_by(.created_at) | last | .body // ""
    ')"; then
      echo "::error::Failed to parse PR comments with jq"
      exit 1
    fi

    # Head-sha binding: when HEAD_SHA is set, the selected marker comment is
    # acceptable only if it carries <!-- claude-head-sha: <HEAD_SHA> -->.
    if [ -n "$body" ] && [ -n "$HEAD_SHA" ]; then
      # grep -iE with hardcoded regex is safe here (not built from user data).
      # || true guards against the bash 3.2 (macOS runner) set -e subshell
      # abort on no-match, same as the tier 2 pipeline below.
      sha_line="$(printf '%s\n' "$body" \
        | grep -iE '<!--[[:space:]]*claude-head-sha:[[:space:]]*[0-9a-f]+[[:space:]]*-->' \
        | tail -1 || true)"
      if [ -z "$sha_line" ]; then
        echo "marker comment has no claude-head-sha, treating as absent"
        body=""
      else
        # Lowercase before extracting so the hex compare is case-insensitive
        # (BSD sed has no /I flag, so fold case with tr instead).
        comment_sha="$(printf '%s\n' "$sha_line" \
          | tr '[:upper:]' '[:lower:]' \
          | sed -E 's/.*claude-head-sha:[[:space:]]*([0-9a-f]+)[[:space:]]*-->.*/\1/')"
        head_sha_lower="$(printf '%s\n' "$HEAD_SHA" | tr '[:upper:]' '[:lower:]')"
        if [ "$comment_sha" != "$head_sha_lower" ]; then
          echo "stale comment from sha ${comment_sha}, current HEAD_SHA ${HEAD_SHA}"
          body=""
        fi
      fi
    fi

    if [ -n "$body" ]; then
      break
    fi
    if [ "$attempt" -ge "$POLL_ATTEMPTS" ]; then
      break
    fi
    echo "no current-head ${VERDICT_BOT_LOGIN} comment with marker '**${REVIEW_KIND}:**' yet (attempt ${attempt}/${POLL_ATTEMPTS}); retrying in ${POLL_INTERVAL_SECONDS}s"
    sleep "$POLL_INTERVAL_SECONDS"
    attempt=$((attempt + 1))
  done

  if [ -z "$body" ]; then
    if [ "$detect_mode" -eq 0 ]; then
      echo "::error::Claude ${REVIEW_KIND} agent produced no verdict file and no current-head summary comment from ${VERDICT_BOT_LOGIN} with marker '**${REVIEW_KIND}:**'. Re-run the job."
      exit 1
    fi
    echo "no current-head ${VERDICT_BOT_LOGIN} comment with marker '**${REVIEW_KIND}:**' found"
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

# --- Tier 4: no tier produced a verdict ---
if [ -z "$verdict" ]; then
  # Enforce mode only; detect mode reports verdict=none in the final block.
  if [ "$detect_mode" -eq 0 ]; then
    if [ -n "${unclassified_line:-}" ]; then
      echo "::error::Marker line did not yield a known verdict: ${unclassified_line}"
    else
      echo "::error::No line starting with marker '**${REVIEW_KIND}:**' in ${VERDICT_BOT_LOGIN} comment"
    fi
    exit 1
  fi
fi

# --- Final verdict ---
if [ "$detect_mode" -eq 1 ]; then
  # Exactly one verdict line for the caller. An empty verdict means no tier
  # produced one (none); any non-pass Tier 1 content maps to fail because
  # enforce mode would fail on it too.
  case "$verdict" in
    pass) echo "verdict=pass" >> "$GITHUB_OUTPUT" ;;
    "")   echo "verdict=none" >> "$GITHUB_OUTPUT" ;;
    *)    echo "verdict=fail" >> "$GITHUB_OUTPUT" ;;
  esac
  exit 0
fi

if [ "$verdict" = "pass" ]; then
  exit 0
fi
echo "::error::Claude ${REVIEW_KIND} did not produce a pass verdict (verdict='${verdict}')"
exit 1
