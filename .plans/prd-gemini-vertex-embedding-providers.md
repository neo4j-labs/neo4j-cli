# PRD: Gemini and Vertex AI Embedding Providers

## Overview

Add two concrete embedding providers to `neo4j-cli query --param key:embed=<text>`:

- **`gemini`** — Google Generative Language API (API-key auth, `x-goog-api-key`
  header).
- **`vertex`** — Google Vertex AI Prediction API (OAuth via Application Default
  Credentials, `Authorization: Bearer` header).

Today the embed subsystem supports `openai`, `ollama`, and `huggingface`.
Document Intelligence stores graphs with `gemini-embedding-001` (3072 dims) as
the only supported model, which means anyone building on top of a DI graph
cannot use `neo4j-cli query --param q:embed=...` to test vector search and must
fall back to Python.

Tracking: [CLI-193](https://linear.app/neo4j/issue/CLI-193). Source plan:
`~/.claude/plans/time-to-take-on-curious-reddy.md`.

The implementation deliberately skips the "model abstraction that can handle
local and remote models" suggested in the Linear issue — that is a larger
refactor and not required to unblock the DI use case.

## Goals

- Let users embed a query string with Gemini and use the returned vector
  against existing `gemini-embedding-001` document embeddings in Neo4j.
- Match the existing provider style (plain `net/http`, ~120 lines per
  provider) so the embed package stays consistent.
- Keep the credential-storage and `query` flag surface minimally extended:
  only the two new vertex-specific fields (`vertex-project`,
  `vertex-location`), no new per-query flags.
- Default sensibly for the documented use case (always send
  `taskType=RETRIEVAL_QUERY`, L2-normalize non-default-dim gemini-001 output)
  while letting Gemini ignore fields it does not honour.

## Non-Goals

- No model-abstraction refactor — `gemini` and `vertex` are siblings of
  `openai`/`ollama`/`huggingface`, not a unifying layer.
- No per-call `--task-type` flag. `RETRIEVAL_QUERY` is hardcoded.
- No batch endpoints (`:batchEmbedContents` / `:batchPredict`) —
  `neo4j-cli` only ever embeds one query at a time today.
- No service-account JSON file as a stored credential field. ADC already
  honours `GOOGLE_APPLICATION_CREDENTIALS` via env, which is sufficient.
- No changes to `query`'s per-call flag set (no `--vertex-project` etc. on
  `query`). Vertex project/location are stored-cred-only.
- No support for non-default Vertex API endpoints (e.g., private endpoints,
  global routing). Use the regional `{location}-aiplatform.googleapis.com`
  pattern exclusively.

## Requirements

### Functional Requirements

#### Provider constants and Config (`neo4j-cli/query/embed/embed.go`)

- **REQ-F-001**: Two new provider name constants are added:
  `ProviderGemini = "gemini"` and `ProviderVertex = "vertex"`.
- **REQ-F-002**: Two new env-var constants are added:
  `envGeminiKey = "GEMINI_API_KEY"` and `envGoogleKey = "GOOGLE_API_KEY"`.
- **REQ-F-003**: `Config` gains two string fields, `VertexProject` and
  `VertexLocation`. Both are populated only when the resolved provider is
  `vertex`; for other providers they remain empty and are ignored.
- **REQ-F-004**: `New(cfg Config) (Provider, error)` switch grows two new
  cases that return `newGeminiProvider(cfg)` and `newVertexProvider(cfg)`
  respectively. The `default` branch's error message updates to include the
  new providers in the "must be one of …" list.

#### API-key resolution for `gemini` (`embed.go` `resolveAPIKey`)

- **REQ-F-010**: For `provider == ProviderGemini`, `resolveAPIKey` layers
  `envGeminiKey` then `envGoogleKey` within each precedence stage (`.env`,
  OS env), so the chain becomes (lowest → highest): stored cred APIKey →
  `.env` `NEO4J_EMBED_API_KEY` → `.env` `GOOGLE_API_KEY` → `.env`
  `GEMINI_API_KEY` → OS `NEO4J_EMBED_API_KEY` → OS `GOOGLE_API_KEY` → OS
  `GEMINI_API_KEY`.
