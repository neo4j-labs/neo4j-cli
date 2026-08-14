# PRD: Remove the phantom `credential dbms get` from shipped docs (CLI-230)

## Overview

`neo4j-cli credential dbms get <name>` is documented as the way to recover a stored dbms
credential's password, but **the command has never existed**. The `credential dbms` tree
registers only `add`, `list`, `remove`, `use`, `set-embed`
(`neo4j-cli/internal/subcommands/credential/dbms/dbms.go:19-23`), and no `get` leaf exists
anywhere under `neo4j-cli/internal/subcommands/credential/` or
`neo4j-cli/aura/internal/subcommands/credential/`.

`list` cannot substitute — it deliberately omits the password
(`dbms/list.go:12` field list; `common/clicfg/credentials/dbms.go:136` "Password is
omitted"). A `DbmsCredentials.Get(name)` **Go method** does exist
(`credentials/dbms.go:108`) and is called internally by `GetDefault` and keyring
migration, which is likely how the phantom CLI command got written into the docs.

This PRD corrects the false promise in shipped help text, README, and the skill-bundle
input, then regenerates the bundle. **It does not add the command.**

### Decision: Option B, from the Linear thread

CLI-230's own description recommended Option A (add the leaf). The comments overrode it:

- **Liam Doodson**: omitting credential-read commands "was a conscious decision" in
  `aura-cli`, and "security normally don't like us to print credentials in responses as it
  increases the risk they get exposed". He proposed `admin user set-password` as the real
  recovery route and said he'd "be more tempted to updating those docs instead of adding
  the commands".
- **Oskar Hane**: "I think I agree with that, so let's take the decision 👍", then
  explicitly: **"Accept with option B as the way to go here."**

Adding a `get` leaf would also cut against CLI-228, which was tightening secret handling
in exactly this area.

### Corrections to the ticket

- **It is 5 files, not 4.** The ticket's reference list misses
  `neo4j-cli/internal/subcommands/docker/create_test.go:1637,1648`, where the doc comment
  on `TestCreate_NoPrintPassword_StoresCredentialForRecovery` calls `credential dbms get`
  "the documented recovery contract". Prose only — the assertions are correct.
- **Line numbers drift.** The ticket cites `README.md:271`; the reference is at
  **`README.md:322`** on `main`. Grep, don't trust the line numbers.
- **"Recovery path is dead" overstates it.** Only *reading the plaintext* is impossible.
  The stored credential remains fully usable — `--credential <name>` resolves it for
  `query` and `admin` (`admin/admin.go:69`) without the operator ever knowing the
  password. This is what makes Option B defensible rather than a bare retraction.

## Goals

- No shipped text (help output, README, skill bundle) references a command that does not exist.
- The `--no-print-password` docs describe what actually works, including the
  stale-credential trap after a password reset.
- The skill bundle stays in sync so `TestGenerator_RoundTrip` is green.

## Non-Goals

- Adding `credential dbms get`, or any other credential-read leaf. Rejected by decision.
- Adding `credential dbms set-password` / an update leaf to close the resync gap (see
  Open Questions).
- Changing `docker create` behaviour, the `create.go:210` flag-combination guard, or
  anything about how credentials are stored.
- A general help-text-accuracy test gate (considered, deferred — see Open Questions).

## Requirements

### The replacement content

Three facts, all verified against the code, to be woven into each site at the length that
site allows:

1. The stored credential still connects via `--credential <name>` — no plaintext needed.
2. The password itself is **not readable through the CLI**, by design.
3. To get a *known* password: `admin user set-password neo4j --new-password <s>
   --credential <name> --rw`, **then resync the stored credential** with `credential dbms
   remove <name>` + `credential dbms add`, because the reset leaves the stored credential
   holding the old password.

Fact 3's resync clause is required, not optional. `admin user set-password`
(`admin/user/set_password.go:38`) issues `ALTER USER ... SET PASSWORD` against the
database and never touches the credentials store, so following the reset advice alone
leaves `--credential <name>` broken. `docker create` stores username `neo4j`, database
`neo4j`, URI `neo4j://localhost:<bolt-port>` (`create.go:351,362`), so a re-add is
`credential dbms add --name <name> --uri neo4j://localhost:<port> --username neo4j
--password <s> --rw`.

### Functional Requirements

- REQ-F-001: In `neo4j-cli/internal/subcommands/docker/create.go:145-146`, replace the
  final `Long` sentence ("Pass --no-print-password to omit the generated password from
  stdout output; retrieve it later via `neo4j-cli credential dbms get <name>`.") with
  prose carrying all three facts including the resync. This is the longest-form site.
  Preserve the existing string-concatenation style and the trailing `,` on the `Long`
  field.

- REQ-F-002: In `create.go:436`, replace the `--no-print-password` flag usage string's
  "Retrieve later via `neo4j-cli credential dbms get <name>`." with a **one-clause**
  version — flag usage renders in `--help` and must stay short. State that the stored
  credential still connects via `--credential <name>` and the password is not
  CLI-readable. Do not attempt to fit the reset+resync procedure here; `Long` carries it.

- REQ-F-003: In `README.md:322`, rewrite the parenthetical "(recover later with
  `neo4j-cli credential dbms get <name>`)" in the "Heads up" paragraph. README has room
  for the full three facts; a short fenced `sh` block showing the reset + resync sequence
  is acceptable and preferred over a long parenthetical.

- REQ-F-004: In `neo4j-cli/internal/skill/additions.md:31`, replace "(recover via
  `neo4j-cli credential dbms get <name>`)" with the three facts. Keep the existing
  single-bullet shape — this file is skill-bundle input, and per AGENTS.md multi-line
  content here must not be indented.

