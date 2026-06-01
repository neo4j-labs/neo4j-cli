// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clierr"
)

// defaultHuggingFaceBaseURL is the public HuggingFace serverless inference
// router. Resolve does not substitute this default; the provider applies it at
// request time when Config.BaseURL is empty so a stored credential pointing at
// a dedicated endpoint overrides cleanly.
//
// When Config.BaseURL equals this default the provider is in "serverless mode"
// and POSTs to {BaseURL}/{Model}/pipeline/feature-extraction; when overridden
// the provider is in "dedicated endpoint mode" and POSTs to {BaseURL} verbatim
// (the model and pipeline are already encoded in the dedicated endpoint URL).
// The /pipeline/feature-extraction suffix is required for sentence-transformers
// models whose default task on HF's router is sentence-similarity — without it
// the router routes to the wrong pipeline and rejects {"inputs": text}.
const defaultHuggingFaceBaseURL = "https://router.huggingface.co/hf-inference/models"

// huggingFaceProvider implements Provider against the HuggingFace inference
// API. Supports both the serverless router (default) and dedicated endpoints
// (any custom BaseURL). The HTTP client has no client-side timeout —
// cancellation is owned by the caller's ctx.
type huggingFaceProvider struct {
	cfg    Config
	client *http.Client
}

// newHuggingFaceProvider constructs a Provider. APIKey is validated lazily in
// Embed so a programmatic caller can construct a Config without a key for
// inspection; the production factory (New) goes straight to Embed.
func newHuggingFaceProvider(cfg Config) *huggingFaceProvider {
	return &huggingFaceProvider{
		cfg:    cfg,
		client: &http.Client{},
	}
}

// huggingFaceEmbedRequest mirrors the HuggingFace inference request body.
// The serverless router and dedicated endpoints both accept the same `inputs`
// shape for embedding models.
type huggingFaceEmbedRequest struct {
	Inputs string `json:"inputs"`
}

// Embed posts a single text/model pair and returns the embedding as
// []float32. URL selection: serverless mode (default BaseURL) posts to
// {BaseURL}/{Model}; dedicated mode (custom BaseURL) posts to {BaseURL}
// verbatim. Response shape is tolerated as either [[floats]] (serverless) or
// [floats] (dedicated). Errors are wrapped with the provider name; the
// Authorization header value never appears in any error text.
func (p *huggingFaceProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if p.cfg.APIKey == "" {
		return nil, clierr.NewAuthError(
			"missing API key for huggingface: set HF_TOKEN, NEO4J_EMBED_API_KEY, or store one with `neo4j-cli credential embed add`").
			WithSuggestion("store an embedding API key with `neo4j-cli credential embed add`")
	}

	base := p.cfg.BaseURL
	if base == "" {
		base = defaultHuggingFaceBaseURL
	}

	// Serverless mode: append {model}/pipeline/feature-extraction to the base
	// URL. Dedicated mode: post to the base URL verbatim (the dedicated
	// endpoint URL already encodes the model and pipeline).
	url := base
	if base == defaultHuggingFaceBaseURL {
		url = base + "/" + p.cfg.Model + "/pipeline/feature-extraction"
	}

	body := huggingFaceEmbedRequest{Inputs: text}

	headers := map[string]string{
		"Authorization": "Bearer " + p.cfg.APIKey,
	}
	raw, err := doJSONRequest(ctx, p.client, ProviderHuggingFace, http.MethodPost, url, body, headers, p.cfg.UserAgent)
	if err != nil {
		return nil, err
	}

	// Tolerate [[floats]] (serverless) and [floats] (dedicated) — a single
	// decode pass picks the right shape based on the first non-whitespace
	// byte that follows the outer `[`.
	var nested [][]float32
	if err := json.Unmarshal(raw, &nested); err == nil {
		if len(nested) == 0 || len(nested[0]) == 0 {
			return nil, fmt.Errorf("huggingface: empty embedding in response")
		}
		return nested[0], nil
	}

	var flat []float32
	if err := json.Unmarshal(raw, &flat); err == nil {
		if len(flat) == 0 {
			return nil, fmt.Errorf("huggingface: empty embedding in response")
		}
		return flat, nil
	}

	return nil, fmt.Errorf("huggingface: decode response: unrecognised shape (expected [[floats]] or [floats])")
}