- **REQ-F-011**: `osEnvSnapshot()` includes `envGeminiKey` and
  `envGoogleKey` so `t.Setenv` in tests is picked up.
- **REQ-F-012**: `vertex` does not participate in `resolveAPIKey` — its auth
  is OAuth ADC, not a stored or env API key.

#### `gemini` provider (`neo4j-cli/query/embed/gemini.go`)

- **REQ-F-020**: A new `gemini.go` file mirrors `openai.go` in shape:
  unexported `geminiProvider` struct with `cfg Config` and `client
  *http.Client`, plus `newGeminiProvider(cfg) *geminiProvider`.
- **REQ-F-021**: `defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"`
  is applied at request time when `cfg.BaseURL` is empty (matching the
  openai pattern — `Resolve` does not substitute it so a stored override
  wins cleanly).
- **REQ-F-022**: The request URL is `{base}/models/{model}:embedContent`.
- **REQ-F-023**: Authentication uses the HTTP header
  `x-goog-api-key: <key>` (never `Authorization: Bearer`).
- **REQ-F-024**: The request JSON body has shape
  `{"content":{"parts":[{"text":"<text>"}]},"taskType":"RETRIEVAL_QUERY","outputDimensionality":<N>}`.
  `outputDimensionality` is a pointer with `omitempty` and is set only when
  `cfg.Dimensions > 0`.
- **REQ-F-025**: `taskType` is always sent as the literal string
  `"RETRIEVAL_QUERY"`. There is no model-aware branching — Gemini ignores
  unknown fields per the API spec, so newer models that do not honour
  `taskType` will simply discard it.
- **REQ-F-026**: The response shape is `{"embedding":{"values":[<float>...]}}`.
  The returned slice is `decoded.Embedding.Values`. An empty `Values`
  yields `fmt.Errorf("gemini: empty embedding in response")`.
- **REQ-F-027**: When `cfg.Dimensions > 0 && cfg.Dimensions != 3072`, the
  provider L2-normalizes the returned vector before returning it. The
  normalization is unconditional under that condition — there is no flag to
  disable it. A zero-vector edge case is left as-is (division by zero is
  guarded; the original vector passes through).
- **REQ-F-028**: Missing API key returns a `clierr.NewAuthError` whose copy
  lists the resolution chain: `GEMINI_API_KEY`, `GOOGLE_API_KEY`,
  `NEO4J_EMBED_API_KEY`, and `neo4j-cli credential embed add`.
- **REQ-F-029**: Base-URL validation uses `urlcheck.ValidateRemoteURL`, the
  same helper openai uses.
- **REQ-F-030**: Non-2xx responses surface a wrapped error
  (`gemini: HTTP <code>: <snippet>`) reading up to 4 KiB of the body for
  context. The API key never appears in any error message.
- **REQ-F-031**: User-Agent and Content-Type/Accept headers follow the
  openai pattern: `Content-Type: application/json`, `Accept:
  application/json`, `User-Agent: <cfg.UserAgent>` when non-empty.

#### `vertex` provider (`neo4j-cli/query/embed/vertex.go`)

- **REQ-F-040**: A new `vertex.go` file follows the same shape as
  `gemini.go` but with OAuth ADC auth.
- **REQ-F-041**: Validation in `Embed` (not `New`) returns
  `clierr.NewUsageError` when `cfg.VertexProject == ""` or
  `cfg.VertexLocation == ""`. Messages reference `--vertex-project` /
  `--vertex-location` and `neo4j-cli credential embed add --vertex-project
  --vertex-location`.
- **REQ-F-042**: The request URL is
  `https://{cfg.VertexLocation}-aiplatform.googleapis.com/v1/projects/{cfg.VertexProject}/locations/{cfg.VertexLocation}/publishers/google/models/{cfg.Model}:predict`.
  `cfg.BaseURL` is intentionally NOT consulted — Vertex's URL is
  location-derived and a stored `base-url` would be ambiguous.
- **REQ-F-043**: The request JSON body has shape
  `{"instances":[{"content":"<text>","task_type":"RETRIEVAL_QUERY"}],"parameters":{"outputDimensionality":<N>}}`.
  Note `task_type` is snake_case here (Vertex convention) — distinct from
  Gemini's camelCase `taskType`. `parameters` is omitted entirely when
  `cfg.Dimensions == 0`.