- REQ-F-005: In `neo4j-cli/internal/subcommands/docker/create_test.go`, reword the
  `TestCreate_NoPrintPassword_StoresCredentialForRecovery` doc comment (line ~1637) and
  the `require.NoError` failure message (line ~1648) so neither claims `credential dbms
  get` is the recovery contract. The real contract: the credential is persisted so
  `query`/`admin --credential <name>` connect. **Assertions and test logic are correct —
  do not change them.** The test name may stay as-is; "ForRecovery" is still accurate.

- REQ-F-006: Run `go generate ./neo4j-cli/internal/skill/...` and commit the regenerated
  bundle in the same commit as the sources. `bundle/references/docker.md:29` embeds
  `docker create`'s `Long` and `:47` embeds the flag table; `bundle/SKILL.md:69` embeds
  `additions.md`. Never hand-edit bundle files — generate overwrites them. Unset any
  `NEO4J_CLI_FLAG_*` env vars first (AGENTS.md) or a flag-gated subtree leaks into the
  bundle.

- REQ-F-007: Add a Patch changie entry: `changie new --projects neo4j-cli --kind Patch
  --body "<body>"`. The body states only the observable effect — `docker create
  --no-print-password` help no longer points at a nonexistent `credential dbms get`
  command, and now explains how to use the stored credential and reset the password. It
  must not describe internal mechanics.

### Non-Functional Requirements

- REQ-NF-001: Zero production behaviour change. Only cobra `Long` / flag-usage strings,
  markdown, and test prose change. No `RunE`, no flag registration, no stored-credential
  shape.

- REQ-NF-002: `make test`, `make fmt-check`, `make lint` must all pass (AGENTS.md final gates).

- REQ-NF-003: `policy.golden` must **not** change. No command is added, removed, or
  renamed, so the MCP policy is untouched. A diff there means something went wrong.

- REQ-NF-004: Replacement text must not trip the tee redactor. Per AGENTS.md, do not begin
  a line with a `<secretword>:` prefix — `RedactText`'s assignment regex treats
  `password: <word>` as a secret assignment and scrubs the next word to `***`. Word the
  prose so no secret word (`password`, `token`, `secret`, `key`, `auth`) immediately
  precedes a `:` or `=`.

## Technical Considerations

