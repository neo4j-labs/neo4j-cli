# bench

Agent benchmark for `neo4j-cli`. Each task gives a headless Claude Code session a prompt and checks the outcome with code. The protocol is frozen in `protocol.json`; change it and the old results stop being comparable.

```
python3 -I bench/run.py <task> <run>     # T1..T7, writes bench/results/<task>-<run>/
```

Each run writes `stream.jsonl` (full transcript) and `result.json` (success, tokens, turns, commands, cost).

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

T8 to T10 (Aura read, pause, preview-then-delete) need an Aura key and are not written yet. T7 needs Docker running and `docker pull neo4j:community` done first.

## Baseline

`neo4j-cli` v1.19.0, model `claude-sonnet-5-5`, Claude Code 2.1.311, 3 runs per task, 2026-11-16. Three runs are a smoke baseline.

| Task | Success | Median cost (USD) | Turns |
| --- | --- | --- | --- |
| T1 | 3/3 | 0.013 | 4 |
| T2 | 3/3 | 0.014 | 5 |
| T3 | 0/3 | 0.015 | 3 |
| T4 | 3/3 | 0.020 | 5 |
| T5 | 3/3 | 0.012 | 3 |
| T6 | 3/3 | 0.015 | 4 to 6 |
| T7 | 3/3 | 0.042 | 8 to 9 |

T3 fails because the error names the rejected value but not the valid ones. Every run gave up or guessed.
