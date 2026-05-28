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
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/neo4j/cli/common/clicfg/urlcheck"
	"github.com/neo4j/cli/common/clierr"
)

// vertexScope is the OAuth2 scope required to call the Vertex AI predict
// endpoint.
const vertexScope = "https://www.googleapis.com/auth/cloud-platform"

// findDefaultTokenSource resolves Application Default Credentials and returns
// an oauth2.TokenSource scoped for Vertex AI. Indirected as a package-level
// variable so tests can stub it without touching real ADC.
var findDefaultTokenSource = func(ctx context.Context) (oauth2.TokenSource, error) {
	creds, err := google.FindDefaultCredentials(ctx, vertexScope)
	if err != nil {
		return nil, err
	}
	return creds.TokenSource, nil
}

// vertexURLHostPrefix builds the canonical Vertex AI host prefix. Indirected
// as a package-level variable so tests can substitute an httptest.Server URL
// without exercising real DNS / TLS.
var vertexURLHostPrefix = func(location string) string {
	return "https://" + location + "-aiplatform.googleapis.com"
}

// vertexProvider implements Provider against the Vertex AI prediction API.
// The HTTP client is constructed once per provider with no client-side timeout
// — cancellation is owned by the caller's ctx. The OAuth token source is
// resolved lazily on the first Embed call so a programmatic caller can
// construct a Config without GCP creds for inspection.
type vertexProvider struct {
	cfg    Config
	client *http.Client

	tokenOnce sync.Once
	tokenSrc  oauth2.TokenSource
	tokenErr  error
}

// newVertexProvider constructs a Provider. Required fields (VertexProject,
// VertexLocation) are validated lazily in Embed so the factory in New can
// return without surfacing a usage error to programmatic callers.
func newVertexProvider(cfg Config) *vertexProvider {
	return &vertexProvider{
		cfg:    cfg,
		client: &http.Client{},
	}
}

// vertexEmbedRequest mirrors the Vertex AI :predict request body. `task_type`
// is snake_case (Vertex convention) — distinct from Gemini's camelCase
// `taskType`. Parameters is a pointer so omitempty drops the field entirely
// when Dimensions is 0.
type vertexEmbedRequest struct {
	Instances  []vertexInstance  `json:"instances"`
	Parameters *vertexParameters `json:"parameters,omitempty"`
}

type vertexInstance struct {
	Content  string `json:"content"`
	TaskType string `json:"task_type"`
}

type vertexParameters struct {
	OutputDimensionality int `json:"outputDimensionality"`
}

// vertexEmbedResponse mirrors the Vertex AI :predict response shape we care
// about. Fields not consumed are omitted to keep the decode permissive for
// forward-compat additions.
type vertexEmbedResponse struct {
	Predictions []struct {
		Embeddings struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	} `json:"predictions"`
}

// Embed posts a single text/model pair to the Vertex AI :predict endpoint and
// returns predictions[0].embeddings.values as []float32. cfg.BaseURL is
// intentionally ignored — Vertex's URL is location-derived. The OAuth Bearer
// token is fetched lazily from Application Default Credentials; the token
// value never appears in any error text.
func (p *vertexProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if p.cfg.VertexProject == "" {
		return nil, clierr.NewUsageError(
			"missing vertex project: set --vertex-project or store one with `neo4j-cli credential embed add --vertex-project <project> --vertex-location <location>`")
	}
	if p.cfg.VertexLocation == "" {
		return nil, clierr.NewUsageError(
			"missing vertex location: set --vertex-location or store one with `neo4j-cli credential embed add --vertex-project <project> --vertex-location <location>`")
	}

	ts, err := p.tokenSource(ctx)
	if err != nil {
		return nil, clierr.NewAuthError(
			"vertex: Application Default Credentials not found: %v; run `gcloud auth application-default login` or set GOOGLE_APPLICATION_CREDENTIALS to a service-account JSON file",
			err)
	}
	tok, err := ts.Token()
	if err != nil {
		return nil, clierr.NewAuthError(
			"vertex: failed to obtain OAuth token: %v; run `gcloud auth application-default login` or set GOOGLE_APPLICATION_CREDENTIALS to a service-account JSON file",
			err)
	}

	url := vertexURLHostPrefix(p.cfg.VertexLocation) +
		"/v1/projects/" + p.cfg.VertexProject +
		"/locations/" + p.cfg.VertexLocation +
		"/publishers/google/models/" + p.cfg.Model +
		":predict"
	if err := urlcheck.ValidateRemoteURL(url); err != nil {
		return nil, fmt.Errorf("vertex: url rejected: %w", err)
	}

	body := vertexEmbedRequest{
		Instances: []vertexInstance{{Content: text, TaskType: "RETRIEVAL_QUERY"}},
	}
	if p.cfg.Dimensions > 0 {
		body.Parameters = &vertexParameters{OutputDimensionality: p.cfg.Dimensions}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("vertex: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("vertex: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if p.cfg.UserAgent != "" {
		req.Header.Set("User-Agent", p.cfg.UserAgent)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vertex: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close error is not actionable

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read up to 4KiB of the body for the error message — enough to
		// surface a JSON error envelope without spamming the terminal on
		// massive non-JSON HTML pages from misconfigured proxies. The
		// Authorization header lives on the request, never the response,
		// so echoing the body is safe.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("vertex: HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}

	var decoded vertexEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("vertex: decode response: %w", err)
	}
	if len(decoded.Predictions) == 0 || len(decoded.Predictions[0].Embeddings.Values) == 0 {
		return nil, fmt.Errorf("vertex: empty embedding in response")
	}
	return decoded.Predictions[0].Embeddings.Values, nil
}

// tokenSource memoises the ADC lookup across Embed calls. The first call
// resolves credentials via findDefaultTokenSource; subsequent calls reuse the
// cached source (or the cached error, surfaced as a fresh AuthError).
func (p *vertexProvider) tokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	p.tokenOnce.Do(func() {
		p.tokenSrc, p.tokenErr = findDefaultTokenSource(ctx)
	})
	return p.tokenSrc, p.tokenErr
}
