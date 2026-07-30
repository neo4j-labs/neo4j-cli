// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
)

// RawRequestConfig describes an arbitrary Aura API request. VersionPath is the
// literal first path segment ("v1", "v2beta1", "v9alpha3", …) used verbatim with
// no enum lookup, so an unreleased API version is reachable without a CLI
// release; "" omits the version segment entirely. Body is sent as-is, so any
// JSON shape including a top-level array is expressible.
type RawRequestConfig struct {
	Method      string
	VersionPath string
	Path        string
	Body        []byte
	QueryParams url.Values
	// Headers are overlaid on the generated auth headers, so a caller may
	// deliberately override e.g. Accept or Content-Type.
	Headers http.Header
	// WarnW receives keyring-failure warnings produced during token acquisition.
	// If nil, warnings are written to os.Stderr.
	WarnW io.Writer
}

// RawResponse is the unparsed result of a MakeRawRequest call. Body is
// populated for every status code, including 4xx and 5xx, so the caller can
// surface the upstream error body instead of discarding it.
type RawResponse struct {
	StatusCode int
	Status     string
	Proto      string
	Header     http.Header
	Body       []byte
}

// MakeRawRequest issues an arbitrary authenticated request against the Aura API,
// sharing MakeRequest's credential resolution, SSRF gating, token cache, and
// --debug tracing through prepareRequest.
//
// It diverges from MakeRequest on three points, each load-bearing for a
// passthrough pointed at pre-GA endpoints: handleResponseError is never called
// (it panics on statuses the v2beta1 spec documents), a 2xx body carrying
// `errors[]` stays a success (7 spec operations return that shape), and nothing
// panics — transport and body-read failures come back as *CLIError. A non-2xx
// status is therefore not an error here; map it with RawStatusError.
//
// HTTP 401 is the one status handled inline: the stale access token is cleared
// through formatAuthorizationError so the next invocation mints a fresh one, and
// that auth error is returned alongside the response.
func MakeRawRequest(cfg *clicfg.Config, config *RawRequestConfig) (*RawResponse, error) {
	if config.Method == "" {
		return nil, clierr.NewUsageError("http method not set for aura api request")
	}

	req, credential, err := prepareRequest(cfg, config)
	if err != nil {
		return nil, err
	}

	client := http.Client{Timeout: httpClientTimeout}
	start := time.Now()

	res, err := client.Do(req)
	if err != nil {
		// Transport errors embed the full request URL, which may carry
		// user-supplied query params, so scrub before the text reaches stdout.
		return nil, clierr.NewUpstreamError("aura api request failed: %s", scrub(err.Error()))
	}
	defer res.Body.Close() //nolint:errcheck // response body close error is not actionable in a defer

	body, err := io.ReadAll(res.Body)
	if err != nil {
		// A truncated response is exactly the case a trace is wanted for, so
		// report the status reached before bailing.
		if cfg.Aura.Debug() {
			debugInfo("response body read failed after status %d: %s", res.StatusCode, err.Error())
		}
		return nil, clierr.NewUpstreamError("could not read aura api response body: %s", scrub(err.Error()))
	}

	if cfg.Aura.Debug() {
		debugResponse(res.StatusCode, res.Header, body, time.Since(start))
	}

	raw := &RawResponse{
		StatusCode: res.StatusCode,
		Status:     res.Status,
		Proto:      res.Proto,
		Header:     res.Header,
		Body:       body,
	}

	if res.StatusCode == http.StatusUnauthorized {
		return raw, formatAuthorizationError(body, res.StatusCode, credential, cfg)
	}

	return raw, nil
}

// rawErrorBodyLimit bounds how many bytes of the upstream body are folded into
// the returned error message. clierr.Render puts that message in a JSON envelope
// on stdout, so an unbounded body (an HTML error page, a spec document) would
// swamp it.
const rawErrorBodyLimit = 4096

// RawStatusError maps a RawResponse's HTTP status onto the same clierr codes
// handleResponseError uses, and returns nil for any 2xx (including 201/202/204).
//
// Unlike handleResponseError it parses no response schema and never panics.
// Statuses the v2beta1 spec documents but the CLI never modelled (405, 413, 415,
// 422, …) fall back to an upstream error rather than a panic, and the upstream
// body is folded into the message verbatim rather than being read through the
// fixed api.Error shape — most 4xx responses on the newer endpoints declare no
// body schema at all. That fallback marks those permanent client errors
// retryable in the envelope, which is deliberate: with no schema to read, this
// mapper cannot tell a permanent rejection from a transient upstream fault, and
// over-reporting retryable is the safer half of that trade.
//
// The body goes into the error, never to stdout: clierr.Render already writes a
// JSON error envelope there, so echoing it too would put two documents on stdout.
func RawStatusError(res *RawResponse) error {
	if res == nil {
		return clierr.NewUpstreamError("aura api request produced no response")
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}

	detail := rawErrorDetail(res)

	switch res.StatusCode {
	case http.StatusBadRequest:
		return clierr.NewValidationError("%s", detail)
	case http.StatusUnauthorized, http.StatusForbidden:
		return clierr.NewAuthError("%s", detail).WithSuggestion(authSuggestion)
	case http.StatusNotFound:
		return clierr.NewNotFoundError("%s", detail)
	case http.StatusPaymentRequired, http.StatusConflict:
		return clierr.NewConflictError("%s", detail)
	case http.StatusTooManyRequests:
		retryAfter := res.Header.Get("Retry-After")
		err := clierr.NewRateLimitError(retryAfter, "%s", detail)
		if retryAfter != "" {
			return err.WithSuggestion(fmt.Sprintf("Retry after %s seconds.", retryAfter))
		}
		return err
	}

	// 5xx plus every unmapped status (3xx, 405, 413, 415, 422, …).
	return clierr.NewUpstreamError("%s", detail)
}

// rawErrorDetail renders the status line and the upstream body as one message
// line. Both halves pass through scrub (RedactText then StripControl) because
// both are upstream-controlled: the reason phrase is not filtered by net/http,
// and the body echoes back whatever was submitted, which may include a secret.
func rawErrorDetail(res *RawResponse) string {
	status := strings.TrimSpace(scrub(res.Status))
	if status == "" {
		status = strings.TrimSpace(fmt.Sprintf("%d %s", res.StatusCode, http.StatusText(res.StatusCode)))
	}

	// Pre-bound before scrubbing so a multi-megabyte error page doesn't pay for
	// several regex passes to produce a few KB of message. The generous outer
	// limit keeps redaction safe: a secret split by the outer cut is dropped by
	// the final one.
	body := strings.TrimSpace(scrub(truncateBytes(string(res.Body), rawErrorBodyLimit*16)))
	if body == "" {
		return fmt.Sprintf("aura api request failed with status %s", status)
	}
	return fmt.Sprintf("aura api request failed with status %s: %s", status, truncateBytes(body, rawErrorBodyLimit))
}

// truncateBytes cuts s to at most limit bytes without splitting a UTF-8 rune,
// marking the result so a reader knows the upstream body continued.
func truncateBytes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "... (truncated)"
}
