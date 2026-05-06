#!/bin/bash
# Skill-trigger eval — pure bash + jq, no python.
#
# Renders <workspace>/.claude/skills/neo4j-cli/SKILL.md from `skill_print` with
# the candidate description spliced into the description: line, then runs
# `claude -p` per prompt with cwd at the workspace and a per-call timeout.
# Triggered := first tool_use in the assistant stream is Skill(neo4j-cli) or
# Read(skills/neo4j-cli/SKILL.md).  All else is "did not trigger".
set -euo pipefail
set +m  # suppress "Terminated" notifications when guard processes kill claude

EVAL_SET=""
DESC_FILE=""
SKILL_PRINT=""
WORKSPACE=""
OUTPUT=""
WORKERS="${WORKERS:-4}"
RUNS_PER_QUERY="${RUNS_PER_QUERY:-2}"
TIMEOUT="${TIMEOUT:-120}"
MODEL="${MODEL:-claude-haiku-4-5}"
SKILL_NAME="neo4j-cli"

while [ $# -gt 0 ]; do
  case "$1" in
    --eval-set) EVAL_SET="$2"; shift 2;;
    --description-file) DESC_FILE="$2"; shift 2;;
    --skill-print) SKILL_PRINT="$2"; shift 2;;
    --workspace) WORKSPACE="$2"; shift 2;;
    --output) OUTPUT="$2"; shift 2;;
    --num-workers) WORKERS="$2"; shift 2;;
    --runs-per-query) RUNS_PER_QUERY="$2"; shift 2;;
    --timeout) TIMEOUT="$2"; shift 2;;
    --model) MODEL="$2"; shift 2;;
    *) echo "unknown flag: $1" >&2; exit 2;;
  esac
done
for v in EVAL_SET DESC_FILE SKILL_PRINT WORKSPACE OUTPUT; do
  if [ -z "${!v}" ]; then echo "missing --${v//_/-}" >&2; exit 2; fi
done

# 1. Render the project-scoped SKILL.md.
mkdir -p "$WORKSPACE/.claude/skills/$SKILL_NAME"
SKILL_MD="$WORKSPACE/.claude/skills/$SKILL_NAME/SKILL.md"
DESC=$(tr '\n' ' ' < "$DESC_FILE" | sed 's/  */ /g; s/ *$//')
awk -v desc="$DESC" '
  BEGIN { in_fm=0; fm_seen=0; replaced=0 }
  /^---$/ { fm_seen++; in_fm=(fm_seen==1); print; next }
  in_fm && !replaced && /^description:[[:space:]]*/ { print "description: " desc; replaced=1; next }
  { gsub(/\{\{VERSION\}\}/, "dev"); print }
' "$SKILL_PRINT" > "$SKILL_MD"

# 2. Per-prompt runner.  Times out after $TIMEOUT seconds via a guard process.
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
RESULTS_DIR="$TMP/results"; mkdir -p "$RESULTS_DIR"

