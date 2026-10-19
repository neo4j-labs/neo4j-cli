#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# publish-gh-pages.sh
#
# Commits everything a CI job changed in its `gh-pages` checkout
# and pushes it, rebasing onto whatever else was pushed in the
# meantime.
#
# Several workflows publish to gh-pages, each owning its own
# files, and the site is also updated by hand through the prompt
# flow (AGENTS.md "Website (neo4j.sh)"). A job's checkout can
# fall behind the branch before the job pushes, and a plain
# `git push` from a stale checkout is rejected: non-fast-forward,
# or "cannot lock ref" when two pushes land together.
#
# Policy:
#   - Stage everything in the checkout and commit it as the CI
#     bot. A caller that must not publish a file restores it
#     before calling. With nothing to commit, exit 0 unpushed.
#   - Push HEAD to origin's gh-pages. On a rejection, fetch the
#     current remote tip and rebase ONLY this job's commits (those
#     after the tip the checkout started from) onto it, then push
#     again: at most MAX_ATTEMPTS pushes, with a linear back-off.
#     Rebasing that explicit range, not the whole branch, keeps a
#     shallow (fetch-depth: 1) checkout correct.
#   - The writers own disjoint files, so the rebase is clean. A
#     failed rebase (a conflict means two writers changed the same
#     file) is aborted and the script fails rather than picking a
#     winner.
#   - Never force-pushes.
#
# Usage: publish-gh-pages.sh CHECKOUT_DIR MESSAGE
#   CHECKOUT_DIR is the gh-pages checkout (actions/checkout with
#   `ref: gh-pages`). Its `origin` remote with the persisted
#   checkout credentials is the push target, and
#   refs/remotes/origin/gh-pages marks the tip it started from.
#   MESSAGE is the commit message.
#
# Env:
#   PUBLISH_GH_PAGES_BACKOFF_SECONDS
#     Back-off unit in whole seconds (default 2): the retry after
#     attempt n waits n units. Tests set 0.
#
# Exit status: 0 = published, or nothing to publish; 1 = refused
# or gave up, with a `::error::` diagnostic.
#
# Adapted from neo4j-labs/cypherlite's push-gh-pages.sh (their
# v0.12.4 release race, PR #320).
# ============================================================

BRANCH=gh-pages
REMOTE=origin
TRACKING_REF="refs/remotes/${REMOTE}/${BRANCH}"
MAX_ATTEMPTS=5
BACKOFF_SECONDS="${PUBLISH_GH_PAGES_BACKOFF_SECONDS:-2}"

if [ "$#" -ne 2 ] || [ -z "${1:-}" ] || [ -z "${2:-}" ]; then
  echo "::error::Usage: publish-gh-pages.sh CHECKOUT_DIR MESSAGE"
  exit 1
fi
CHECKOUT_DIR="$1"
MESSAGE="$2"

if ! [[ "$BACKOFF_SECONDS" =~ ^[0-9]+$ ]]; then
  echo "::error::PUBLISH_GH_PAGES_BACKOFF_SECONDS '${BACKOFF_SECONDS}' is not a whole number of seconds"
  exit 1
fi

if [ ! -d "$CHECKOUT_DIR" ]; then
  echo "::error::Checkout directory '${CHECKOUT_DIR}' does not exist"
  exit 1
fi
cd "$CHECKOUT_DIR"

# The tip this checkout started from; this job's commits are base..HEAD.
if ! base="$(git rev-parse --verify --quiet "${TRACKING_REF}^{commit}" 2> /dev/null)"; then
  echo "::error::'${CHECKOUT_DIR}' is not a ${BRANCH} checkout: ${TRACKING_REF} not found"
  exit 1
fi

git add -A
if git diff --cached --quiet; then
  echo "[publish-gh-pages] nothing changed in ${CHECKOUT_DIR}; nothing to publish"
  exit 0
fi
git -c user.name="github-actions[bot]" \
  -c user.email="github-actions[bot]@users.noreply.github.com" \
  commit -q -m "$MESSAGE"

attempt=1
while true; do
  if git push "$REMOTE" "HEAD:refs/heads/${BRANCH}"; then
    echo "[publish-gh-pages] pushed $(git rev-parse --short HEAD) to ${BRANCH} (attempt ${attempt}/${MAX_ATTEMPTS})"
    exit 0
  fi

  if [ "$attempt" -ge "$MAX_ATTEMPTS" ]; then
    echo "::error::Push to ${BRANCH} still rejected after ${MAX_ATTEMPTS} attempts; giving up"
    exit 1
  fi

  echo "[publish-gh-pages] push rejected (attempt ${attempt}/${MAX_ATTEMPTS}); rebasing onto the current ${REMOTE}/${BRANCH}"
  sleep "$((attempt * BACKOFF_SECONDS))"

  if ! git fetch "$REMOTE" "+refs/heads/${BRANCH}:${TRACKING_REF}"; then
    echo "::error::Could not fetch ${REMOTE}/${BRANCH} to rebase onto"
    exit 1
  fi
  if ! git rebase --onto "$TRACKING_REF" "$base"; then
    # A conflict leaves a rebase to abort; a rebase that never started
    # leaves none, and a bare --abort would then exit non-zero under set -e.
    git rebase --abort 2> /dev/null || true
    echo "::error::Rebasing onto the current ${REMOTE}/${BRANCH} failed (a conflict means another run changed the same files); refusing to pick a winner"
    exit 1
  fi
  base="$(git rev-parse "$TRACKING_REF")"
  attempt=$((attempt + 1))
done
