#!/bin/bash
# Skill-trigger benchmark for the neo4j-cli skill (real installed skill).
# Backs up ~/.claude/skills/neo4j-cli/SKILL.md, splices the candidate
# description from neo4j-cli/internal/skill/description.txt into the
# frontmatter, runs claude -p for every prompt in eval_set.json,
# detects whether the model invoked Skill(skill="neo4j-cli"), then
# restores the SKILL.md.  Outputs METRIC lines for autoresearch.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$DIR/../.." && pwd)"
DESC_FILE="$REPO/neo4j-cli/internal/skill/description.txt"
EVAL_SET="$DIR/eval_set.json"
RESULT_FILE="$DIR/last_run.json"

WORKERS="${WORKERS:-8}"
RUNS_PER_QUERY="${RUNS_PER_QUERY:-2}"
TIMEOUT="${TIMEOUT:-90}"
MODEL="${MODEL:-claude-sonnet-4-6}"

cd "$DIR"
rm -f "$RESULT_FILE" "$RESULT_FILE.tmp"
PYTHONPATH="$DIR" python3 -m scripts.run_eval_real \
  --eval-set "$EVAL_SET" \
  --description-file "$DESC_FILE" \
  --num-workers "$WORKERS" \
  --runs-per-query "$RUNS_PER_QUERY" \
  --timeout "$TIMEOUT" \
  --trigger-threshold 0.5 \
  --model "$MODEL" \
  --output "$RESULT_FILE"

python3 - "$RESULT_FILE" "$EVAL_SET" <<'PY'
import json, sys, collections
data = json.loads(open(sys.argv[1]).read())
tagged = {item["query"]: item.get("tag", "?") for item in json.loads(open(sys.argv[2]).read())}
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
by_tag = collections.defaultdict(lambda: {"pass": 0, "total": 0})
for r in results:
    tag = tagged.get(r["query"], "?")
    by_tag[tag]["total"] += 1
    if r["pass"]:
        by_tag[tag]["pass"] += 1
print("# per-tag:", " ".join(f"{t}={v['pass']}/{v['total']}" for t, v in sorted(by_tag.items())))
print("# failures:")
for r in results:
    if not r["pass"]:
        marker = "FN" if r["should_trigger"] else "FP"
        print(f"#   [{marker}] rate={r['triggers']}/{r['runs']} :: {r['query']}")
PY
