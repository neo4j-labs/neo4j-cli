# PRD: `:embed` param modifier and `query :embed` leaf

## Overview

Add an `:embed` parameter-name modifier and a standalone `query :embed [text]` cobra leaf to `neo4j-cli`, allowing users to turn natural-language text into Neo4j vector-search inputs without leaving the CLI. Adds a new top-level `EmbedCredentials` collection (mirroring the `DbmsCredentials` shape established by PRs #37 and #35), an optional `embed-credential` link on each `DbmsCredential`, and a `credential embed {add,list,remove,use}` subtree. Three providers ship in v1: **OpenAI**, **Ollama**, **HuggingFace**.

After this change:

```sh
# inline modifier — text → embedding → bound to $q before the query runs
neo4j-cli query --param q:embed='sci-fi movies' --param k=5 \
  "CALL db.index.vector.queryNodes('idx', \$k, \$q) YIELD node, score RETURN node, score"

# standalone leaf — prints the vector via --format json|table|toon
neo4j-cli query :embed "hello world"

# linked-cred path — one --credential pulls both DB + embed config
neo4j-cli credential embed add --name openai-shared --provider openai --model text-embedding-3-small --api-key …
neo4j-cli credential dbms add --name prod --uri … --username … --password … --embed-credential openai-shared
neo4j-cli query --credential prod --param q:embed='…' "…"
```

## Goals

- Replace external-then-paste workflows with first-class embedding support inside `neo4j-cli query`.
- Reuse the existing connection/credential/.env/env-var resolution patterns so embedding config feels consistent with dbms config.
- Allow one `--credential <name>` to drive both DB connection and embedding config (via the optional dbms→embed link).
- Keep the embedding path opt-in: with no embed config and no `:embed` modifier in use, the query path is byte-identical to today.
- Keep `--format` rendering uniform (json / table / toon) for the standalone leaf — no embed-specific output formats.

## Non-Goals

- Embedding cache (e.g. by `(provider, model, sha256(text))`). Deferred to a follow-up PRD.
- `--param NAME:embed=@file.txt` (read text from a file). Not in v1.
- A leaf-local `--dimensions` override on `query :embed`. Only `--embed-dimensions` (persistent on the `query` parent) exists; the leaf inherits it.
- Re-running embedding for the EXPLAIN preflight separately from the real run. The same vector is reused.
- New embed-specific output formats (e.g. Rust repo's `--format raw` newline-per-float). Standard `--format` only.
- New providers beyond OpenAI / Ollama / HuggingFace.
- Cascade-delete or reference counting when removing an embed credential that is linked from one or more dbms credentials.
- Changes to Aura subcommands or Aura credentials.
- Ahead-of-time validation that a stored embed credential's API key is still valid (no probe call at `add` time).

## Requirements

### Functional Requirements

#### Param modifier

- REQ-F-001: `--param NAME:embed=<text>` is accepted by `neo4j-cli query`. The literal text is sent to the configured embedding provider; the resulting `[]float32` is bound to the cypher parameter `$NAME` for both the EXPLAIN preflight and the actual run.
- REQ-F-002: A `--param` entry with no `:` continues to behave exactly as today (JSON-typed if the value parses as JSON, otherwise string). No regressions.
- REQ-F-003: An unknown modifier (anything other than `embed`) returns `clierr.NewUsageError("invalid --param %q: unknown modifier %q", entry, modifier)`.
- REQ-F-004: An empty parameter name (e.g. `:embed=foo`) returns a usage error referencing the offending entry.
- REQ-F-005: Empty text (`q:embed=`) is accepted and forwarded to the provider verbatim — provider errors propagate as-is.
- REQ-F-006: When `:embed` is used but the value parses as a JSON array, the command rejects the entry with `clierr.NewUsageError("--param %q: :embed expects text, got JSON array", entry)`. Other JSON shapes (objects, scalars, null) pass through as text since the modifier disables JSON-typing.
- REQ-F-007: Multiple `--param NAME:embed=<text>` entries in a single invocation are supported. Each results in one provider call; calls happen sequentially in the order the params appear on the command line.
- REQ-F-008: All embedding calls happen **once up front**, before the EXPLAIN preflight (`rejectWriteCypher` in `neo4j-cli/query/run.go`). Failure of any embedding call aborts the command before any Cypher is sent to Neo4j.

#### Standalone `:embed` leaf

- REQ-F-009: `neo4j-cli query :embed [text]` is a new cobra leaf alongside `:schema`. With one positional arg, that arg is the text. With zero args and piped stdin, stdin is the text. With zero args and a TTY stdin, the command returns a usage error.
- REQ-F-010: The leaf does NOT open a Bolt driver and does NOT prompt for a password. `--uri`, `--username`, `--password`, `--database` are accepted (inherited persistent flags) but ignored; `--credential <name>` is read only to pick up an optional `embed-credential` link from the named dbms cred.
- REQ-F-011: Output is the raw vector — `[]float32`. Under `--format json`, it is a JSON array of floats. Under `--format toon`, the toon-encoded equivalent. Under `--format table` or `default`, a single-cell table with `[N floats]` for readability. No provider/model metadata is added to the output.

#### Embed config + flags

- REQ-F-012: The following persistent flags are added to the `query` parent (and inherited by `:schema` and `:embed`):
  - `--embed-credential <name>` (string)
  - `--embed-provider <openai|ollama|huggingface>` (string)
  - `--embed-model <name>` (string)
  - `--embed-base-url <url>` (string)
  - `--embed-dimensions <int>` (int)
  Each of `--embed-provider`, `--embed-model`, `--embed-base-url`, `--embed-dimensions` is documented in `Short`/help text as `[env: NEO4J_EMBED_<NAME>]`.
- REQ-F-013: Resolution order (highest wins):
  1. CLI flags (above)
  2. OS environment: `NEO4J_EMBED_PROVIDER`, `NEO4J_EMBED_MODEL`, `NEO4J_EMBED_BASE_URL`, `NEO4J_EMBED_DIMENSIONS`. API key from per-provider env first, then generic: OpenAI → `OPENAI_API_KEY` → `NEO4J_EMBED_API_KEY`; HuggingFace → `HF_TOKEN` → `NEO4J_EMBED_API_KEY`; Ollama → no API key required.
  3. `.env` walk-up from cwd (same `findDotenv` walker as `query/connect.go:307-320`)
  4. **Base embed credential**, picked by FIRST match:
     - Explicit `--embed-credential <name>`
     - Linked embed cred from the resolved dbms cred — the dbms cred selected by `--credential <name>` or `cfg.Credentials.Dbms.GetDefault()` — if its `EmbedCredential` field is non-empty
     - `cfg.Credentials.Embed.GetDefault()`
     - Empty
  5. Anything still empty → provider's hard-coded default (OpenAI: `https://api.openai.com/v1`; Ollama: `http://localhost:11434`; HuggingFace: `https://router.huggingface.co/hf-inference/models`).
- REQ-F-014: When `--embed-credential <name>` references a non-existent embed credential, the command returns a usage error naming the missing credential and pointing at `neo4j-cli credential embed list`.
- REQ-F-015: When the resolved provider needs an API key (OpenAI, HuggingFace) and none is found across env / .env / stored cred, the command returns `clierr.NewUsageError("missing API key for <provider>: set <ENV>")` (e.g. `OPENAI_API_KEY` or `HF_TOKEN`).

#### Provider HTTP behaviour

- REQ-F-016: **OpenAI** — `POST {base_url}/embeddings`, bearer auth, body `{"model": ..., "input": <text>, "dimensions": <int>?}` (`dimensions` field omitted when zero/unset). Response parsed as `data[0].embedding` → `[]float32`. Default `BaseURL = https://api.openai.com/v1`.
- REQ-F-017: **Ollama** — `POST {base_url}/api/embed`, no auth, body `{"model": ..., "input": <text>}`. Response parsed as `embeddings[0]` → `[]float32`. Default `BaseURL = http://localhost:11434`. `dimensions` is ignored (Ollama doesn't honour it).
- REQ-F-018: **HuggingFace** — bearer auth (`HF_TOKEN`). When `BaseURL` equals the default `https://router.huggingface.co/hf-inference/models`, request is `POST {base_url}/{model}` (serverless mode). When `BaseURL` is anything else, request is `POST {base_url}` (dedicated endpoint mode). Body `{"inputs": <text>}`. Response tolerated as either `[[floats]]` or `[floats]`.
- REQ-F-019: Provider errors (non-2xx, malformed JSON, missing fields) return wrapped errors that include the provider name and HTTP status (when applicable). Non-2xx responses do NOT leak Authorization headers in error text.

#### Credentials store

- REQ-F-020: A new `EmbedCredentials` collection is added at `cfg.Credentials.Embed`, with the same shape and methods as `DbmsCredentials`: `Add`, `Remove`, `SetDefault`, `GetDefault`, `Get`, `List`, `Printable`. Persisted in the same `credentials.json` file under a top-level `"embed"` key (alongside existing `"aura"` and `"dbms"` keys).
- REQ-F-021: `EmbedCredential` fields: `Name`, `Provider` (`openai|ollama|huggingface`), `Model`, `BaseURL` (optional), `Dimensions` (int, optional), `APIKey` (optional, since Ollama doesn't need one). JSON keys are kebab-case (`base-url`, `api-key`).
- REQ-F-022: `PrintableEmbedCredentials.AsArray` and `MarshalJSON` MUST omit the `api-key` field — same omit-secret pattern as `PrintableDbmsCredentials` for `password`.
- REQ-F-023: First credential added becomes the default automatically (mirrors `DbmsCredentials.Add` behaviour).

#### Credential management commands

- REQ-F-024: New cobra subtree `neo4j-cli credential embed {add, list, remove, use}` mirroring `credential dbms {add, list, remove, use}`.
- REQ-F-025: `credential embed add` flags: `--name` (required), `--provider` (required, validated against `{openai, ollama, huggingface}` — invalid value returns a usage error), `--model` (required), `--api-key` (optional), `--base-url` (optional), `--dimensions` (optional int).
- REQ-F-026: `credential embed list` columns: `name`, `provider`, `model`, `base-url`, `dimensions`, `default`. The `api-key` column is NEVER shown.
- REQ-F-027: `credential embed remove <name>` is non-cascading. Stale references from `DbmsCredential.EmbedCredential` are surfaced lazily at query time as `clierr.NewUsageError("dbms credential %q references missing embed credential %q; run 'neo4j-cli credential dbms set-embed %s' to update", dbmsName, embedName, dbmsName)`.

#### Dbms ↔ embed link

- REQ-F-028: `DbmsCredential` gains an optional `EmbedCredential string \`json:"embed-credential,omitempty"\`` field. JSON omitempty so existing on-disk credentials.json files round-trip unchanged.
- REQ-F-029: `credential dbms add` accepts a new optional `--embed-credential <name>` flag. When set, the cobra leaf validates the named embed cred exists via `cfg.Credentials.Embed.Get(name)` BEFORE calling `cfg.Credentials.Dbms.Add(...)`. Validation failure surfaces as a usage error pointing at `credential embed list` and does NOT create the dbms credential.
- REQ-F-030: A new leaf `credential dbms set-embed <dbms-name> [embed-name]` updates the link on an existing dbms cred. With one positional arg, the link is cleared. With two positional args, the second is validated to exist before being persisted. Annotated `write: true`. Dbms cred not found → usage error. Embed cred not found (when set) → usage error pointing at `credential embed list`.
- REQ-F-031: `credential dbms list` always includes an `embed-credential` column (empty string when unset). The column is added at the end of `dbmsCredentialFields`.
- REQ-F-032: `PrintableDbmsCredentials.AsArray` and `MarshalJSON` include the `embed-credential` key (with empty string when unset). On-disk JSON omits the field when empty (`omitempty`); the printable view always includes the key.

### Non-Functional Requirements

- REQ-NF-001: All new/modified Go source files carry the Neo4j copyright header (existing `addlicense` CI gate).
- REQ-NF-002: `make test`, `make lint`, and `make fmt-check` pass with no failures.
- REQ-NF-003: Skill bundle is regenerated via `go generate ./neo4j-cli/internal/skill/...` after the cobra command tree changes, and `make generate-check` exits 0. (`TestGenerator_RoundTrip` is the gate for stale bundles.)
- REQ-NF-004: A changelog entry is created for `neo4j-cli` via `changie new --projects neo4j-cli --kind Minor --body ...` (user-facing surface change).
- REQ-NF-005: Tests follow the colocated `*_test.go` per leaf convention; new leaf tests sit next to their source. Table-driven tests are preferred for parser-style code (`parseParams`, `embed.Resolve`).
- REQ-NF-006: `query` package tests must continue to use `testfs.GetTestFs(...)` and never `afero.NewOsFs()`. New embed-related tests follow the same rule.
- REQ-NF-007: All HTTP calls to embedding providers respect `cmd.Context()` for cancellation. The default HTTP client has no timeout, but the context is inherited so callers (and signal handlers) can cancel.
- REQ-NF-008: Provider HTTP errors do NOT leak the API key in returned messages. Authorization headers are never logged.
- REQ-NF-009: `LF` line endings preserved on all committed `.md` / golden / bundle files (existing `.gitattributes` rules cover any new bundle paths).
- REQ-NF-010: New embedding code paths add no new direct dependencies beyond the Go standard library where possible. (No HTTP-client framework; `net/http` is sufficient.)
- REQ-NF-011: Generated documentation (skill bundle `references/`) reflects the new flags, the new leaves, and the new `credential embed` subtree.

## Technical Considerations

### Architecture: where things live

- `common/clicfg/credentials/embed.go` (NEW) — `EmbedCredential`, `EmbedCredentials`, `PrintableEmbedCredentials`. Mirrors `dbms.go`.
- `common/clicfg/credentials/credentials.go` (MODIFY) — add `Embed *EmbedCredentials` to `CredentialsFile` + `Credentials`; init in `load()` + rewire `onUpdate` post-unmarshal (same pattern as `Dbms`, lines 45-65).
- `common/clicfg/credentials/dbms.go` (MODIFY) — add `EmbedCredential` field on `DbmsCredential`; add `SetEmbed(dbmsName, embedName string) error` method; update `PrintableDbmsCredentials.AsArray` to include the new column.
- `neo4j-cli/internal/subcommands/credential/embed/{embed,add,list,remove,use,*_test}.go` (NEW) — copy of the dbms leaf cluster, adapted for the embed cred shape.
- `neo4j-cli/internal/subcommands/credential/credential.go` (MODIFY) — `cmd.AddCommand(embed.NewCmd(cfg))`.
- `neo4j-cli/internal/subcommands/credential/dbms/add.go` (MODIFY) — `--embed-credential` flag + leaf-side validation.
- `neo4j-cli/internal/subcommands/credential/dbms/set_embed.go` (NEW) + `set_embed_test.go` (NEW) — new leaf `credential dbms set-embed`.
- `neo4j-cli/internal/subcommands/credential/dbms/dbms.go` (MODIFY) — `cmd.AddCommand(newSetEmbedCmd(cfg))`.
- `neo4j-cli/internal/subcommands/credential/dbms/list.go:12` (MODIFY) — append `"embed-credential"` to `dbmsCredentialFields`.
- `neo4j-cli/query/embed/{embed,openai,ollama,huggingface,*_test}.go` (NEW) — `Provider` interface, three HTTP clients, `Config`, `Resolve(cmd, cfg)`, `New(cfg)` factory + `providerFactory` test seam.
- `neo4j-cli/query/embed.go` (NEW) + `embed_test.go` (NEW) — `:embed` leaf + tests.
- `neo4j-cli/query/input.go` (NEW) — extracted helper `readPositionalOrStdin(cmd, args, name string) (string, error)`. `resolveCypher` (currently in `run.go`) and the new `runEmbed` both call it.
- `neo4j-cli/query/params.go` (MODIFY) — `parseParams` returns `(map[string]any, []EmbedJob, error)`.
- `neo4j-cli/query/query.go` (MODIFY) — register the five new persistent flags + `cmd.AddCommand(newEmbedCmd(cfg))`.
- `neo4j-cli/query/run.go` (MODIFY) — wire embed resolution before EXPLAIN preflight.
- `neo4j-cli/query/connect.go` — read-only reference for `findDotenv` and the env-var pattern (no edits required if `embed.Resolve` calls `findDotenv` directly).

### Embed resolution helper

`embed.Resolve(cmd *cobra.Command, cfg *clicfg.Config) (Config, error)` lives in `neo4j-cli/query/embed/`. It:

1. Picks the base embed cred via the first-match list in REQ-F-013 step 4. The "resolved dbms cred" is selected by an `embed`-package-private helper `resolveSelectedDbmsCred(cmd, cfg)` that returns the cred named by `--credential <name>` if `cmd.Flag("credential").Changed`, else `cfg.Credentials.Dbms.GetDefault()`, else nil.
2. Loads `.env` via the existing `findDotenv` walker (extracted from `query/connect.go` if necessary, or imported package-publicly — the package boundary is internal to the binary, so a small `query.FindDotenv` export is acceptable).
3. Overlays env vars, then flags, in that order.
4. Resolves API key per provider with the precedence in REQ-F-013.
5. Returns a populated `Config`. Validation (required `Provider`, required `Model`, required API key for OpenAI/HuggingFace) happens in `New(cfg)`, NOT in `Resolve`, so the standalone leaf can produce a clear error path.

### Run-time flow (with `:embed` modifier)

In `runQuery` (`neo4j-cli/query/run.go`), after `parseParams`:

```go
rawParams, _ := cmd.Flags().GetStringArray("param")
params, embeds, err := parseParams(rawParams)
if err != nil { return clierr.NewUsageError("%s", err.Error()) }

if len(embeds) > 0 {
    ec, err := embed.Resolve(cmd, cfg);                if err != nil { return err }
    prov, err := providerFactory(ec);                  if err != nil { return err }
    for _, j := range embeds {
        v, err := prov.Embed(cmd.Context(), j.Text);    if err != nil { return err }
        params[j.Name] = v
    }
}
// existing flow continues: resolveConn → openDriver → rejectWriteCypher → runStatement
```

The `params` map is populated once. Both the EXPLAIN preflight and the real run consume the same map, so the embedding API is hit at most once per param per invocation.

### Run-time flow (`:embed` standalone leaf)

```go
text, err := readPositionalOrStdin(cmd, args, "text")
if err != nil { return err }

ec, err := embed.Resolve(cmd, cfg);            if err != nil { return err }
prov, err := providerFactory(ec);              if err != nil { return err }
v, err := prov.Embed(cmd.Context(), text);     if err != nil { return err }

commonoutput.PrintBodyMap(cmd, cfg, embedVector(v), nil)
```

`embedVector` is a `[]float32` newtype that implements `ResponseData` (`AsArray() []map[string]any`) and `json.Marshaler` to produce the raw vector form for json/toon and the `[N floats]` summary for table.

### Testing approach

- `params_test.go` extension — table cases for the new modifier syntax (REQ-F-001 through REQ-F-006).
- `embed/embed_test.go` — config resolution precedence matrix using `httptest` + `testfs.GetTestFs`. Covers all four base-cred selection paths + each overlay tier.
- `embed/openai_test.go`, `ollama_test.go`, `huggingface_test.go` — HTTP behaviour using `httptest.NewServer`. Cover happy path, missing API key (where applicable), non-200 responses, malformed bodies, both HuggingFace serverless/dedicated paths.
- `query/embed_test.go` (leaf) — fake `providerFactory` returning a fixed vector; assert json/table/toon outputs and that no Bolt driver is opened (a `driverOpener` that panics confirms).
- `run_test.go` extension — end-to-end `--param q:embed=…` with the provider seam mocked.
- `credentials/embed_test.go` — round-trip + Printable omits api-key.
- `credentials/dbms_test.go` — `SetEmbed` happy path, JSON round-trip preserves `EmbedCredential`, Printable always includes the column.
- `credential/embed/*_test.go` — leaf tests mirroring the dbms ones.
- `credential/dbms/set_embed_test.go` — happy path, missing dbms cred, missing embed cred, unset.
- `credential/dbms/add_test.go` extension — `--embed-credential` with valid + missing target.

### Skill bundle

The cobra tree gains five persistent flags under `query` plus a new `:embed` leaf and a new `credential embed` subtree, so `references/query.md`, `references/credential.md`, and `SKILL.md` flag tables all change. Run `go generate ./neo4j-cli/internal/skill/...` once after all command-tree edits land. CI's `make generate-check` will catch a missed regen.

### Backwards compatibility

- Existing `--param key=value` syntax is unchanged. The `:` is only meaningful when present in the key portion before the first `=`.
- Existing `credentials.json` files round-trip unchanged: new fields use `omitempty`; missing top-level `"embed"` is initialised to an empty `EmbedCredentials` on load.
- Existing `credential dbms list` output gains one trailing column. JSON output gains one trailing key (`"embed-credential": ""`). Scripts that do positional column parsing of the table form will break, but JSON consumers using key access will not. Document in changelog.

## Acceptance Criteria

### Param modifier

- [ ] `neo4j-cli query --param q:embed='hello'` with mock provider → `$q` bound to the mock's vector; query runs.
- [ ] `neo4j-cli query --param q:embed='hello' --param k=5 "..."` mixes embed + literal params correctly.
- [ ] `neo4j-cli query --param q:bogus=x "..."` returns a usage error naming `bogus`.
- [ ] `neo4j-cli query --param :embed=x "..."` returns a usage error about the empty parameter name.
- [ ] `neo4j-cli query --param q:embed='[1,2,3]' "..."` returns the JSON-array rejection error.
- [ ] Embedding happens once even with `--rw` set; mock provider is called exactly len(embeds) times per invocation.

### Standalone `:embed` leaf

- [ ] `neo4j-cli query :embed "hello"` with `--format json` prints a JSON array of floats.
- [ ] `neo4j-cli query :embed "hello"` with `--format table` prints a single-cell `[N floats]`.
- [ ] `echo "hello" | neo4j-cli query :embed` reads from stdin.
- [ ] `neo4j-cli query :embed` with no arg and a TTY stdin returns a usage error.
- [ ] `neo4j-cli query :embed "hello"` does not open a Bolt driver (test asserts via panicking `driverOpener`).
- [ ] `neo4j-cli query :embed "hello"` does not prompt for a password even with no `--password`/`NEO4J_PASSWORD`.

### Embed config resolution

- [ ] CLI flags override env vars override `.env` overrides stored cred (covered by table-driven test).
- [ ] `--embed-credential nonexistent` returns a usage error pointing at `credential embed list`.
- [ ] OpenAI provider with no key in env / .env / cred returns `"missing API key for openai: set OPENAI_API_KEY"`.
- [ ] HuggingFace provider with no key returns `"missing API key for huggingface: set HF_TOKEN"`.
- [ ] Ollama provider with no key works (no key required).
- [ ] `--embed-credential openai-shared` overrides a different embed cred linked from the resolved dbms cred.

### Provider HTTP

- [ ] OpenAI client posts to `{base_url}/embeddings` with bearer auth and parses `data[0].embedding`.
- [ ] OpenAI client omits `dimensions` from the request body when zero/unset; includes it when set.
- [ ] Ollama client posts to `{base_url}/api/embed` with no auth and parses `embeddings[0]`.
- [ ] HuggingFace client posts to `{base_url}/{model}` when base-url is the default; to `{base_url}` raw when overridden.
- [ ] HuggingFace client tolerates both `[[floats]]` and `[floats]` response shapes.
- [ ] Non-2xx responses surface a wrapped error containing the provider name and HTTP status; no `Authorization` header value appears in the error.
- [ ] Context cancellation aborts the in-flight HTTP call (test via `httptest.NewServer` + a slow handler + cancelled context).

### Credential storage and management

- [ ] `credential embed add --name x --provider openai --model y --api-key k` creates a credential and marks it default (first added).
- [ ] `credential embed add --name x --provider bogus --model y` returns a usage error about invalid provider.
- [ ] `credential embed list` shows `name`, `provider`, `model`, `base-url`, `dimensions`, `default` columns. `api-key` is NEVER shown.
- [ ] `credential embed remove x` succeeds; `credential embed list` no longer shows `x`.
- [ ] `credential embed use y` sets `y` as the default.
- [ ] `credentials.json` on disk omits `api-key` for embed creds with no key (Ollama) and includes it for ones with a key (OpenAI).

### Dbms ↔ embed link

- [ ] `credential dbms add --name p --uri ... --username ... --password ... --embed-credential e` succeeds when `e` exists.
- [ ] Same with non-existent `e` returns a usage error before the dbms cred is created (verify by listing dbms creds afterwards).
- [ ] `credential dbms set-embed p e` links `p → e`.
- [ ] `credential dbms set-embed p` (no second arg) clears the link.
- [ ] `credential dbms set-embed missing e` returns a "dbms cred not found" usage error.
- [ ] `credential dbms set-embed p missing` returns an "embed cred not found" usage error.
- [ ] `credential dbms list` always includes an `embed-credential` column (empty string when unset).
- [ ] After `credential embed remove e` while `p` is linked to `e`, `query --credential p --param q:embed=x ...` returns the stale-link usage error pointing at `set-embed`.

### Cross-cutting

- [ ] `make test`, `make lint`, `make fmt-check` all green.
- [ ] `make generate-check` exits 0 after `go generate ./...`.
- [ ] Changelog entry present for `neo4j-cli` (kind `Minor`).
- [ ] Existing `query` tests (no embed params, no `:embed` leaf) pass byte-identical to before — opt-in default verified.
- [ ] `NEO4J_BOLT_TEST=1 go test -run TestBolt_Smoke -v ./neo4j-cli/query/...` passes (regular query path unbroken).

## Out of Scope

- Embedding result caching (deferred to a follow-up PRD).
- `--param NAME:embed=@file.txt` reading text from a file.
- A leaf-local `--dimensions` flag distinct from `--embed-dimensions`.
- New providers beyond the three listed (Cohere, Vertex, Bedrock, etc.).
- Re-embedding when params would otherwise be cached or memoised across invocations.
- New `--format` values (e.g. Rust repo's `--format raw` newline-per-float).
- Cascade-delete or reference-count enforcement for embed creds linked from dbms creds.
- Adding embed config under any command tree other than `query` (e.g. no Aura embed support).
- `credential dbms update`-style edit commands beyond `set-embed`.

## Open Questions

- None. (Provider set, output shape, list-column visibility, HuggingFace base-url heuristic, dimensions storage, file-value syntax, leaf-local dimensions flag, JSON-array rejection, and embedding-cache scope are all resolved in the source plan and confirmed in clarifying Q&A.)
