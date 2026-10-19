#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# push-gh-pages.sh
#
# Pushes the commits a CI job made in its `gh-pages` checkout,
# rebasing them onto whatever else pushed in the meantime.
#
# update-website.yml and update-releases-json.yml are the
# automated gh-pages writers. Both trigger on the same release,
# the website run takes minutes (the website-update step) between
# the gh-pages checkout and the push, and the site is also updated
# by hand through the prompt flow (AGENTS.md "Website
# (neo4j.sh)": worktree + commit + push on gh-pages). A plain
# `git push` from a checkout the branch has since left behind is
# rejected — non-fast-forward, or "cannot lock ref" when two
# pushes land together — and releases.json / the site stay stale
# until someone re-dispatches the workflow.
#
# Policy:
#   - Push HEAD to origin's gh-pages. On a rejection, fetch the
#     current remote tip and rebase ONLY this job's commits (those
#     after the tip the checkout started from) onto it, then push
#     again: at most MAX_ATTEMPTS pushes, with a linear back-off.
#     Rebasing that explicit range, not the whole branch, keeps a
#     shallow (fetch-depth: 1) checkout correct.
#   - The writers own disjoint files, so the rebase is clean. A
#     conflict means two runs changed the same file: the rebase is
#     aborted and the script fails rather than picking a winner.
#   - Never force-pushes.
#
# Usage: push-gh-pages.sh CHECKOUT_DIR
#   CHECKOUT_DIR is the gh-pages checkout (actions/checkout with
#   `ref: gh-pages`). Its `origin` remote with the persisted
#   checkout credentials is the push target, and
#   refs/remotes/origin/gh-pages marks the tip it started from.
#
# Env:
#   PUSH_GH_PAGES_BACKOFF_SECONDS
#     Back-off unit in whole seconds (default 2): the retry after
#     attempt n waits n units. Tests set 0.
#
# Exit status: 0 = pushed; 1 = refused or gave up, with a
# `::error::` diagnostic.
#
# Adapted from neo4j-labs/cypherlite's same-named script (their
# v0.12.4 release race, PR #320).
# ============================================================

BRANCH=gh-pages
REMOTE=origin
TRACKING_REF="refs/remotes/${REMOTE}/${BRANCH}"
MAX_ATTEMPTS=5
BACKOFF_SECONDS="${PUSH_GH_PAGES_BACKOFF_SECONDS:-2}"

if [ "$#" -ne 1 ] || [ -z "${1:-}" ]; then
  echo "::error::Usage: push-gh-pages.sh CHECKOUT_DIR"
  exit 1
fi
CHECKOUT_DIR="$1"

if ! [[ "$BACKOFF_SECONDS" =~ ^[0-9]+$ ]]; then
  echo "::error::PUSH_GH_PAGES_BACKOFF_SECONDS '${BACKOFF_SECONDS}' is not a whole number of seconds"
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

attempt=1
while true; do
  if git push "$REMOTE" "HEAD:refs/heads/${BRANCH}"; then
    echo "[push-gh-pages] pushed $(git rev-parse --short HEAD) to ${BRANCH} (attempt ${attempt}/${MAX_ATTEMPTS})"
    exit 0
  fi

  if [ "$attempt" -ge "$MAX_ATTEMPTS" ]; then
    echo "::error::Push to ${BRANCH} still rejected after ${MAX_ATTEMPTS} attempts; giving up"
    exit 1
  fi

  echo "[push-gh-pages] push rejected (attempt ${attempt}/${MAX_ATTEMPTS}); rebasing onto the current ${REMOTE}/${BRANCH}"
  sleep "$((attempt * BACKOFF_SECONDS))"

  if ! git fetch "$REMOTE" "+refs/heads/${BRANCH}:${TRACKING_REF}"; then
    echo "::error::Could not fetch ${REMOTE}/${BRANCH} to rebase onto"
    exit 1
  fi
  if ! git rebase --onto "$TRACKING_REF" "$base"; then
    git rebase --abort
    echo "::error::Rebasing onto the current ${REMOTE}/${BRANCH} conflicts: another run changed the same files; refusing to pick a winner"
    exit 1
  fi
  base="$(git rev-parse "$TRACKING_REF")"
  attempt=$((attempt + 1))
done
