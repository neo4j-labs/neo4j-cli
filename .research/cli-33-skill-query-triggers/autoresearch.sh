#!/bin/bash
# Self-contained skill-trigger benchmark — pure bash, no python.
#
# 1. Build bin/neo4j-cli (if missing or older than description.txt).
# 2. `bin/neo4j-cli skill print` → workspace/.claude/skills/neo4j-cli/SKILL.md
#    (with description.txt content spliced into the description: line).
# 3. Spawn `claude -p` per prompt with cwd=workspace, parallel via xargs -P.
# 4. Score F1 / precision / recall / accuracy / fp / fn.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$DIR/../.." && pwd)"
# Iteration target — a local candidate description, NOT the source-tree
# description.txt. Only port back to neo4j-cli/internal/skill/ once the loop
# has settled on a winner.
DESC_FILE="$DIR/candidate_description.txt"
EVAL_SET="$DIR/eval_set.json"
WORKSPACE="$DIR/workspace"
SKILL_PRINT="$DIR/skill_print.md"
RESULT_FILE="$DIR/last_run.json"
BIN="$REPO/bin/neo4j-cli"

WORKERS="${WORKERS:-8}"
RUNS_PER_QUERY="${RUNS_PER_QUERY:-3}"
TIMEOUT="${TIMEOUT:-90}"
MODEL="${MODEL:-claude-haiku-4-5}"

if [ ! -x "$BIN" ]; then
  (cd "$REPO" && go build -o "$BIN" ./neo4j-cli)
fi

"$BIN" skill print > "$SKILL_PRINT"
mkdir -p "$WORKSPACE/.claude/skills/neo4j-cli"
rm -f "$RESULT_FILE" "$RESULT_FILE.tmp"

bash "$DIR/scripts/run_eval.sh" \
  --eval-set "$EVAL_SET" \
  --description-file "$DESC_FILE" \
  --skill-print "$SKILL_PRINT" \
  --workspace "$WORKSPACE" \
  --output "$RESULT_FILE" \
  --num-workers "$WORKERS" \
  --runs-per-query "$RUNS_PER_QUERY" \
  --timeout "$TIMEOUT" \
  --model "$MODEL"
