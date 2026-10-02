// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllama_Embed_HappyPath(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotUA string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2,0.3]]}`))
	}))
	defer srv.Close()

	p := newOllamaProvider(Config{
		Provider:  ProviderOllama,
		Model:     "nomic-embed-text",
		BaseURL:   srv.URL,
		UserAgent: "neo4j-cli/vtest",
	})
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, got)

	assert.Equal(t, "/api/embed", gotPath)
	assert.Equal(t, "", gotAuth, "Authorization header should never be sent for Ollama")
	assert.Equal(t, "neo4j-cli/vtest", gotUA)
	assert.Equal(t, "nomic-embed-text", gotBody["model"])
	assert.Equal(t, "hello", gotBody["input"])
	_, hasDim := gotBody["dimensions"]
	assert.False(t, hasDim, "dimensions should never be sent to Ollama")
}

func TestOllama_Embed_DimensionsIgnored(t *testing.T) {
	// Even when Config.Dimensions is set, Ollama provider must not include
	// the field in the request body (Ollama doesn't honour it).
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[[0.5]]}`))
	}))
	defer srv.Close()

	p := newOllamaProvider(Config{
		Provider:   ProviderOllama,
		Model:      "nomic-embed-text",
		BaseURL:    srv.URL,
		Dimensions: 768,
	})
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.5}, got)

	_, hasDim := gotBody["dimensions"]
	assert.False(t, hasDim, "dimensions must never be sent to Ollama regardless of Config.Dimensions")
}

func TestOllama_Embed_NoAuthHeaderEvenWithAPIKey(t *testing.T) {
	// Ollama provider must never send an Authorization header — even when
	// Config.APIKey is populated (e.g. when sharing creds with an OpenAI
	// stored credential).
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[[0.1]]}`))
	}))
	defer srv.Close()

	p := newOllamaProvider(Config{
		Provider: ProviderOllama,
		Model:    "nomic-embed-text",
		BaseURL:  srv.URL,
		APIKey:   "should-be-ignored",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Empty(t, gotAuth)
}

func TestOllama_Embed_Non2xxWrapsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer srv.Close()

	p := newOllamaProvider(Config{
		Provider: ProviderOllama,
		Model:    "missing-model",
		BaseURL:  srv.URL,
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ollama")
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "model not found")
}

func TestOllama_Embed_MalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[`)) // truncated JSON
	}))
	defer srv.Close()

	p := newOllamaProvider(Config{
		Provider: ProviderOllama,
		Model:    "m",
		BaseURL:  srv.URL,
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ollama")
	assert.Contains(t, strings.ToLower(err.Error()), "decode")
}

func TestOllama_Embed_EmptyEmbeddingsArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[]}`))
	}))
	defer srv.Close()

	p := newOllamaProvider(Config{
		Provider: ProviderOllama,
		Model:    "m",
		BaseURL:  srv.URL,
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty embedding")
}

func TestOllama_Embed_CtxCancellationAborts(t *testing.T) {
	// Server that blocks until the test cancels ctx, then writes a response
	// the client will never see. The handler signals via started so we know
	// the request reached the server before we cancel.
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		// Server-side ctx propagation from a closed client conn is best-effort;
		// add a hard cap so a flaky propagation does not hang the handler (and
		// the whole test binary) if the client-side cancellation does not reach
		// us. The test asserts on the client-side error, not the server timing.
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	p := newOllamaProvider(Config{
		Provider: ProviderOllama,
		Model:    "m",
		BaseURL:  srv.URL,
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := p.Embed(ctx, "hello")
		errCh <- err
	}()
	<-started
	cancel()

	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "context")
	case <-time.After(5 * time.Second):
		t.Fatal("Embed did not return after ctx cancel")
	}
}

// TestOllama_Embed_RejectsBlockedBaseURL asserts that an SSRF-blocked base
// URL fails before any HTTP traffic. The default localhost endpoint stays
// permitted (covered by TestOllama_Embed_DefaultBaseURL).
func TestOllama_Embed_RejectsBlockedBaseURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		baseURL string
	}{
		{name: "metadata IP", baseURL: "http://169.254.169.254"},
		{name: "private RFC1918", baseURL: "http://10.0.0.1:11434"},
		{name: "cleartext non-loopback", baseURL: "http://prod.example.com:11434"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newOllamaProvider(Config{
				Provider: ProviderOllama,
				Model:    "m",
				BaseURL:  tc.baseURL,
			})
			p.client = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				t.Fatalf("transport must not be hit for blocked URL")
				return nil, nil
			})}

			_, err := p.Embed(context.Background(), "hello")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ollama")
			assert.Contains(t, err.Error(), "rejected")
		})
	}
}

func TestOllama_Embed_DefaultBaseURL(t *testing.T) {
	// Construct provider with no BaseURL set; assert the default is used at
	// request time. We swap http.Client.Transport so we never hit a real
	// network.
	captured := captureRoundTripper{}
	p := newOllamaProvider(Config{
		Provider: ProviderOllama,
		Model:    "m",
	})
	p.client = &http.Client{Transport: &captured}

	_, _ = p.Embed(context.Background(), "hello") // err expected (no real response)
	require.NotNil(t, captured.req)
	assert.Equal(t, "http://localhost:11434/api/embed", captured.req.URL.String())
}
