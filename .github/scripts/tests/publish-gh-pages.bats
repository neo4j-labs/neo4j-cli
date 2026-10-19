# Behavioral tests for publish-gh-pages.sh
#
# Covers the publish policy for CI writes to gh-pages. Everything a
# job changed in its checkout is committed in one commit; nothing
# changed means nothing is pushed. A push rejected because the
# branch advanced since checkout (another workflow, or a manual
# website update from AGENTS.md "Website (neo4j.sh)") rebases ONLY
# this job's commit onto the new tip and retries, at most 5 pushes.
# A failed rebase is aborted and fails without pushing, and the
# script never force-pushes.
#
# Hermetic: a local bare repository stands in for GitHub and each
# writer is a `--depth 1` clone of its gh-pages branch, the shape
# actions/checkout leaves with `ref: gh-pages, fetch-depth: 1`. A
# pre-receive hook on the bare repository simulates server-side
# rejections. The user's git config is ignored.
#
# Local run:
#   bats .github/scripts/tests/publish-gh-pages.bats
#
# Requirements:
#   bats-core >= 1.5.0  (brew install bats-core)
#   bash, git

bats_require_minimum_version 1.5.0

SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/publish-gh-pages.sh"

# ---------------------------------------------------------------------------
# setup / teardown
# ---------------------------------------------------------------------------

setup() {
  TEST_TMP="$(mktemp -d)"
  ORIGIN="${TEST_TMP}/origin.git"

  # Ignore the developer's git config (signing, pull.rebase, hooks, ...).
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
  export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
  export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com

  git init -q --bare -b gh-pages "${ORIGIN}"
  git init -q -b gh-pages "${TEST_TMP}/seed"
  printf '<html>v1</html>\n' > "${TEST_TMP}/seed/index.html"
  git -C "${TEST_TMP}/seed" add index.html
  git -C "${TEST_TMP}/seed" commit -q -m "seed"
  git -C "${TEST_TMP}/seed" push -q "${ORIGIN}" gh-pages

  export TEST_TMP ORIGIN
}

teardown() {
  rm -rf "${TEST_TMP}"
}

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

# A writer's gh-pages checkout, as actions/checkout leaves it.
# Args: NAME — prints the checkout path.
checkout() {
  local dir="${TEST_TMP}/$1"
  git clone -q --depth 1 --branch gh-pages "file://${ORIGIN}" "${dir}"
  printf '%s\n' "${dir}"
}

# Change one file in a checkout, as a CI job does before publishing.
# Args: DIR FILE CONTENT
write_file() {
  printf '%s\n' "$3" > "$1/$2"
}

# Another writer pushing straight to the remote. Args: NAME FILE CONTENT
concurrent_push() {
  local dir
  dir="$(checkout "$1")"
  write_file "${dir}" "$2" "$3"
  git -C "${dir}" add "$2"
  git -C "${dir}" commit -q -m "$1: $2"
  git -C "${dir}" push -q origin gh-pages
}

# Run the helper with no back-off. Args: DIR MESSAGE [NAME=VALUE ...]
run_publish() {
  local dir="$1" message="$2"
  shift 2
  env PUBLISH_GH_PAGES_BACKOFF_SECONDS=0 "$@" bash "${SCRIPT}" "${dir}" "${message}" 2>&1
}

remote_tip() {
  git --git-dir="${ORIGIN}" rev-parse gh-pages
}

remote_file() {
  git --git-dir="${ORIGIN}" show "gh-pages:$1"
}

remote_subject() {
  git --git-dir="${ORIGIN}" log -1 --format=%s gh-pages
}

# Install a pre-receive hook that rejects the first N pushes (all of
# them when N is "all").
reject_pushes() {
  cat > "${ORIGIN}/hooks/pre-receive" <<HOOK
#!/usr/bin/env bash
count_file="${TEST_TMP}/rejections"
count=\$(( \$(cat "\${count_file}" 2>/dev/null || echo 0) + 1 ))
echo "\${count}" > "\${count_file}"
if [ "$1" = all ] || [ "\${count}" -le "$1" ]; then
  echo "simulated rejection \${count}" >&2
  exit 1
fi
HOOK
  chmod +x "${ORIGIN}/hooks/pre-receive"
}

