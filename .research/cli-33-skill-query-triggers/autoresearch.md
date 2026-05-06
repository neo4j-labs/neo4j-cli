# Autoresearch: neo4j-cli skill trigger accuracy (CLI-33)

## Objective
Optimize the neo4j-cli SKILL.md description so the skill fires on real
neo4j-cli intents (cypher, schema, aura, credential, skill management) and
stays silent on Q&A, driver code, other DBs, Docker/Kubernetes, Browser UI.

Linear: https://linear.app/neo4j/issue/CLI-33

## Metrics
- **Primary**: `f1` (harmonic mean of precision + recall)
- **Secondary**: `precision`, `recall`, `accuracy`, `false_positives`, `false_negatives`

`f1` is primary because the user explicitly cares about both directions:
"verify that it doesn't fire when it shouldn't as well."

## How to Run
```
bash .research/cli-33-skill-query-triggers/autoresearch.sh
```

The harness is **pure bash + jq + go** (no python).  Per iteration it:

1. Builds `bin/neo4j-cli` if missing.
2. Captures `bin/neo4j-cli skill print` to `skill_print.md`.
3. Splices the candidate description from `candidate_description.txt` into
   the description: line of `workspace/.claude/skills/neo4j-cli/SKILL.md`.
4. Runs `claude -p <prompt>` per row of `eval_set.json` with `cwd=workspace`,
   3 runs per query, parallel via `xargs -0 -P`.  Per-call timeout 120s.
5. Triggered iff the first assistant `tool_use` is `Skill(neo4j-cli)` or
   `Read(<workspace>/.claude/skills/neo4j-cli/SKILL.md)`.
6. Aggregates F1/precision/recall/accuracy and emits METRIC lines.

The harness is fully self-contained: it does NOT touch `~/.claude/skills/`
or any other global state.  All eval state lives under `.research/`.

## Files in Scope
- `.research/cli-33-skill-query-triggers/candidate_description.txt` — the
  iteration target.  Source-tree `neo4j-cli/internal/skill/description.txt`
  is touched only once at the very end, after the loop converges.

## Off Limits
- `neo4j-cli/internal/skill/{description.txt,bundle/}` during iteration.
- All other source code, tests, build scripts, CI.

## Constraints (per Anthropic skill best-practices)
- Description MUST be ≤ 1024 chars, third-person, no XML.
- Should include both **what** the skill does and **when** to use it.
- Concise: their canonical examples are 1–2 sentences.
- Prefer specific key terms over vague generalities.

## What's Been Tried (segment 1, project-scoped harness)

| iter | desc length | f1     | precision | recall | fp | fn | notes |
|------|-------------|--------|-----------|--------|----|----|-------|
| baseline | ~245 chars | 0.9778 / 0.9545 (over 2 runs) | 0.96–1.0 | 0.95–1.0 | 0–1 | 0–1 | One-sentence + "Use when …".  Single flaky kubectl FP at rate=1/2. |
| **iter1**     | ~440 chars | **0.9767** (stable, 2 runs) | **1.0** | **0.9545** | **0** | **1** | Append SKIP clause (cypher syntax / drivers / Docker-Kubernetes / Browser / other DBs).  Bumped runs-per-query to 3.  Kubectl FP eliminated. |
| iter2     | ~480 chars | 0.9302 | 0.95 | 0.91 | 1 | 2 | DISCARD.  Added "uninstall" verb + verbatim "remove the neo4j-cli skill from my agent" example.  Backfired: kubectl FP became consistent (3/3) and a new schema-uri FN appeared. |

### Final winner (iter1)
```
Runs Cypher and manages Neo4j Aura from the terminal via the neo4j-cli CLI.
Use when the user wants to execute Cypher, introspect a Neo4j schema, manage
Aura instances/tenants/credentials, or install/remove the neo4j-cli skill in
an agent. Skip for Cypher syntax questions, graph data modeling, Neo4j
drivers, Docker/Kubernetes, Neo4j Browser, or other databases.
```

**F1 = 0.9767** (precision = 1.0, recall = 0.9545) on 42 prompts × 3 runs.

### Wins
- All 9 cypher/query intents trigger (was 0/9 with the old short description).
- All 3 schema-introspection intents trigger.
- All 6 Aura-management intents trigger.
- All 2 credential intents trigger.
- 0 false positives across 20 negatives (driver code, Q&A, other DBs,
  shell, docker, kubectl, Neo4j Browser).

### Remaining FN
"remove the embedded neo4j cli skill from my agent" — the model treats
agent-side skill management as something to handle directly (Bash into
`~/.claude/skills/`) rather than reaching for the neo4j-cli skill.
Three different attempts to flip this with the description alone (broader
verbs, verbatim example, anti-pattern hint) either tied F1 or regressed it.
Accepted as the realistic ceiling for description-only triggering.

## Notes
- An earlier segment of this experiment (segment 0) used a different harness
  that patched `~/.claude/skills/neo4j-cli/SKILL.md` in place; the user
  flagged it as risky / conflated with the source-tree description, and the
  results from that segment were discarded.  Segment 1 is the project-scoped
  rewrite.
- macOS bash 3.2 + BSD xargs lack `-d`/`wait -n`; the harness uses
  NUL-delimited records via `xargs -0` and a sleep+kill guard for per-call
  timeouts.