**Why the flag-usage site gets the short version.** `create.go:436` is rendered inline in
`docker create --help`'s flag table and mirrored into `bundle/references/docker.md:47`'s
markdown table. A multi-sentence procedure there would wrap badly in the terminal and
bloat the bundle table cell. `Long` is the correct home for the procedure; the flag string
just needs to stop lying.

**The `create.go:210` guard stays.** It rejects `--no-print-password` +
`--no-store-credential` without an explicit `--password`, saying the generated password
would be discarded "unrecoverably". Still accurate: with no credential stored there is no
recovery of any kind, whereas stored-but-unreadable is a usable credential. The guard's
rationale is unaffected by this PRD.

**Bundle regeneration is forced, not optional.** `TestGenerator_RoundTrip` runs inside
`make test` and fails on bundle drift. Editing `create.go`'s `Long` or `additions.md`
without regenerating breaks the build.

**Skill-bundle `description.txt` is not affected.** It describes credential subtrees
generally and does not mention `get`; no frontmatter edit needed.

**Grep, don't trust line numbers.** Every line number in this PRD was verified on `main`
at authoring time but `create.go` is ~440 lines and shifts easily. Locate each site by
grepping `credential dbms get`.

## Acceptance Criteria

- [ ] `grep -rn 'credential dbms get' . | grep -v '^./.git/'` returns **zero** results.
- [ ] `docker create --help` shows a `--no-print-password` description that names
      `--credential <name>` and does not name `credential dbms get`.
- [ ] `docker create`'s `Long` explains: credential still connects, password not
      CLI-readable, and reset-then-resync.
- [ ] `README.md`'s "Heads up" paragraph and `additions.md`'s bullet both carry the three
      facts, including the resync step.
- [ ] `create_test.go`'s doc comment and `require.NoError` message no longer cite
      `credential dbms get`; assertions unchanged.
- [ ] `go generate ./neo4j-cli/internal/skill/... && git diff --exit-code` is clean on a
      committed tree (bundle in sync).
- [ ] `git diff` shows **no** change to
      `neo4j-cli/internal/subcommands/mcp/server/testdata/policy.golden`.
- [ ] `git diff` shows **no** change to `create.go`'s `--no-print-password` /
      `--no-store-credential` guard (~line 210).
- [ ] One new file under `.changes/unreleased/` with `kind: Patch`.
- [ ] `go test ./neo4j-cli/internal/subcommands/docker/... -run NoPrintPassword` passes.
- [ ] `make test`, `make fmt-check`, `make lint` all pass.

## Out of Scope

- Adding `credential dbms get` or any credential-read leaf — rejected by decision.
- Adding `credential dbms set-password` / update leaf to close the resync gap.
- `create.go:210` flag-combination guard — accurate as written.
- Any change to `credentials.DbmsCredentials.Get` (`credentials/dbms.go:108`) — a legitimate
  internal Go API with real callers; only the *CLI* command was fictional.
- `CHANGELOG.md`, `.changes/neo4j-cli/v*.md`, `.plans/archive/**` — historical records.
- A help-text-accuracy test gate.
- The other ~40 legitimate `credential dbms <add|list|remove|use|set-embed>` references
  across the repo — those commands all exist.

## Open Questions

- **Resync gap, documented not fixed.** There is no way to update a stored dbms
  credential's password in place; `remove` + `add` is the only route. This PRD documents
  that rather than closing it. A `credential dbms set-password <name> --new-password <s>`
  leaf (writing only to the local store, never to the database) would make the reset
  procedure one step instead of three, and does not print any secret — so it does not
  conflict with the Option B reasoning. Worth a follow-up ticket.
- **Class-of-bug gate deferred.** This phantom command survived in four hand-written
  places plus the generated bundle because nothing verifies that backtick-quoted
  `neo4j-cli ...` strings in help text resolve to real cobra commands. Decision was to use
  a grep acceptance criterion here (matching CLI-229's approach) rather than build the
  gate. If a second phantom command turns up, build the test.
- **Linear hygiene:** CLI-230's description recommends Option A, which the comments
  overrode — worth editing so a future reader doesn't implement the wrong option. Its
  reference list should also be corrected to 5 files and `README.md:322`.
