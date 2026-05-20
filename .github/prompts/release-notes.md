You are generating richer release notes for a tagged `neo4j-cli` release. The workflow that invokes you has already exported these env vars:

- `RELEASE_TAG` — the published tag, e.g. `v1.4.0` (always has a leading `v`).
- `RELEASE_VERSION` — the bare version, e.g. `1.4.0` (no leading `v`).
- `PREV_TAG` — the previous release tag, e.g. `v1.3.0`. May be empty if there is no prior tag.

You have `Bash`, `Read`, and `Write` tools available. Stay inside the repo working directory.

## Your task

Produce a markdown document that gives a human reader a quick, enjoyable summary of what shipped in `${RELEASE_TAG}`, grounded entirely in the canonical changelog and the git log range. Write the final markdown to `/tmp/release-notes-generated.md`. Do NOT print to stdout — the workflow logs the file separately.

## Step 1 — Read the canonical changelog

Read `.changes/neo4j-cli/v${RELEASE_VERSION}.md`. This is the canonical, hand-authored per-release changelog produced by `changie`. It is your source of truth for what changed in this release.

CRITICAL: do NOT read `CHANGELOG.md` (the aggregate file). Do NOT read other `.changes/neo4j-cli/v*.md` files for unrelated versions. The per-version file at the path above is the only changelog input you may use.

If the file does not exist, exit immediately by writing a one-line note ("Changelog `.changes/neo4j-cli/v${RELEASE_VERSION}.md` is missing — release notes cannot be generated.") to `/tmp/release-notes-generated.md` and stop. Do NOT invent a changelog from the git log.

## Step 2 — Inspect the git log range

Run:

```bash
if [ -n "${PREV_TAG}" ]; then
  git log "${PREV_TAG}..${RELEASE_TAG}" --oneline
else
  git log --oneline -30
fi
```

This gives you the candidate commits to review. Use this list to identify the SHAs you'll want to dig into in the next step.

## Step 3 — Dig into the biggest user-facing commits

Pick the commits in the range that look user-facing — new command trees, new flags, breaking changes, deprecations, behaviour changes a CLI user would notice. Skip pure refactors, CI-only changes, dependency bumps, and internal test work unless they are unusually large or relevant.

For each candidate, inspect the diff:

```bash
git show <sha> -- README.md AGENTS.md neo4j-cli/app/app.go 'neo4j-cli/aura/internal/subcommands/**' 'common/skill/**'
```

Use `git show <sha>` (no path filter) only if the path-filtered form returns nothing and you still suspect the commit is user-facing.

## Step 4 — Ground all command/flag syntax in the live cobra tree

When you write an `Example:` block in a highlight, the command and flags MUST exist in the cobra tree at `${RELEASE_TAG}`. Cross-check by reading:

- `neo4j-cli/app/app.go` — the root command tree.
- `neo4j-cli/aura/internal/subcommands/**` — per-resource command trees.
- `neo4j-cli/internal/subcommands/**` — top-level command trees (e.g. `query`, `credential`, `docker`, `skill`).
- `README.md` — canonical usage examples.

Do NOT invent flags. Do NOT cite a flag you cannot find in the source. If you are unsure whether a flag exists, omit the example.

For write operations (anything that mutates Aura state, e.g. `instance create`, `credential aura-client add`), include `--rw` in the example — write commands require it.

For read commands that support multiple output formats, prefer `--format json` to make the example feel agent-friendly.

## Step 5 — Compose the output

Write a single markdown document to `/tmp/release-notes-generated.md` with this structure:

```
### Highlights

- **<single-topic headline>** — <why it matters in one sentence>. Example:
  ```
  neo4j-cli <invocation>
  ```

- **<themed bundle headline>** — <one-sentence theme>:
  - <entry one>
  - <entry two>

### Other changes

- <one-line summary of a smaller change>
- <one-line summary of a smaller change>
```

Constraints — these are firm:

- NO top-level `# h1`. The workflow concatenates a `## Release notes` header upstream of your output. Your first heading must be `### Highlights`.
- Keep the whole document tight — aim short, hard cap 150 lines.
- **Maximum 3 highlights.** Pick only genuinely important or exciting changes: new commands, breaking changes, notable new capability. Skip minor tweaks. If only one thing is exciting, ship one highlight. If nothing is, say so in a single sentence under `### Highlights` and skip `### Other changes`.
- Default highlight shape is single-topic with a runnable example. Use that whenever the change stands on its own.
- Optionally, a highlight may bundle several related entries under one themed headline (e.g. "Security hardening", "Better error handling", "Docker UX polish") when grouping genuinely saves space — list the entries as sub-bullets and skip the example block. Only use the bundle form when the entries share a real theme; otherwise stick with separate single-topic highlights.
- Single-topic highlights must include one runnable `neo4j-cli` invocation in a fenced code block. Use the exact `neo4j-cli` binary name. One sentence of "why it matters" — no recap of the implementation.
- Group every other changelog entry under `### Other changes` as a single bullet per entry. One line each. No example blocks. Terse.
- Tone: a little witty, a little wholesome, dry humour, no corporate fluff, no marketing language, no superlatives. Maintainer quietly pleased the thing shipped. Avoid emoji unless the source changelog entry has one — never add one that isn't in the source.
- Do not mention the SHA, PR number, or Linear ticket ID unless the changelog entry itself includes it.
- Do not include a section about how to install or update — the workflow's Slack preamble already says that.

## Step 6 — Write the file and stop

Write the final markdown to `/tmp/release-notes-generated.md` using the `Write` tool. Do NOT print the body to stdout. Do NOT print a summary at the end of your run — the workflow's `cat` step will surface the file in the job log.
