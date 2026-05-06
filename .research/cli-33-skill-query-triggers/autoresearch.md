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
(updated as the loop progresses)

- Baseline: current description.txt (committed at session start). Mentions
  provisioning aura instances, listing tenants, creating credentials,
  deployments, installing skills — but does NOT mention `query` / cypher /
  schema introspection at all. Hypothesis: query-intent recall will be low.
