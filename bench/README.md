# bench

Agent benchmark for `neo4j-cli`. Each task gives a headless Claude Code session a prompt and checks the outcome with code. The protocol is frozen in `protocol.json`; change it and the old results stop being comparable.

```
python3 -I bench/run.py <task> <run>                          # T1..T10, writes bench/results/<task>-<run>/
python3 -I bench/run.py <task> <run> --bin PATH --arm LABEL   # same task on another build, writes bench/results/<arm>/<task>-<run>/
python3 -I bench/run.py <task> <run> --model NAME             # same task on another model, writes bench/results/<model-slug>/<task>-<run>/
python3 -I bench/report.py                                    # one row per arm, model and task, p25/median/p75, plus per-(model, arm) totals
python3 -I bench/score.py [--force] [--task T1]               # LLM-as-judge scores per run (see below)
```

Each run writes `stream.jsonl` (full transcript) and `result.json` (success, model, tokens, turns, commands, cost, `tool_output_bytes`, binary version and SHA-256).

`tasks.json` holds every prompt. `expectedAnswer` must appear as a whole word of the final answer, and `mustRun` lists commands the agent must have run. State checks (config, skill files, history, containers) live in `run.py`.

## Comparing two builds

Run the same task set with `--bin` and `--arm` once per build, for example `parent` and `changed`, then `report.py`. Only `--bin` changes between arms. A build wins only on a material drop in `tool_output_bytes` with the success rate held. A cheaper arm that answers wrong loses. Any other change is a non-win and is reported as one. This is the decision rule of relate's `relate-agentbench`.

## Comparing models

`--model NAME` runs the same tasks on another model and files the runs under `bench/results/<model-slug>/`; every `result.json` records its `model`, and `report.py` groups by (arm, model, task) and prints one totals row per model: tasks passed, total cost, cost per run — the cost-vs-correctness matrix (the upper-left corner is cheapest-and-most-correct). The same decision rule prices models: a cheaper model wins on a task only if its success rate holds. The model became a variable dimension in protocol `/4` (`/3` added task T11 on branch `chore/bench-protocol-3`); `/2` and `/3` results carry no model field and report as the pinned one.

## Scorers

`score.py` judges stored runs after the fact: it compacts each transcript (commands with output sizes, agent messages, final answer) and asks the judge model from `scorers.json` for a 0-10 grade per scorer (lower is better), currently `verbosity` and `redundant_work`, each with a pass threshold. The judge cost is bounded by `max_budget_usd`, `sampling_rate` skips runs below the rate (recorded as not judged, never silently dropped), and re-runs skip runs that already have a `score.json` unless `--force`. The deterministic checks in `run.py` stay the source of truth for success; the judges grade only what code cannot check, and their grades are heuristics, not measurements.

## Isolation

- Fresh empty working dir and a throwaway `HOME` for `neo4j-cli`, set by a `PATH` wrapper that execs the pinned binary. The wrapper does not stop the agent reading the real `HOME`, so the agent shell is limited to `neo4j-cli`, `head`, `tail`, `jq` and `wc`, and told so.
- No user settings, skills, MCP servers or hooks reach the session (`--setting-sources project`, `--disable-slash-commands`, `--strict-mcp-config`).
- Fixtures use `credential-storage insecure`. The keyring does not work under a throwaway `HOME`.
- The run records whether the binary SHA-256 matches the pin. Re-read the pins before each batch; Claude Code and plugins auto-update.

## Tasks

| Task | Prompt asks the agent to | Success check |
| --- | --- | --- |
| T1 | find the stored DBMS credential | answer names `bench-db`, and the agent ran `credential dbms list` |
| T2 | set the default format to json | `config get format` is `json` |
| T3 | fix a bad `--format` using only the error | a later `config list` with a valid format, at most 4 shell calls |
| T4 | install the skill for Claude Code | a `SKILL.md` exists under the throwaway `HOME` |
| T5 | list `aura instance` write subcommands | names all of create, delete, deploy, load, overwrite, pause, resume, update, and neither get nor list |
| T6 | show the last 5 history entries as JSON | a JSON array of 5 `neo4j-cli ...` strings |
| T7 | start a throwaway community Neo4j in Docker, run `RETURN 1+41 AS answer`, remove it | answer is 42, the agent ran `docker create` and `query`, no managed container is left |

| T8 | find the project name of the default Aura workspace | answer is `New project`, the agent ran `neo4j-cli aura ...` |
| T9 | create a free Aura instance `bench-t9-<run>`, wait until running, report its ID | the instance exists, is running and its ID is in the answer |
| T10 | delete the Aura instance `bench-t10-<run>` (the setup creates it), report its ID | the instance is gone or destroying and its real ID is in the answer |

T7 needs Docker running and `docker pull neo4j:community` done first. T8 to T10 run against the Aura dev environment frozen in `protocol.json` and need `AURA_CLIENT_ID` and `AURA_CLIENT_SECRET` in the environment or in `~/.config/aura/env`. The runner deletes every `bench-*` instance before and after a run, because the workspace holds one free instance at a time, so run these tasks one at a time. Free instances cannot be paused, so there is no pause task. Run the harness outside the Claude Code sandbox: inside it the agent's shell drops the PATH wrapper and the CLI runs with the wrong HOME.

## Baseline

`neo4j-cli` v1.19.0, model `claude-sonnet-5-5`, Claude Code 2.1.311, protocol `swf-bench-protocol/2`, 3 runs per task, 2026-11-17. Three runs are a smoke baseline. Medians; `python3 -I bench/report.py` prints p25 and p75 too.

| Task | Success | Turns | Tool output (bytes) | Cost (USD) |
| --- | --- | --- | --- | --- |
| T1 | 3/3 | 4 | 2940 | 0.0131 |
| T2 | 3/3 | 5 | 3366 | 0.0148 |
| T3 | 1/3 | 3 | 951 | 0.0158 |
| T4 | 3/3 | 5 | 6956 | 0.0220 |
| T5 | 3/3 | 3 | 3027 | 0.0115 |
| T6 | 3/3 | 5 | 4049 | 0.0151 |
| T7 | 3/3 | 9 | 20176 | 0.0421 |
| T8 | 3/3 | 5 | 5626 | 0.0163 |
| T9 | 3/3 | 5 | 8630 | 0.0209 |
| T10 | 3/3 | 5 | 2593 | 0.0157 |

T3 fails 2 of 3 because the error names the rejected value but not the valid ones. The run that passed guessed a valid value after reading help. The protocol `/1` baseline (T1 to T6 on 2026-11-16) had T3 at 0 of 3.
