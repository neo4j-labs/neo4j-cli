// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clierr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAI_Embed_HappyPath_NoDimensions(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer srv.Close()

	p := newOpenAIProvider(Config{
		Provider:  ProviderOpenAI,
		Model:     "text-embedding-3-small",
		BaseURL:   srv.URL,
		APIKey:    "sk-test",
		UserAgent: "neo4j-cli/vtest",
	})
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, got)

	assert.Equal(t, "/embeddings", gotPath)
	assert.Equal(t, "Bearer sk-test", gotAuth)
	assert.Equal(t, "neo4j-cli/vtest", gotUA)
	assert.Equal(t, "text-embedding-3-small", gotBody["model"])
	assert.Equal(t, "hello", gotBody["input"])
	_, hasDim := gotBody["dimensions"]
	assert.False(t, hasDim, "dimensions should be omitted when 0")
}

func TestOpenAI_Embed_HappyPath_WithDimensions(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.5]}]}`))
	}))
	defer srv.Close()

	p := newOpenAIProvider(Config{
		Provider:   ProviderOpenAI,
		Model:      "text-embedding-3-small",
		BaseURL:    srv.URL,
		APIKey:     "sk-test",
		Dimensions: 256,
	})
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.5}, got)

	assert.EqualValues(t, 256, gotBody["dimensions"])
}

func TestOpenAI_Embed_MissingKeyReturnsAuthError(t *testing.T) {
	// Server that would fail loudly if hit; missing key must short-circuit
	// before any HTTP traffic.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be hit when API key is missing")
	}))
	defer srv.Close()

	p := newOpenAIProvider(Config{
		Provider:      ProviderOpenAI,
		Model:         "text-embedding-3-small",
		BaseURL:       srv.URL,
		AcceptEnvVars: true,
		// No APIKey.
	})
	got, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "missing API key for openai")
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 4, ce.Code)
	assert.Contains(t, ce.Suggestion, "neo4j-cli credential embed add", "auth error signposts the credential-store command")
}

// TestOpenAI_Embed_MissingKeyMessageGateAware verifies the provider backstop
// obeys REQ-F-023: off-mode references a .env file / stored credential and does
// NOT advertise the OS key env vars as a direct fix; on-mode may name them.
func TestOpenAI_Embed_MissingKeyMessageGateAware(t *testing.T) {
	t.Run("off mode does not advertise the gated env var as a fix", func(t *testing.T) {
		p := newOpenAIProvider(Config{Provider: ProviderOpenAI, AcceptEnvVars: false})
		_, err := p.Embed(context.Background(), "hello")
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, "missing API key for openai")
		assert.Contains(t, msg, ".env")
		assert.Contains(t, msg, "credential embed add")
		assert.Contains(t, msg, "enable accept-env-vars")
		assert.NotContains(t, msg, "set OPENAI_API_KEY",
			"off-mode message must not advertise OPENAI_API_KEY as a direct fix")

		var ce *clierr.CLIError
		require.True(t, errors.As(err, &ce))
		assert.NotContains(t, ce.Suggestion, "via an env var",
			"off-mode suggestion must not advertise an env var as a direct fix")
	})

	t.Run("on mode may name the env vars directly", func(t *testing.T) {
		p := newOpenAIProvider(Config{Provider: ProviderOpenAI, AcceptEnvVars: true})
		_, err := p.Embed(context.Background(), "hello")
		require.Error(t, err)
		msg := err.Error()
		assert.Contains(t, msg, "OPENAI_API_KEY")
		assert.Contains(t, msg, "NEO4J_EMBED_API_KEY")
	})
}

func TestOpenAI_Embed_Non2xxWrapsStatusNoAuthLeak(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key"}}`))
	}))
	defer srv.Close()

	p := newOpenAIProvider(Config{
		Provider: ProviderOpenAI,
		Model:    "text-embedding-3-small",
		BaseURL:  srv.URL,
		APIKey:   "sk-secret-do-not-leak",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "openai")
	assert.Contains(t, err.Error(), "401")
	assert.Contains(t, err.Error(), "Incorrect API key")
	assert.NotContains(t, err.Error(), "sk-secret-do-not-leak")
	assert.NotContains(t, err.Error(), "Bearer")
}

func TestOpenAI_Embed_MalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[`)) // truncated JSON
	}))
	defer srv.Close()

	p := newOpenAIProvider(Config{
		Provider: ProviderOpenAI,
		Model:    "m",
		BaseURL:  srv.URL,
		APIKey:   "sk-test",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "openai")
	assert.Contains(t, strings.ToLower(err.Error()), "decode")
}

func TestOpenAI_Embed_EmptyDataArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	p := newOpenAIProvider(Config{
		Provider: ProviderOpenAI,
		Model:    "m",
		BaseURL:  srv.URL,
		APIKey:   "sk-test",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty embedding")
}

func TestOpenAI_Embed_CtxCancellationAborts(t *testing.T) {
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

	p := newOpenAIProvider(Config{
		Provider: ProviderOpenAI,
		Model:    "m",
		BaseURL:  srv.URL,
		APIKey:   "sk-test",
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

// TestOpenAI_Embed_RejectsBlockedBaseURL asserts that an SSRF-blocked base
// URL fails before any HTTP traffic. The provider's transport would panic if
// hit (we use captureRoundTripper-style checks via t.Fatal in the handler).
func TestOpenAI_Embed_RejectsBlockedBaseURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		baseURL string
	}{
		{name: "metadata IP", baseURL: "http://169.254.169.254/v1"},
		{name: "private RFC1918", baseURL: "http://10.0.0.1/v1"},
		{name: "cleartext non-loopback", baseURL: "http://api.openai.com/v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newOpenAIProvider(Config{
				Provider: ProviderOpenAI,
				Model:    "m",
				BaseURL:  tc.baseURL,
				APIKey:   "sk-test",
			})
			p.client = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				t.Fatalf("transport must not be hit for blocked URL")
				return nil, nil
			})}

			_, err := p.Embed(context.Background(), "hello")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "openai")
			assert.Contains(t, err.Error(), "rejected")
		})
	}
}

// roundTripperFunc adapts a func to http.RoundTripper for tests that must
// assert no HTTP traffic was issued.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenAI_Embed_DefaultBaseURL(t *testing.T) {
	// Construct provider with no BaseURL set; assert the default is used at
	// request time. We swap http.Client.Transport so we never hit a real
	// network.
	captured := captureRoundTripper{}
	p := newOpenAIProvider(Config{
		Provider: ProviderOpenAI,
		Model:    "m",
		APIKey:   "sk-test",
	})
	p.client = &http.Client{Transport: &captured}

	_, _ = p.Embed(context.Background(), "hello") // err expected (no real response)
	require.NotNil(t, captured.req)
	assert.Equal(t, "https://api.openai.com/v1/embeddings", captured.req.URL.String())
}

// captureRoundTripper records the outbound request and returns a stub error
// so the embed call short-circuits without performing real I/O.
type captureRoundTripper struct {
	req *http.Request
}

func (c *captureRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	c.req = r
	return nil, errStubTransport
}

var errStubTransport = stubErr("stub transport: no real response")

type stubErr string

func (e stubErr) Error() string { return string(e) }
