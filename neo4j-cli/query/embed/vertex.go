// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/neo4j/cli/common/clierr"
)

// vertexLocationPattern matches a GCP region name (e.g. us-central1,
// europe-west1, northamerica-northeast2). Single source of truth for both
// credential-add-time validation and Embed-time validation — must stay
// strict enough to reject any value that could escape the
// `{location}-aiplatform.googleapis.com` host suffix during URL composition
// (see SSRF/token-exfil notes in CLI-193 review).
var vertexLocationPattern = regexp.MustCompile(`^[a-z]+(-[a-z0-9]+)+$`)

// vertexProjectPattern matches a GCP project ID (6-30 chars, starts with a
// letter, ends alphanumeric, lower-case letters / digits / hyphens only).
var vertexProjectPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)

// ValidateVertexLocation rejects empty or malformed GCP region values.
// Empty input yields a missing-style usage error pointing at the recovery
// command; non-empty values that don't match a GCP region shape yield an
// invalid-format usage error. Single source of truth for both
// credential-add-time validation and Embed-time validation.
func ValidateVertexLocation(location string) error {
	if location == "" {
		return clierr.NewUsageError(
			"missing --vertex-location: set vertex location with `neo4j-cli credential embed add --vertex-project <project> --vertex-location <location>`")
	}
	if !vertexLocationPattern.MatchString(location) {
		return clierr.NewUsageError(
			"invalid --vertex-location %q: must be a GCP region like us-central1 (lower-case letters and digits, hyphen-separated)",
			location)
	}
	return nil
}

// ValidateVertexProject rejects empty or malformed GCP project ID values.
// Empty input yields a missing-style usage error pointing at the recovery
// command; non-empty values that don't match the GCP project ID shape
// yield an invalid-format usage error.
func ValidateVertexProject(project string) error {
	if project == "" {
		return clierr.NewUsageError(
			"missing --vertex-project: set vertex project with `neo4j-cli credential embed add --vertex-project <project> --vertex-location <location>`")
	}
	if !vertexProjectPattern.MatchString(project) {
		return clierr.NewUsageError(
			"invalid --vertex-project %q: must be a GCP project ID like my-project-123 (6-30 chars, starts with a letter, ends alphanumeric)",
			project)
	}
	return nil
}

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
	if err := ValidateVertexProject(p.cfg.VertexProject); err != nil {
		return nil, err
	}
	if err := ValidateVertexLocation(p.cfg.VertexLocation); err != nil {
		return nil, err
	}

	const authSuggestion = "authenticate with `gcloud auth application-default login`, then store project/location with `neo4j-cli credential embed add`"

	ts, err := p.tokenSource(ctx)
	if err != nil {
		return nil, clierr.NewAuthError(
			"vertex: Application Default Credentials not found: %v; run `gcloud auth application-default login` or set GOOGLE_APPLICATION_CREDENTIALS to a service-account JSON file",
			err).
			WithSuggestion(authSuggestion)
	}
	tok, err := ts.Token()
	if err != nil {
		return nil, clierr.NewAuthError(
			"vertex: failed to obtain OAuth token: %v; run `gcloud auth application-default login` or set GOOGLE_APPLICATION_CREDENTIALS to a service-account JSON file",
			err).
			WithSuggestion(authSuggestion)
	}

	reqURL := vertexURLHostPrefix(p.cfg.VertexLocation) +
		"/v1/projects/" + p.cfg.VertexProject +
		"/locations/" + p.cfg.VertexLocation +
		"/publishers/google/models/" + url.PathEscape(p.cfg.Model) +
		":predict"

	body := vertexEmbedRequest{
		Instances: []vertexInstance{{Content: text, TaskType: "RETRIEVAL_QUERY"}},
	}
	if p.cfg.Dimensions > 0 {
		body.Parameters = &vertexParameters{OutputDimensionality: p.cfg.Dimensions}
	}

	headers := map[string]string{
		"Authorization": "Bearer " + tok.AccessToken,
	}
	raw, err := doJSONRequest(ctx, p.client, ProviderVertex, http.MethodPost, reqURL, body, headers, p.cfg.UserAgent)
	if err != nil {
		return nil, err
	}

	var decoded vertexEmbedResponse
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&decoded); err != nil {
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