- **REQ-F-044**: The response shape is
  `{"predictions":[{"embeddings":{"values":[<float>...]}}]}`. The returned
  slice is `decoded.Predictions[0].Embeddings.Values`.
- **REQ-F-045**: The OAuth token is obtained from a `oauth2.TokenSource`
  created via `google.FindDefaultCredentials(ctx,
  "https://www.googleapis.com/auth/cloud-platform")`. The token source is
  initialized lazily inside `Embed` (not `newVertexProvider`) so a
  programmatic caller can construct a Config without GCP creds for
  inspection.
- **REQ-F-046**: ADC lookup failure surfaces a `clierr.NewAuthError` whose
  copy mentions `gcloud auth application-default login` and
  `GOOGLE_APPLICATION_CREDENTIALS`. The provider does NOT attempt to read
  the service-account JSON file directly — the oauth2 helper handles all
  ADC sources transparently.
- **REQ-F-047**: A package-level test seam allows tests to override the
  token source without touching real ADC. The default value resolves
  through `google.FindDefaultCredentials`; tests assign a stub that
  returns a fixed `oauth2.StaticTokenSource`.
- **REQ-F-048**: A second package-level test seam allows tests to override
  the URL template (e.g., to substitute `httptest.NewServer().URL` for the
  `https://{location}-aiplatform.googleapis.com` prefix). The default
  builds the canonical URL per REQ-F-042.
- **REQ-F-049**: Vertex does NOT perform L2-normalization on the response —
  Vertex's `gemini-embedding-001` returns already-normalized vectors at all
  supported dimensionalities (per Vertex API docs). The provider trusts the
  upstream.
- **REQ-F-050**: Non-2xx, URL validation, error wrapping, and User-Agent /
  Content-Type / Accept header handling all match the gemini provider.

#### Credential storage (`common/clicfg/credentials/embed.go`)

- **REQ-F-060**: `EmbedCredential` gains two new fields:
  `VertexProject string` with JSON tag `"vertex-project,omitempty"` and
  `VertexLocation string` with JSON tag `"vertex-location,omitempty"`.
  `omitempty` keeps non-vertex credentials' on-disk shape unchanged.
- **REQ-F-061**: `EmbedCredentials.Add` gains two new trailing string
  parameters `vertexProject, vertexLocation` and persists them on the new
  `EmbedCredential`. All call sites update.
- **REQ-F-062**: `PrintableEmbedCredentials.AsArray()` adds
  `"vertex-project"` and `"vertex-location"` keys to the per-row map only
  when their value is non-empty (so the table for openai/ollama/huggingface
  rows stays unchanged).
- **REQ-F-063**: The marshalled JSON for `PrintableEmbedCredentials` omits
  empty vertex-* fields (consistent with `AsArray`).

#### CLI add command (`neo4j-cli/internal/subcommands/credential/embed/add.go`)

- **REQ-F-070**: `validProviders` is extended to include `"gemini"` and
  `"vertex"`. The error message produced by the validator updates
  automatically (it joins `validProviders`).
- **REQ-F-071**: Two new flags are registered on `credential embed add`:
  `--vertex-project <string>` and `--vertex-location <string>`. Both
  default to empty.
- **REQ-F-072**: Pre-`Add` validation: when `--provider vertex`, both
  `--vertex-project` and `--vertex-location` are required. Missing either
  returns `clierr.NewUsageError`.
