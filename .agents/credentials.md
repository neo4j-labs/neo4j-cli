# Credentials Storage Notes

Patterns for `CredentialsFile` and the `Aura` / `Dbms` / `Embed` credential types.

## load() re-wiring

- `Credentials.load()` re-wires `onUpdate` on Aura, Dbms, AND Embed after JSON unmarshal — JSON decode creates a new struct pointer that loses the callback. Mirror this pattern for any future credential type added to `CredentialsFile`.
- When adding a new credential type with `omitempty`, `load()` must defensively re-init the field if older `credentials.json` files lack the key — otherwise `c.Embed.onUpdate = c.save` panics on nil. Mirror the `if c.Embed == nil { ... }` guard in credentials.go.

## GetDefault semantics

- `DbmsCredentials.GetDefault()` returns `(nil, nil)` when no default is set (not a usage error). Use `nil` check at the call site to decide whether to fall back to other connection resolution strategies. `EmbedCredentials.GetDefault()` follows the same convention.

## Sensitive fields

- `PrintableDbmsCredentials.AsArray()` / `MarshalJSON()` omit `password`; `PrintableEmbedCredentials.AsArray()` / `MarshalJSON()` omit `api-key`. Any future credential type with sensitive fields must follow this pattern.
- `PrintBodyMap` `fields` slice only affects TABLE rendering — it is ignored for JSON/toon formats. To suppress a sensitive field (e.g. `password`) from ALL output formats you must `delete(data, "field")` from the map before passing to `NewSingleValueResponseData`. The `fields` slice alone is insufficient.

## omitempty vs unconditional emit

- `DbmsCredential.EmbedCredential` uses `json:"embed-credential,omitempty"` so older creds without a link don't gain the key on disk. `PrintableDbmsCredentials.AsArray` / `MarshalJSON` always emit the key (empty string when unset) so the column is stable across rows for table rendering and external JSON consumers. Pattern: omitempty on the persisted struct, unconditional emit on the printable wrapper.

## Add() and resolveCredentialName

- `DbmsCredentials.Add` can only fail with "already exists". Since `resolveCredentialName` guarantees the resolved name is free, Add cannot fail in a single-threaded context after a successful `resolveCredentialName` call. Storage failure warning paths are effectively dead code in normal operation.

## Cross-type validation

- For commands like `credential dbms add --embed-credential x`: validate the target with `Embed.Get(name)` BEFORE calling `Dbms.Add(...)` so a bad name never half-creates a cred. Then call `Dbms.SetEmbed(name, embedName)` AFTER `Dbms.Add` succeeds — keeps the storage `Add` signature stable rather than threading new optional fields through.

## Test fixtures

- To verify DBMS credential storage in integration tests, use `helper.AssertCredentialsValue("dbms.credentials.0.name", "expected-name")`. To pre-populate DBMS credentials for collision tests, use `helper.SetCredentialsValue("dbms.credentials", []map[string]string{{...}})` + `helper.SetCredentialsValue("dbms.default-credential", "name")`.
- The default test helper credentials JSON only has `"aura": {...}` (no `"dbms"` key). During `Credentials.load()`, the absence of `"dbms"` in JSON leaves the field at its initial value `&DbmsCredentials{Credentials: []*DbmsCredential{}}`, so `cfg.Credentials.Dbms` is non-nil in all default test helpers. Passing `"dbms": null` in the JSON explicitly sets it to nil.
- When a dbms test exercises code that touches `cfg.Credentials.Embed`, seed the `embed` block in `newDbmsTestHelper` initial JSON; sjson.Set on `embed.credentials` from a partial fixture works but seeding upfront is more honest about the test surface.
