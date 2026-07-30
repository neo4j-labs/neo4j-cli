// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"io"
	"net/http"
	"net/url"
	"time"

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
