// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clierr"
)

// defaultOpenAIBaseURL is the public OpenAI v1 endpoint. Resolve does not
// substitute this default; the provider applies it at request time when
// Config.BaseURL is empty so a stored credential pointing at e.g. an
// Azure-OpenAI deployment overrides cleanly.
const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// openAIProvider implements Provider against the OpenAI embeddings API. The
// HTTP client is constructed once per provider with no client-side timeout —
// cancellation is owned by the caller's ctx.
type openAIProvider struct {
	cfg    Config
	client *http.Client
}

// newOpenAIProvider constructs a Provider. APIKey is validated lazily in
// Embed so a programmatic caller can construct a Config without a key for
// inspection; the production factory (New) goes straight to Embed.
func newOpenAIProvider(cfg Config) *openAIProvider {
	return &openAIProvider{
		cfg:    cfg,
		client: &http.Client{},
	}
}

// openAIEmbedRequest mirrors the OpenAI /embeddings request body. Dimensions
// is a pointer so omitempty drops the field when unset; OpenAI rejects an
// explicit `0` for models that do not honour `dimensions`.
type openAIEmbedRequest struct {
	Model      string `json:"model"`
	Input      string `json:"input"`
	Dimensions *int   `json:"dimensions,omitempty"`
}

// openAIEmbedResponse mirrors the OpenAI /embeddings response shape we care
// about. Fields not consumed (object, usage, model) are omitted to keep the
// decode permissive for forward-compat additions.
type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed posts a single text/model pair to {BaseURL}/embeddings and returns
// data[0].embedding as []float32. Errors are wrapped with the provider name
// so the caller's surface always identifies the upstream; the Authorization
// header value never appears in any error text.
func (p *openAIProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if p.cfg.APIKey == "" {
		return nil, clierr.NewAuthError("%s", missingAPIKeyMessage(ProviderOpenAI, p.cfg.AcceptEnvVars)).
			WithSuggestion(missingAPIKeySuggestion(p.cfg.AcceptEnvVars))
	}

	base := p.cfg.BaseURL
	if base == "" {
		base = defaultOpenAIBaseURL
	}

	body := openAIEmbedRequest{
		Model: p.cfg.Model,
		Input: text,
	}
	if p.cfg.Dimensions > 0 {
		d := p.cfg.Dimensions
		body.Dimensions = &d
	}

	headers := map[string]string{
		"Authorization": "Bearer " + p.cfg.APIKey,
	}
	raw, err := doJSONRequest(ctx, p.client, ProviderOpenAI, http.MethodPost, base+"/embeddings", body, headers, p.cfg.UserAgent)
	if err != nil {
		return nil, err
	}

	var decoded openAIEmbedResponse
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("openai: decode response: %w", err)
	}
	if len(decoded.Data) == 0 || len(decoded.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("openai: empty embedding in response")
	}
	return decoded.Data[0].Embedding, nil
}
