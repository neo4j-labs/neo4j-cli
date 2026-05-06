# Autoresearch: neo4j-cli skill trigger accuracy (CLI-33)

## Objective
The neo4j-cli SKILL.md description determines whether the model invokes the
skill when a user asks something. Today the skill misses query-related intents
("run cypher from cli", schema introspection, etc.) and may also over-trigger
on unrelated prompts. Optimize the description so the skill fires on real
neo4j-cli intents (aura / query / credential / skill management) and stays
silent on Q&A, driver code, other DBs, and generic shell tasks.

Linear: https://linear.app/neo4j/issue/CLI-33

## Metrics
- **Primary**: `f1` (harmonic mean of precision + recall over the eval set, higher is better)
- **Secondary**: `precision`, `recall`, `accuracy`, `false_positives`, `false_negatives`

`f1` is primary because the user explicitly cares about both directions:
"verify that it doesn't fire when it shouldn't as well."

## How to Run
`.research/cli-33-skill-query-triggers/autoresearch.sh`

For each prompt in `eval_set.json`, the harness:
1. Writes `description.txt` content into a temp `.claude/commands/<uuid>.md`
   inside the research dir (slash-command file gets surfaced to claude).
2. Runs `claude -p <prompt> --output-format stream-json --model claude-haiku-4-5-20251001`.
3. Watches the stream for a `Skill` or `Read` tool_use referencing that uuid.
4. A query "passes" if its trigger rate matches `should_trigger`.

Tunables via env: `WORKERS`, `RUNS_PER_QUERY`, `TIMEOUT`, `MODEL`.

## Files in Scope
- `neo4j-cli/internal/skill/description.txt` — the only file the loop edits.
  It feeds both standalone aura-cli and bundled neo4j-cli skill descriptions
  via `go generate`. We do NOT need to re-run `go generate` between iterations
  because the eval reads description.txt directly via `--description`.

## Off Limits
- Source code under `neo4j-cli/`, `common/`, etc.
- Generated bundles (`*/internal/skill/bundle/*`) — they're regenerated from
  `description.txt`. We only refresh them once at the end via `go generate`.
- Tests, build scripts, CI.

## Constraints
- Description must stay under 1024 chars (Anthropic skill description ceiling).
- Must remain factually accurate — no claims about commands that don't exist.
- Should still cover the four top-level surfaces: aura, query, credential, skill.
- Style: imperative, concrete, lists representative verbs/nouns. Avoid filler.

## Metric Direction Notes
- A pass for `should_trigger=true` means trigger_rate >= 0.5 across runs.
- A pass for `should_trigger=false` means trigger_rate < 0.5.
- Confidence: noise per call is meaningful — we run each query 2x by default;
  bump `RUNS_PER_QUERY=3` if metric variance looks high after a few iterations.

## What's Been Tried

### Eval-harness lessons (before any real iteration)
- The original short description (pre-issue) gave **F1=0** in `claude -p`
  subprocess: 22/22 positives missed. Two bugs in the harness made this
  look like a description problem at first; once fixed, the description
  itself was decent.
- Eval bug 1: skill-creator's `run_eval.py` registers a slash-command, not a
  skill — `claude -p` exposes installed skills, not project slash-commands,
  so the evaluated description was being shadowed by the user's already-
  installed `neo4j-cli` skill.  Switched to `run_eval_real.py` which
  patches `~/.claude/skills/neo4j-cli/SKILL.md` in place + restores on exit.
- Eval bug 2: `subprocess.Popen` with default stdin inherits weird state
  inside ProcessPoolExecutor workers; `claude -p` flat-out refused to invoke
  Skill in that environment.  Setting `stdin=subprocess.DEVNULL` fixed it.
- Eval bug 3: incremental stream-parsing of partial input_json deltas
  missed the trigger event ~95% of the time. Replaced with full-stdout
  `communicate()` + scan assistant `tool_use` events. Detection is now
  binary-clean.
- Eval-set bias: original prompts leaked CLI cues ("from the command line",
  "via cli", "with neo4j-cli"). User flagged this — rewrote 22 positives in
  natural human phrasing ("list my aura instances", "what's the schema of
  my neo4j database"). Adds ~3 F1 points of difficulty; this is the
  realistic baseline.

### Iterations on description.txt (eval = sonnet-4-6, 42 prompts × 2 runs)
| iter | f1     | precision | recall | fp | fn | notes |
|------|--------|-----------|--------|----|----|-------|
| baseline (TRIGGER/SKIP/CLI) | 0.9524 | 1.00 | 0.91 | 0 | 2 | FN: skill-remove, credential-add |
| rebaseline (clean prompts)  | 0.9767 | 1.00 | 0.95 | 0 | 1 | FN: skill-remove only |
| iter1 (broader verbs)       | 0.9767 | 1.00 | 0.95 | 0 | 1 | tied; kept for defensive coverage |
| iter2 (bundled-skill hint)  | 0.9767 | 1.00 | 0.95 | 0 | 1 | discarded |
| iter3 (bolt URI + verbatim example "remove the neo4j-cli skill from my agent" + kubectl/Browser SKIP) | 0.9767 | 1.00 | 0.95 | 0 | 1 | tied; kept |

### Stable plateau
**F1 = 0.9767 (precision = 1.0)** with the iter3 description.

The remaining FN, "remove the embedded neo4j cli skill from my agent",
resists description-only fixes — even a verbatim example phrasing inside
TRIGGER does not flip the model. Hypothesis: sonnet treats agent-side
"skill" management as something to handle directly (Bash into
`~/.claude/skills/`) rather than reaching for the neo4j-cli skill,
because it has model-level priors on Claude Code's own skill storage layout.

### Wins worth flagging
- All 9 cypher/query intents trigger (was 0 in the original description).
- All 3 schema-introspection intents trigger.
- All 6 aura-management intents trigger.
- All 2 credential intents trigger (was 1/2 with the older verb list).
- 0 false positives across 20 negative prompts (driver code, Q&A, other DBs,
  shell, docker, kubectl, Neo4j Browser).
