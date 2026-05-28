// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"github.com/neo4j/cli/common/clierr"
)

// defaultGeminiBaseURL is the public Google Generative Language API v1beta
// endpoint. Resolve does not substitute this default; the provider applies it
// at request time when Config.BaseURL is empty so a stored credential pointing
// at e.g. a regional or proxy endpoint overrides cleanly.
const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// geminiNativeDimensions is the gemini-embedding-001 native output size; the
// API returns already-normalized vectors at this size, so the provider only
// L2-normalizes when Dimensions is explicitly set to a non-default value.
const geminiNativeDimensions = 3072

// geminiProvider implements Provider against the Gemini embeddings API. The
// HTTP client is constructed once per provider with no client-side timeout —
// cancellation is owned by the caller's ctx.
type geminiProvider struct {
	cfg    Config
	client *http.Client
}

// newGeminiProvider constructs a Provider. APIKey is validated lazily in
// Embed so a programmatic caller can construct a Config without a key for
// inspection; the production factory (New) goes straight to Embed.
func newGeminiProvider(cfg Config) *geminiProvider {
	return &geminiProvider{
		cfg:    cfg,
		client: &http.Client{},
	}
}

// geminiEmbedRequest mirrors the Gemini :embedContent request body.
// OutputDimensionality is a pointer so omitempty drops the field when unset.
type geminiEmbedRequest struct {
	Content              geminiContent `json:"content"`
	TaskType             string        `json:"taskType"`
	OutputDimensionality *int          `json:"outputDimensionality,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

// geminiEmbedResponse mirrors the Gemini :embedContent response shape. Fields
// not consumed are omitted to keep the decode permissive for forward-compat
// additions.
type geminiEmbedResponse struct {
	Embedding struct {
		Values []float32 `json:"values"`
	} `json:"embedding"`
}

// Embed posts a single text/model pair to
// {BaseURL}/models/{Model}:embedContent and returns embedding.values as
// []float32. taskType is always sent as "RETRIEVAL_QUERY"; Gemini ignores
// unknown fields per the API spec. When cfg.Dimensions > 0 && != 3072 the
// returned vector is L2-normalized before return. Errors are wrapped with
// the provider name; the API key header value never appears in any error text.
func (p *geminiProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if p.cfg.APIKey == "" {
		return nil, clierr.NewAuthError(
			"missing API key for gemini: set GEMINI_API_KEY, GOOGLE_API_KEY, NEO4J_EMBED_API_KEY, or store one with `neo4j-cli credential embed add`")
	}

	base := p.cfg.BaseURL
	if base == "" {
		base = defaultGeminiBaseURL
	}

	body := geminiEmbedRequest{
		Content:  geminiContent{Parts: []geminiPart{{Text: text}}},
		TaskType: "RETRIEVAL_QUERY",
	}
	if p.cfg.Dimensions > 0 {
		d := p.cfg.Dimensions
		body.OutputDimensionality = &d
	}

	url := base + "/models/" + p.cfg.Model + ":embedContent"
	headers := map[string]string{
		"x-goog-api-key": p.cfg.APIKey,
	}
	raw, err := doJSONRequest(ctx, p.client, ProviderGemini, http.MethodPost, url, body, headers, p.cfg.UserAgent)
	if err != nil {
		return nil, err
	}

	var decoded geminiEmbedResponse
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("gemini: decode response: %w", err)
	}
	if len(decoded.Embedding.Values) == 0 {
		return nil, fmt.Errorf("gemini: empty embedding in response")
	}

	if p.cfg.Dimensions > 0 && p.cfg.Dimensions != geminiNativeDimensions {
		return l2Normalize(decoded.Embedding.Values), nil
	}
	return decoded.Embedding.Values, nil
}

// l2Normalize returns a unit vector for v. A zero-vector input is returned
// unchanged (division by zero is guarded) — the caller's downstream consumer
// gets the original vector rather than a NaN payload.
func l2Normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	norm := math.Sqrt(sum)
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / norm)
	}
	return out
}
