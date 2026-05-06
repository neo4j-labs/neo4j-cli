#!/bin/bash
# Skill-trigger benchmark for the neo4j-cli skill.
# Reads the current description.txt, evaluates it against eval_set.json by
# spawning `claude -p` for each prompt and watching the stream for a skill
# invocation, and emits METRIC lines for autoresearch.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$DIR/../.." && pwd)"
DESC_FILE="$REPO/neo4j-cli/internal/skill/description.txt"
SKILL_DIR="$REPO/neo4j-cli/internal/skill/bundle"
EVAL_SET="$DIR/eval_set.json"
RESULT_FILE="$DIR/last_run.json"

# claude -p needs a project root that owns a `.claude/commands/` dir; the
# eval script writes its temp slash-command file there.
mkdir -p "$DIR/.claude/commands"

# Tunables (override via env if needed)
WORKERS="${WORKERS:-8}"
RUNS_PER_QUERY="${RUNS_PER_QUERY:-2}"
TIMEOUT="${TIMEOUT:-90}"
MODEL="${MODEL:-claude-haiku-4-5-20251001}"

DESC="$(cat "$DESC_FILE")"

cd "$DIR"
PYTHONPATH="$DIR" python3 -m scripts.run_eval \
  --eval-set "$EVAL_SET" \
  --skill-path "$SKILL_DIR" \
  --description "$DESC" \
  --num-workers "$WORKERS" \
  --runs-per-query "$RUNS_PER_QUERY" \
  --timeout "$TIMEOUT" \
  --trigger-threshold 0.5 \
  --model "$MODEL" \
  > "$RESULT_FILE"

python3 - "$RESULT_FILE" <<'PY'
import json, sys
data = json.loads(open(sys.argv[1]).read())
results = data["results"]
TP = sum(1 for r in results if r["should_trigger"] and r["pass"])
FN = sum(1 for r in results if r["should_trigger"] and not r["pass"])
FP = sum(1 for r in results if not r["should_trigger"] and not r["pass"])
TN = sum(1 for r in results if not r["should_trigger"] and r["pass"])
total = len(results)
acc = (TP + TN) / total if total else 0.0
prec = TP / (TP + FP) if (TP + FP) else 0.0
rec = TP / (TP + FN) if (TP + FN) else 0.0
f1 = 2 * prec * rec / (prec + rec) if (prec + rec) else 0.0

print(f"METRIC f1={f1:.4f}")
print(f"METRIC precision={prec:.4f}")
print(f"METRIC recall={rec:.4f}")
print(f"METRIC accuracy={acc:.4f}")
print(f"METRIC false_positives={FP}")
print(f"METRIC false_negatives={FN}")
print(f"# TP={TP} TN={TN} FP={FP} FN={FN} total={total}")

# also surface per-tag breakdown for debugging
import collections
by_tag = collections.defaultdict(lambda: {"pass": 0, "total": 0})
# the run_eval output drops the tag, so re-attach by query lookup
with open("eval_set.json") as f:
    tagged = {item["query"]: item.get("tag", "?") for item in json.load(f)}
for r in results:
    tag = tagged.get(r["query"], "?")
    by_tag[tag]["total"] += 1
    if r["pass"]:
        by_tag[tag]["pass"] += 1
print("# per-tag:", " ".join(f"{t}={v['pass']}/{v['total']}" for t, v in sorted(by_tag.items())))

# list failing prompts so the loop can reason about what to fix next
print("# failures:")
for r in results:
    if not r["pass"]:
        marker = "FN" if r["should_trigger"] else "FP"
        print(f"#   [{marker}] rate={r['triggers']}/{r['runs']} :: {r['query']}")
PY
