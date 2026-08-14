**Security review** (golang-security skill, review mode)

The PR fixes panics in `handleResponseError` (`neo4j-cli/aura/internal/api/response.go`) that previously escaped to `main.go`'s top-level recover with exit code 0. All changed Go files were traced for injection, information-disclosure, and secret-handling issues.

**Closed by this PR (security improvement):**
- The old `default:` panic branch interpolated `resBody` **unscrubbed** directly into the error message: `"unexpected status code %d and body %s …", statusCode, resBody`. A secret echoed back by the API (e.g. `{"password":"…"}`) could reach stdout. The new `upstreamDetail` → `scrubbedBodyTrunc` path applies `RedactText + StripControl` before embedding any body content in an error.
- `reportIssueFatal` embedded `clievents.RedactArgs(os.Args[1:])` in the error message (args redaction). Removing it and routing through the body-scrubbing helper narrows the attack surface for accidental secret disclosure.

**Paths reviewed and found clean:**
- `scrubbedBodyTrunc` refactored from inline code in `rawErrorDetail` — output is byte-identical; both truncate bounds and the `scrub` call are preserved in the same order.
- `upstreamDetail(statusCode, nil)` on `io.ReadAll` failure — `string(nil)` is `""` in Go, so `scrubbedBodyTrunc(nil)` returns `""` and the function emits the safe `"upstream error [status N] with no response body"` fallback.
- `errorMessages` replacing `extractEmbeddedErrors` in `api.go` — identical semantics when `withField=false`; the 2xx-with-embedded-errors path is unaffected.
- 402 double-unmarshal (`errorMessages` then `json.Unmarshal` for `suggestionForPaymentRequired`) — safe; the comment documents the intent.
- No new dependencies in `go.mod`/`go.sum`.

**Pre-existing (not introduced by this PR):**
- `serverError.Error` (HTTP 403 branch) and `e.Field`/`e.Message` from parsed `ErrorResponse.Errors` are placed into error messages without `scrubbedBodyTrunc`. This is unchanged from the prior code and outside the PR's scope; the parseable-body path is not what the `default:` panic closed.

No issues found.

**Security review:**  no issues found