- **REQ-F-073**: Pre-`Add` validation: when `--provider` is anything other
  than `vertex` and either `--vertex-project` or `--vertex-location` is
  set, `add` returns a `clierr.NewUsageError` ("--vertex-* flags only
  apply when --provider=vertex"). This is a hard error, not a warning, to
  prevent silently-saved-but-ignored config.
- **REQ-F-074**: The flag values flow into `EmbedCredentials.Add` (new
  trailing args from REQ-F-061).

#### Resolve plumbing (`embed.go` `Resolve`)

- **REQ-F-080**: When the base stored credential is non-nil, `Resolve`
  copies `base.VertexProject` and `base.VertexLocation` into `Config`
  alongside the existing `Provider`/`Model`/`BaseURL`/`Dimensions`/`APIKey`
  copy.
- **REQ-F-081**: No new env vars or flags override `VertexProject` /
  `VertexLocation` — these are stored-credential-only (matches REQ in
  Non-Goals).

#### Dependencies

- **REQ-F-090**: `golang.org/x/oauth2` is added as a direct module
  dependency. `go mod tidy` is run; the diff in `go.mod`/`go.sum` is
  limited to oauth2 and any transitively new deps needed by
  `google.FindDefaultCredentials` (`cloud.google.com/go/compute/metadata`
  is acceptable; anything larger triggers a scope review per the plan
  file).

#### Skill bundle regeneration

- **REQ-F-100**: `go generate ./neo4j-cli/internal/skill/...` is run after
  source changes. The diff must include updates to:
  - `references/credential.md` (new `--vertex-project` / `--vertex-location`
    flags and updated provider list on `credential embed add`),
  - `references/query.md` only if any embed-related help-text changes
    bleed through (none expected), and
  - any other auto-derived files flagged by `TestGenerator_RoundTrip`.

#### Changelog

- **REQ-F-110**: A new changie entry is added via `changie new --projects
  neo4j-cli --kind Minor --body "Added Gemini and Vertex AI embedding
  providers."`. The resulting YAML lives at
  `.changes/unreleased/neo4j-cli-Minor-<timestamp>.yaml`.

### Non-Functional Requirements

- **REQ-NF-001**: Both providers honour `ctx` for cancellation. Neither
  installs a client-side timeout on `*http.Client` — cancellation is the
  caller's responsibility (matches the existing openai/ollama/huggingface
  contract documented at `embed.go:28`).
- **REQ-NF-002**: API keys, OAuth access tokens, and ADC source paths never
  appear in error messages, log lines, or `--format json` output.
- **REQ-NF-003**: All new tests pass on the existing CI matrix
  (ubuntu/windows/macos) and run without network access. `gemini_test.go`
  and `vertex_test.go` use `httptest.NewServer`; the vertex token source
  is stubbed via the seam in REQ-F-047.
- **REQ-NF-004**: Bundle drift is detected by `make generate-check` (the
  existing CI gate). No CI-level changes are needed.
- **REQ-NF-005**: New code passes `make fmt-check`, `make lint`, and
  `make license-check`. License headers are present on `gemini.go` /
  `vertex.go` / `gemini_test.go` / `vertex_test.go`.

## Technical Considerations

- **Auth-model split between providers**: gemini uses an API-key header,
  vertex uses an OAuth Bearer token from ADC. They are kept as two
  providers (not one with a sub-mode) so each file stays ~150 lines and the
  flag/credential surface is unambiguous (`--vertex-project` only makes
  sense for vertex).
