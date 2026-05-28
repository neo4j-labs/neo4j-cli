// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGemini_Embed_HappyPath_NoDimensions(t *testing.T) {
	var gotPath string
	var gotKey string
	var gotAuth string
	var gotUA string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-goog-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embedding":{"values":[0.1,0.2,0.3]}}`))
	}))
	defer srv.Close()

	p := newGeminiProvider(Config{
		Provider:  ProviderGemini,
		Model:     "gemini-embedding-001",
		BaseURL:   srv.URL,
		APIKey:    "gk-test",
		UserAgent: "neo4j-cli/vtest",
	})
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, got)

	assert.Equal(t, "/models/gemini-embedding-001:embedContent", gotPath)
	assert.Equal(t, "gk-test", gotKey)
	assert.Empty(t, gotAuth, "must not send Authorization: Bearer")
	assert.Equal(t, "neo4j-cli/vtest", gotUA)

	// taskType always sent
	assert.Equal(t, "RETRIEVAL_QUERY", gotBody["taskType"])
	// outputDimensionality omitted when 0
	_, hasDim := gotBody["outputDimensionality"]
	assert.False(t, hasDim, "outputDimensionality should be omitted when 0")

	// content shape
	content, ok := gotBody["content"].(map[string]any)
	require.True(t, ok, "content should be an object")
	parts, ok := content["parts"].([]any)
	require.True(t, ok, "content.parts should be an array")
	require.Len(t, parts, 1)
	first, _ := parts[0].(map[string]any)
	assert.Equal(t, "hello", first["text"])
}

func TestGemini_Embed_WithDimensions_SendsField(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		// Native-dim 3072 path: response returned verbatim (no normalize).
		_, _ = w.Write([]byte(`{"embedding":{"values":[0.5,0.5]}}`))
	}))
	defer srv.Close()

	p := newGeminiProvider(Config{
		Provider:   ProviderGemini,
		Model:      "gemini-embedding-001",
		BaseURL:    srv.URL,
		APIKey:     "gk-test",
		Dimensions: 3072,
	})
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.5, 0.5}, got, "no normalization when dims == 3072")
	assert.EqualValues(t, 3072, gotBody["outputDimensionality"])
}

func TestGemini_Embed_Normalizes_WhenDimsNotNative(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Vector with length sqrt(0.09 + 0.16) = 0.5; after normalize → (0.6, 0.8).
		_, _ = w.Write([]byte(`{"embedding":{"values":[0.3,0.4]}}`))
	}))
	defer srv.Close()

	p := newGeminiProvider(Config{
		Provider:   ProviderGemini,
		Model:      "gemini-embedding-001",
		BaseURL:    srv.URL,
		APIKey:     "gk-test",
		Dimensions: 768,
	})
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.InDelta(t, 0.6, got[0], 1e-6)
	assert.InDelta(t, 0.8, got[1], 1e-6)
	// Length should be 1.
	length := math.Sqrt(float64(got[0])*float64(got[0]) + float64(got[1])*float64(got[1]))
	assert.InDelta(t, 1.0, length, 1e-6)
}

func TestGemini_Embed_NormalizeZeroVectorPassesThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embedding":{"values":[0,0,0]}}`))
	}))
	defer srv.Close()

	p := newGeminiProvider(Config{
		Provider:   ProviderGemini,
		Model:      "m",
		BaseURL:    srv.URL,
		APIKey:     "gk-test",
		Dimensions: 768,
	})
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0, 0, 0}, got)
}

func TestGemini_Embed_MissingKeyReturnsAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be hit when API key is missing")
	}))
	defer srv.Close()

	p := newGeminiProvider(Config{
		Provider: ProviderGemini,
		Model:    "gemini-embedding-001",
		BaseURL:  srv.URL,
	})
	got, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "missing API key for gemini")
	assert.Contains(t, err.Error(), "GEMINI_API_KEY")
	assert.Contains(t, err.Error(), "GOOGLE_API_KEY")
	assert.Contains(t, err.Error(), "NEO4J_EMBED_API_KEY")
	assert.Contains(t, err.Error(), "credential embed add")

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 4, ce.Code)
}

func TestGemini_Embed_Non2xxWrapsStatusNoKeyLeak(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"API key not valid"}}`))
	}))
	defer srv.Close()

	p := newGeminiProvider(Config{
		Provider: ProviderGemini,
		Model:    "gemini-embedding-001",
		BaseURL:  srv.URL,
		APIKey:   "gk-secret-do-not-leak",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gemini")
	assert.Contains(t, err.Error(), "403")
	assert.Contains(t, err.Error(), "API key not valid")
	assert.NotContains(t, err.Error(), "gk-secret-do-not-leak")
}

func TestGemini_Embed_EmptyValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embedding":{"values":[]}}`))
	}))
	defer srv.Close()

	p := newGeminiProvider(Config{
		Provider: ProviderGemini,
		Model:    "m",
		BaseURL:  srv.URL,
		APIKey:   "gk-test",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty embedding")
}

func TestGemini_Embed_MalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embedding":{"values":[`)) // truncated JSON
	}))
	defer srv.Close()

	p := newGeminiProvider(Config{
		Provider: ProviderGemini,
		Model:    "m",
		BaseURL:  srv.URL,
		APIKey:   "gk-test",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gemini")
	assert.Contains(t, strings.ToLower(err.Error()), "decode")
}

func TestGemini_Embed_RejectsBlockedBaseURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		baseURL string
	}{
		{name: "metadata IP", baseURL: "http://169.254.169.254/v1beta"},
		{name: "private RFC1918", baseURL: "http://10.0.0.1/v1beta"},
		{name: "cleartext non-loopback", baseURL: "http://generativelanguage.googleapis.com/v1beta"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newGeminiProvider(Config{
				Provider: ProviderGemini,
				Model:    "m",
				BaseURL:  tc.baseURL,
				APIKey:   "gk-test",
			})
			p.client = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				t.Fatalf("transport must not be hit for blocked URL")
				return nil, nil
			})}

			_, err := p.Embed(context.Background(), "hello")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "gemini")
			assert.Contains(t, err.Error(), "rejected")
		})
	}
}

func TestGemini_Embed_DefaultBaseURL(t *testing.T) {
	captured := captureRoundTripper{}
	p := newGeminiProvider(Config{
		Provider: ProviderGemini,
		Model:    "gemini-embedding-001",
		APIKey:   "gk-test",
	})
	p.client = &http.Client{Transport: &captured}

	_, _ = p.Embed(context.Background(), "hello") // err expected (no real response)
	require.NotNil(t, captured.req)
	assert.Equal(t,
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:embedContent",
		captured.req.URL.String())
}

func TestGemini_New_ReturnsGeminiProvider(t *testing.T) {
	p, err := New(Config{Provider: ProviderGemini, Model: "gemini-embedding-001"})
	require.NoError(t, err)
	_, ok := p.(*geminiProvider)
	assert.True(t, ok, "New(ProviderGemini) should yield *geminiProvider")
}