# ---------------------------------------------------------------------------
# Tests: publishes that land
# ---------------------------------------------------------------------------

@test "an unraced publish commits the change and lands on the first attempt" {
  dir="$(checkout writer)"
  write_file "${dir}" llms.txt 'v1'

  run run_publish "${dir}" "publish llms.txt"
  echo "$output"
  [ "$status" -eq 0 ]
  grep -qF "(attempt 1/5)" <<< "$output"
  [[ "$output" != *"rebasing"* ]]
  [ "$(remote_tip)" = "$(git -C "${dir}" rev-parse HEAD)" ]
  [ "$(remote_subject)" = "publish llms.txt" ]
  [ "$(remote_file llms.txt)" = 'v1' ]
}

@test "a checkout with nothing changed publishes nothing" {
  dir="$(checkout writer)"
  seed_tip="$(remote_tip)"

  run run_publish "${dir}" "publish nothing"
  echo "$output"
  [ "$status" -eq 0 ]
  grep -qF "nothing to publish" <<< "$output"
  [ "$(remote_tip)" = "${seed_tip}" ]
  [ "$(git -C "${dir}" rev-parse HEAD)" = "${seed_tip}" ]
}

@test "a publish raced by another writer rebases onto it and lands" {
  # Another writer lands between this job's checkout and its push: the
  # plain push is rejected.
  dir="$(checkout releases-json)"
  [ "$(git -C "${dir}" rev-parse --is-shallow-repository)" = true ]
  concurrent_push website index.html '<html>v2</html>'
  website_tip="$(remote_tip)"
  write_file "${dir}" releases.json '{"schema_version":1}'

  run run_publish "${dir}" "publish releases.json"
  echo "$output"
  [ "$status" -eq 0 ]
  grep -qF "push rejected (attempt 1/5)" <<< "$output"
  grep -qF "(attempt 2/5)" <<< "$output"

  # Linear history: this job's commit sits on top of the other writer's.
  [ "$(git --git-dir="${ORIGIN}" rev-parse gh-pages^)" = "${website_tip}" ]
  [ "$(remote_subject)" = "publish releases.json" ]
  [ -z "$(git --git-dir="${ORIGIN}" rev-list --merges gh-pages)" ]

  # Both writers' files are live.
  [ "$(remote_file releases.json)" = '{"schema_version":1}' ]
  [ "$(remote_file index.html)" = '<html>v2</html>' ]
}

@test "three writers from the same tip all land, one commit each" {
  install_dir="$(checkout install)"
  releases_dir="$(checkout releases)"
  website_dir="$(checkout website)"
  write_file "${install_dir}" install.sh '#!/bin/sh'
  write_file "${releases_dir}" releases.json '{}'
  write_file "${website_dir}" index.html '<html>v2</html>'

  for dir in "${install_dir}" "${releases_dir}" "${website_dir}"; do
    run run_publish "${dir}" "publish $(basename "${dir}")"
    echo "$output"
    [ "$status" -eq 0 ]
  done

  # seed + one commit per writer, no merges.
  [ "$(git --git-dir="${ORIGIN}" rev-list --count gh-pages)" -eq 4 ]
  [ -z "$(git --git-dir="${ORIGIN}" rev-list --merges gh-pages)" ]
  [ "$(remote_file install.sh)" = '#!/bin/sh' ]
  [ "$(remote_file releases.json)" = '{}' ]
  [ "$(remote_file index.html)" = '<html>v2</html>' ]
}

@test "a transient rejection is retried without changing the commit" {
  dir="$(checkout writer)"
  seed_tip="$(remote_tip)"
  write_file "${dir}" llms.txt 'v1'
  reject_pushes 1

  run run_publish "${dir}" "publish llms.txt"
  echo "$output"
  [ "$status" -eq 0 ]
  grep -qF "(attempt 2/5)" <<< "$output"
  # Nothing moved upstream, so the rebase replayed nothing: the pushed
  # commit is the one made on top of the seed.
  [ "$(remote_tip)" = "$(git -C "${dir}" rev-parse HEAD)" ]
  [ "$(git --git-dir="${ORIGIN}" rev-parse gh-pages^)" = "${seed_tip}" ]
}

# ---------------------------------------------------------------------------
# Tests: refusals
# ---------------------------------------------------------------------------

