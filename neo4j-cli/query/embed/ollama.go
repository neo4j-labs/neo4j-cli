// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// defaultOllamaBaseURL is the conventional local Ollama endpoint. Resolve does
// not substitute this default; the provider applies it at request time when
// Config.BaseURL is empty so a stored credential pointing at a remote Ollama
// host overrides cleanly.
const defaultOllamaBaseURL = "http://localhost:11434"

// ollamaProvider implements Provider against the Ollama /api/embed API.
// Ollama does not require an API key and ignores the OpenAI-style
// `dimensions` field, so neither is sent. The HTTP client has no client-side
// timeout — cancellation is owned by the caller's ctx.
type ollamaProvider struct {
	cfg    Config
	client *http.Client
}

// newOllamaProvider constructs a Provider. There is no API-key validation:
// Ollama accepts unauthenticated requests by default and any key set on the
// Config is intentionally ignored (no Authorization header is sent).
func newOllamaProvider(cfg Config) *ollamaProvider {
	return &ollamaProvider{
		cfg:    cfg,
		client: &http.Client{},
	}
}

// ollamaEmbedRequest mirrors the Ollama /api/embed request body. There is no
// `dimensions` field by design — Ollama's models advertise a fixed embedding
// size and silently ignore the OpenAI-style dimensions parameter.
type ollamaEmbedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// ollamaEmbedResponse mirrors the Ollama /api/embed response shape we care
// about. Fields not consumed (model, total_duration, etc.) are omitted to
// keep the decode permissive for forward-compat additions.
type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed posts a single text/model pair to {BaseURL}/api/embed and returns
// embeddings[0] as []float32. Errors are wrapped with the provider name so
// the caller's surface always identifies the upstream.
func (p *ollamaProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	base := p.cfg.BaseURL
	if base == "" {
		base = defaultOllamaBaseURL
	}

	body := ollamaEmbedRequest{
		Model: p.cfg.Model,
		Input: text,
	}

	raw, err := doJSONRequest(ctx, p.client, ProviderOllama, http.MethodPost, base+"/api/embed", body, nil, p.cfg.UserAgent)
	if err != nil {
		return nil, err
	}

	var decoded ollamaEmbedResponse
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("ollama: decode response: %w", err)
	}
	if len(decoded.Embeddings) == 0 || len(decoded.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("ollama: empty embedding in response")
	}
	return decoded.Embeddings[0], nil
}
