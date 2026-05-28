// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/neo4j/cli/common/clicfg/urlcheck"
)

// doJSONRequest centralises the JSON-POST boilerplate shared by every embed
// provider: URL validation, body marshalling, request construction, header
// setting, dispatch, status check (4KiB snippet on non-2xx, wrapped with
// `<provider>: HTTP <code>: <snippet>`), and response body read.
//
// The helper is auth-agnostic — provider-specific auth headers
// (Authorization Bearer, x-goog-api-key, etc.) are passed in via headers so
// no auth value can leak into errors produced here. URL validation happens
// before request construction so SSRF-rejection paths still short-circuit.
//
// Returns the response body bytes on success (caller decodes); returns an
// error already prefixed with the provider name on any failure stage.
func doJSONRequest(
	ctx context.Context,
	client *http.Client,
	provider string,
	method string,
	url string,
	body any,
	headers map[string]string,
	userAgent string,
) ([]byte, error) {
	if err := urlcheck.ValidateRemoteURL(url); err != nil {
		return nil, fmt.Errorf("%s: url rejected: %w", provider, err)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %w", provider, err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", provider, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed: %w", provider, err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close error is not actionable

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read up to 4KiB of the body for the error message — enough to
		// surface a JSON error envelope without spamming the terminal on
		// massive non-JSON HTML pages from misconfigured proxies. Auth
		// headers live on the request, never the response, so echoing the
		// body is safe.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%s: HTTP %d: %s", provider, resp.StatusCode, bytes.TrimSpace(snippet))
	}

	// Cap the happy-path read at 10 MiB — ~100x the largest legitimate
	// embedding response — to block a compromised proxy or attacker-controlled
	// BaseURL from exhausting memory via a multi-GB 200 OK body. At-or-over
	// the cap the JSON decode downstream fails with "unexpected EOF" rather
	// than OOMing the process.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", provider, err)
	}
	return raw, nil
}

// maxResponseBodyBytes caps how much of a successful response body we will
// buffer into memory. Set to 10 MiB; embedding responses are <100 KiB even
// at the full 3072 dims gemini-embedding-001 returns.
const maxResponseBodyBytes = 10 << 20
