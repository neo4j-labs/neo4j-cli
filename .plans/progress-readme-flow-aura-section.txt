## task-001 — 2026-05-08T11:15:00Z
Verified live --help output for credential aura-client add, aura instance list/create, aura tenant list. All flag names in PRD draft match shipped CLI; no discrepancies. Read-only.
Files: ~.plans/tasks-readme-flow-aura-section.yml

## task-002 — 2026-05-08T11:30:00Z
Deleted ## Usage block; inserted ## Aura quickstart between Credentials and Querying Neo4j per PRD verbatim. make test/fmt-check/lint all pass.
Files: ~README.md, ~.plans/tasks-readme-flow-aura-section.yml

## task-003 — 2026-05-08T11:40:00Z
Final gates: make test/fmt-check/lint pass. Diff scoped to README.md (committed in task-002) + plan files; no skill-bundle drift, no new .changes/unreleased entries; #credentials anchor resolves.
Files: ~.plans/tasks-readme-flow-aura-section.yml, ~.plans/progress-readme-flow-aura-section.txt