- **`net/http` not the genai SDK**: keeps the dependency footprint
  contained and matches the existing provider style. The only new dep is
  `golang.org/x/oauth2` (for vertex's ADC).
- **`taskType` always-sent**: Gemini's spec ignores unknown fields, so
  newer models that drop `taskType` simply discard it. This avoids a
  model-aware switch table that would drift as Google adds models.
- **L2-normalization scope**: gemini-001 returns un-normalized vectors at
  non-default `outputDimensionality`. Neo4j's `db.index.vector.queryNodes`
  with `COSINE` is mathematically invariant to normalization, but
  `DOT_PRODUCT` indexes assume unit vectors. Normalizing in the provider
  removes the foot-gun. Vertex's pipeline returns normalized vectors
  already, so vertex skips the normalization step.
- **Vertex regional URL**: the request URL embeds the location twice
  (host prefix + path). The provider builds it once per call; the
  URL-template test seam (REQ-F-048) substitutes the host prefix only,
  preserving the full path so request-body assertions stay realistic.
- **Token caching**: `oauth2.TokenSource` is wrapped in `oauth2.ReuseTokenSource`
  by `google.FindDefaultCredentials`, so per-call `Token()` invocations
  return the cached token until it expires. No extra caching needed at the
  provider level.
- **`base-url` semantics for vertex**: stored `BaseURL` on a vertex
  credential is ignored. The CLI does not warn on stored-but-ignored
  values — a future PRD could surface that, but it is outside scope here.
- **`add` flag-misuse guard** (REQ-F-073): rejecting `--vertex-*` for
  non-vertex providers is a hard error so users get immediate feedback
  rather than a silently-saved-but-ignored credential.

## Acceptance Criteria

- [ ] `neo4j-cli credential embed add --name gtest --provider gemini --model
      gemini-embedding-001 --api-key <key>` succeeds; the resulting JSON on
      disk shows the new credential with no `vertex-project` /
      `vertex-location` keys present.
- [ ] `neo4j-cli credential embed add --name vtest --provider vertex --model
      gemini-embedding-001 --vertex-project myproj --vertex-location
      us-central1` succeeds; the resulting JSON on disk shows the new
      credential with both vertex-* fields populated.
- [ ] `neo4j-cli credential embed add --name x --provider vertex --model
      gemini-embedding-001` fails with a usage error naming
      `--vertex-project` and `--vertex-location`.
- [ ] `neo4j-cli credential embed add --name x --provider openai --model
      text-embedding-3-small --api-key sk-... --vertex-project p` fails with
      a usage error explaining that `--vertex-*` only applies to vertex.
- [ ] `neo4j-cli credential embed list` shows the new credentials. The
      gemini row does not contain vertex-* columns; the vertex row contains
      them in both table and JSON output.
- [ ] `GEMINI_API_KEY=… neo4j-cli query --param 'q:embed=hello world' --embed-
      provider gemini --embed-model gemini-embedding-001 --embed-dimensions
      3072 'RETURN size($q) AS dims'` (against a local docker neo4j) returns
      `dims=3072`.
- [ ] Same as above with `--embed-dimensions 768` returns `dims=768` and the
      returned vector's L2 norm is within `1e-5` of 1.0.
- [ ] Vertex e2e (manual, requires GCP creds): `gcloud auth
      application-default login`; then `neo4j-cli query --param 'q:embed=…'
      --embed-provider vertex --embed-model gemini-embedding-001` works
      against a credential with `vertex-project` / `vertex-location` set.
- [ ] Missing-key on gemini → `clierr.AuthError` listing the env-var chain
      (`GEMINI_API_KEY`, `GOOGLE_API_KEY`, `NEO4J_EMBED_API_KEY`).
- [ ] Missing ADC on vertex → `clierr.AuthError` mentioning
      `gcloud auth application-default login` and
      `GOOGLE_APPLICATION_CREDENTIALS`.
- [ ] Non-2xx from either provider → wrapped error with up-to-4KiB body
      snippet; no API key or access token in the message.
- [ ] All new tests (`gemini_test.go`, `vertex_test.go`, updated
      `add_test.go`, updated `embed_test.go`) pass on ubuntu/windows/macos.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check`,
      `make generate-check` all pass.
- [ ] `go generate ./neo4j-cli/internal/skill/...` produces a non-empty diff
      under `neo4j-cli/internal/skill/bundle/`, and the bundle is committed
      alongside the source changes.
- [ ] `.changes/unreleased/neo4j-cli-Minor-<timestamp>.yaml` exists with the
      expected body.
- [ ] `go.mod` adds a direct `golang.org/x/oauth2` dependency; the
      `go.sum` diff stays contained to oauth2 + ADC transitive deps.

## Out of Scope

- The "model abstraction that can handle local and remote models" suggested
  in the Linear issue. Tracked separately if/when requested.
- Per-call `--task-type` flag.
- Streaming or batch embedding endpoints (`:batchEmbedContents` /
  `:batchPredict`).
- Service-account JSON file as a stored credential field (`ADC` already
  honours `GOOGLE_APPLICATION_CREDENTIALS` env).
- Per-call vertex project/location flags on `query`.
- Surfacing a warning when a stored credential has fields that the
  resolved provider ignores (e.g., `base-url` on a vertex cred).
- Private/global Vertex endpoints. Only regional
  `{location}-aiplatform.googleapis.com` is supported.

## Open Questions

None. All design decisions were resolved during plan review:

- Vertex `Location` is required (no default `us-central1`).
- L2-normalization on gemini is unconditional for non-3072 dimensionalities.
- `taskType` is always sent for gemini regardless of model.
- `--vertex-*` flags on a non-vertex `add` invocation are a hard error.