@test "a conflicting concurrent edit aborts the rebase and pushes nothing" {
  dir="$(checkout writer)"
  seed_tip="$(remote_tip)"
  concurrent_push other index.html '<html>theirs</html>'
  their_tip="$(remote_tip)"
  write_file "${dir}" index.html '<html>ours</html>'

  run run_publish "${dir}" "edit index.html"
  echo "$output"
  [ "$status" -eq 1 ]
  grep -qF "::error::Rebasing onto the current origin/gh-pages failed" <<< "$output"

  # The remote keeps the other writer's commit, and the checkout is back
  # on its own commit with no rebase in progress.
  [ "$(remote_tip)" = "${their_tip}" ]
  [ "$(git -C "${dir}" log -1 --format=%s)" = "edit index.html" ]
  [ "$(git -C "${dir}" rev-parse HEAD^)" = "${seed_tip}" ]
  [ ! -d "${dir}/.git/rebase-merge" ]
  [ ! -d "${dir}/.git/rebase-apply" ]
}

@test "a rebase that fails without a conflict still fails with an ::error::" {
  # A rebase refused before it starts leaves nothing to abort.
  dir="$(checkout writer)"
  concurrent_push other llms.txt 'theirs'
  their_tip="$(remote_tip)"
  write_file "${dir}" index.html '<html>ours</html>'
  printf '#!/usr/bin/env bash\nexit 1\n' > "${dir}/.git/hooks/pre-rebase"
  chmod +x "${dir}/.git/hooks/pre-rebase"

  run run_publish "${dir}" "edit index.html"
  echo "$output"
  [ "$status" -eq 1 ]
  grep -qF "::error::Rebasing onto the current origin/gh-pages failed" <<< "$output"
  [ "$(remote_tip)" = "${their_tip}" ]
  [ ! -d "${dir}/.git/rebase-merge" ]
  [ ! -d "${dir}/.git/rebase-apply" ]
}

@test "a push the remote keeps rejecting gives up after 5 attempts" {
  dir="$(checkout writer)"
  write_file "${dir}" llms.txt 'v1'
  seed_tip="$(remote_tip)"
  reject_pushes all

  run run_publish "${dir}" "publish llms.txt"
  echo "$output"
  [ "$status" -eq 1 ]
  grep -qF "::error::Push to gh-pages still rejected after 5 attempts" <<< "$output"
  [ "$(grep -c "push rejected (attempt" <<< "$output")" -eq 4 ]
  [ "$(cat "${TEST_TMP}/rejections")" -eq 5 ]
  [ "$(remote_tip)" = "${seed_tip}" ]
}

@test "a missing CHECKOUT_DIR or MESSAGE argument is refused" {
  dir="$(checkout writer)"

  run bash "${SCRIPT}"
  echo "$output"
  [ "$status" -eq 1 ]
  grep -qF "::error::Usage: publish-gh-pages.sh CHECKOUT_DIR MESSAGE" <<< "$output"

  run bash "${SCRIPT}" "${dir}"
  echo "$output"
  [ "$status" -eq 1 ]
  grep -qF "::error::Usage: publish-gh-pages.sh CHECKOUT_DIR MESSAGE" <<< "$output"
}

@test "a directory that does not exist is refused" {
  run run_publish "${TEST_TMP}/missing" "publish"
  echo "$output"
  [ "$status" -eq 1 ]
  grep -qF "::error::" <<< "$output"
  grep -qF "does not exist" <<< "$output"
}

@test "a directory that is not a gh-pages checkout is refused" {
  mkdir "${TEST_TMP}/plain"

  run run_publish "${TEST_TMP}/plain" "publish"
  echo "$output"
  [ "$status" -eq 1 ]
  grep -qF "::error::" <<< "$output"
  grep -qF "is not a gh-pages checkout" <<< "$output"
}

@test "a non-numeric back-off is refused" {
  dir="$(checkout writer)"

  run run_publish "${dir}" "publish" PUBLISH_GH_PAGES_BACKOFF_SECONDS=soon
  echo "$output"
  [ "$status" -eq 1 ]
  grep -qF "::error::PUBLISH_GH_PAGES_BACKOFF_SECONDS 'soon'" <<< "$output"
}