run_one() {
  local out_file="$1" query="$2"
  local stream="$out_file.jsonl"
  (
    cd "$WORKSPACE"
    env -u CLAUDECODE claude -p "$query" \
      --output-format stream-json --verbose --include-partial-messages \
      --model "$MODEL" \
      </dev/null >"$stream" 2>/dev/null
  ) &
  local cmd_pid=$!
  ( sleep "$TIMEOUT"; kill -TERM "$cmd_pid" 2>/dev/null; sleep 1; kill -KILL "$cmd_pid" 2>/dev/null ) &
  local guard_pid=$!
  wait "$cmd_pid" 2>/dev/null
  local rc=$?
  kill "$guard_pid" 2>/dev/null || true
  if [ "$rc" -ne 0 ] || [ ! -s "$stream" ]; then
    echo 0 > "$out_file"; return
  fi
  local triggered
  triggered=$(jq -rsc --arg name "$SKILL_NAME" '
    [ .[]
      | select(.type=="assistant")
      | .message.content[]
      | select(.type=="tool_use")
    ] as $tools
    | if ($tools|length)==0 then 0
      else (
        $tools[0] as $t
        | if   $t.name=="Skill" and ($t.input.skill // "")==$name then 1
          elif $t.name=="Read"  and (($t.input.file_path // "") | contains("skills/" + $name + "/SKILL.md")) then 1
          else 0 end
      ) end' "$stream" 2>/dev/null || echo 0)
  echo "${triggered:-0}" > "$out_file"
  rm -f "$stream"
}
export -f run_one
export WORKSPACE MODEL SKILL_NAME TIMEOUT

# 3. NUL-delimited (id\tquery) work list — survives queries with whitespace.
WORKLIST="$TMP/worklist.nul"
jq -rc --argjson runs "$RUNS_PER_QUERY" '
  to_entries[]
  | . as $row
  | range(0;$runs)
  | ($row.key|tostring) + "_" + (.|tostring) + "\t" + $row.value.query
' "$EVAL_SET" | tr '\n' '\0' > "$WORKLIST"

# 4. Fan out via xargs -0 (BSD/macOS supports -0 but not -d).
# 2>/dev/null drops the bash job-control "Terminated" notices that escape
# from guard processes when claude finishes before its timeout fires.
export RESULTS_DIR
< "$WORKLIST" xargs -0 -n1 -P "$WORKERS" -I{} bash -c '
  IFS=$'"'"'\t'"'"' read -r id query <<<"$1"
  run_one "$RESULTS_DIR/$id.txt" "$query"
' _ {} 2>/dev/null

# 5. Aggregate per query.  CSV columns: tag, expected, pass, triggers, runs, idx, query
PER_QUERY="$TMP/per_query.tsv"
n=$(jq 'length' "$EVAL_SET")
i=0
while [ "$i" -lt "$n" ]; do
  query=$(jq -r ".[$i].query" "$EVAL_SET")
  expected=$(jq -r ".[$i].should_trigger" "$EVAL_SET")
  tag=$(jq -r ".[$i].tag // \"?\"" "$EVAL_SET")
  triggers=0
  for r in $(seq 0 $((RUNS_PER_QUERY-1))); do
    f="$RESULTS_DIR/${i}_${r}.txt"
    [ -f "$f" ] && triggers=$((triggers + $(cat "$f")))
  done
  rate_x10=$((triggers * 10 / RUNS_PER_QUERY))
  if [ "$expected" = "true" ]; then
    [ "$rate_x10" -ge 5 ] && pass=1 || pass=0
  else
    [ "$rate_x10" -lt 5 ] && pass=1 || pass=0
  fi
  printf '%s\t%s\t%s\t%d\t%d\t%d\t%s\n' "$tag" "$expected" "$pass" "$triggers" "$RUNS_PER_QUERY" "$i" "$query" >> "$PER_QUERY"
  i=$((i + 1))
done

# 6. Emit results JSON.
jq -Rsc '
  split("\n") | map(select(length>0))
  | map(split("\t") | {tag: .[0], should_trigger: (.[1]=="true"), pass: (.[2]=="1"),
                       triggers: (.[3]|tonumber), runs: (.[4]|tonumber),
                       idx: (.[5]|tonumber), query: .[6]})
  | { results: [ .[] | {query, should_trigger, trigger_rate: (.triggers/.runs),
                        triggers, runs, pass, tag} ],
      summary: { total: length,
                 passed: ([.[]|select(.pass)]|length),
                 failed: ([.[]|select(.pass|not)]|length) } }
' "$PER_QUERY" | jq . > "$OUTPUT.tmp"
mv "$OUTPUT.tmp" "$OUTPUT"

# 7. METRIC + per-tag + failures.
awk -F'\t' '
  BEGIN { TP=0; FN=0; FP=0; TN=0 }
  $2=="true"  && $3=="1" { TP++ }
  $2=="true"  && $3=="0" { FN++ }
  $2=="false" && $3=="1" { TN++ }
  $2=="false" && $3=="0" { FP++ }
  END {
    total = TP + FN + FP + TN
    acc  = (total>0) ? (TP+TN)/total : 0
    prec = (TP+FP>0) ? TP/(TP+FP) : 0
    rec  = (TP+FN>0) ? TP/(TP+FN) : 0
    f1   = (prec+rec>0) ? 2*prec*rec/(prec+rec) : 0
    printf "METRIC f1=%.4f\n", f1
    printf "METRIC precision=%.4f\n", prec
    printf "METRIC recall=%.4f\n", rec
    printf "METRIC accuracy=%.4f\n", acc
    printf "METRIC false_positives=%d\n", FP
    printf "METRIC false_negatives=%d\n", FN
    printf "# TP=%d TN=%d FP=%d FN=%d total=%d\n", TP, TN, FP, FN, total
  }' "$PER_QUERY"

awk -F'\t' '
  { t[$1]++; if ($3=="1") p[$1]++ }
  END {
    for (k in t) printf "%s\t%d\t%d\n", k, (p[k]+0), t[k]
  }' "$PER_QUERY" | sort | awk 'BEGIN{printf "# per-tag:"} {printf " %s=%d/%d",$1,$2,$3} END{print ""}'

echo "# failures:"
awk -F'\t' '$3=="0" {
  marker = ($2=="true") ? "FN" : "FP"
  printf "#   [%s] rate=%d/%d :: %s\n", marker, $4, $5, $7
}' "$PER_QUERY"
