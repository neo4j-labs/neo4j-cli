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

func TestHuggingFace_Embed_ServerlessMode_HappyPath_NestedShape(t *testing.T) {
	// Serverless mode: BaseURL == defaultHuggingFaceBaseURL → POST {BaseURL}/{Model}
	// with the [[floats]] response shape used by most embedding models.
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
		_, _ = w.Write([]byte(`[[0.1,0.2,0.3]]`))
	}))
	defer srv.Close()

	// Use a captureRoundTripper to redirect the default-BaseURL request to the
	// test server while keeping the URL-construction logic in the provider
	// honest about "serverless mode".
	captured := serverlessRedirectTransport{srvURL: srv.URL}
	p := newHuggingFaceProvider(Config{
		Provider:  ProviderHuggingFace,
		Model:     "sentence-transformers/all-MiniLM-L6-v2",
		APIKey:    "hf_test",
		UserAgent: "neo4j-cli/vtest",
	})
	p.client = &http.Client{Transport: &captured}

	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, got)

	// Path on the test server reflects {default-base-host-stripped}/{Model}.
	assert.Equal(t, "/hf-inference/models/sentence-transformers/all-MiniLM-L6-v2", gotPath)
	assert.Equal(t, "Bearer hf_test", gotAuth)
	assert.Equal(t, "neo4j-cli/vtest", gotUA)
	assert.Equal(t, "hello", gotBody["inputs"])
}

func TestHuggingFace_Embed_DedicatedMode_HappyPath_FlatShape(t *testing.T) {
	// Dedicated mode: BaseURL != default → POST {BaseURL} verbatim, [floats]
	// response shape used by some dedicated endpoints.
	var gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[0.4,0.5,0.6]`))
	}))
	defer srv.Close()

	p := newHuggingFaceProvider(Config{
		Provider: ProviderHuggingFace,
		Model:    "ignored-by-dedicated-endpoint",
		BaseURL:  srv.URL,
		APIKey:   "hf_test",
	})
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.4, 0.5, 0.6}, got)

	// Dedicated mode posts to BaseURL verbatim — model is NOT appended.
	assert.Equal(t, "/", gotPath)
	assert.Equal(t, "hello", gotBody["inputs"])
}

func TestHuggingFace_Embed_DedicatedMode_NestedShapeAlsoTolerated(t *testing.T) {
	// Some dedicated endpoints still return [[floats]]; the provider must
	// tolerate either shape regardless of mode.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[[0.7,0.8]]`))
	}))
	defer srv.Close()

	p := newHuggingFaceProvider(Config{
		Provider: ProviderHuggingFace,
		Model:    "m",
		BaseURL:  srv.URL,
		APIKey:   "hf_test",
	})
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.7, 0.8}, got)
}

func TestHuggingFace_Embed_ServerlessMode_FlatShapeAlsoTolerated(t *testing.T) {
	// Mirror of the above: serverless mode tolerating flat shape, in case a
	// future router model returns [floats] directly.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[0.9,1.0]`))
	}))
	defer srv.Close()

	captured := serverlessRedirectTransport{srvURL: srv.URL}
	p := newHuggingFaceProvider(Config{
		Provider: ProviderHuggingFace,
		Model:    "m",
		APIKey:   "hf_test",
	})
	p.client = &http.Client{Transport: &captured}

	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.9, 1.0}, got)
}

func TestHuggingFace_Embed_MissingKeyReturnsUsageError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be hit when API key is missing")
	}))
	defer srv.Close()

	p := newHuggingFaceProvider(Config{
		Provider: ProviderHuggingFace,
		Model:    "m",
		BaseURL:  srv.URL,
		// No APIKey.
	})
	got, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "missing API key for huggingface")
	assert.Contains(t, err.Error(), "HF_TOKEN")
}

func TestHuggingFace_Embed_Non2xxWrapsStatusNoAuthLeak(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Invalid credentials"}`))
	}))
	defer srv.Close()

	p := newHuggingFaceProvider(Config{
		Provider: ProviderHuggingFace,
		Model:    "m",
		BaseURL:  srv.URL,
		APIKey:   "hf_secret_do_not_leak",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "huggingface")
	assert.Contains(t, err.Error(), "401")
	assert.Contains(t, err.Error(), "Invalid credentials")
	assert.NotContains(t, err.Error(), "hf_secret_do_not_leak")
	assert.NotContains(t, err.Error(), "Bearer")
}

func TestHuggingFace_Embed_MalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"unexpected": "shape"}`)) // neither nested nor flat
	}))
	defer srv.Close()

	p := newHuggingFaceProvider(Config{
		Provider: ProviderHuggingFace,
		Model:    "m",
		BaseURL:  srv.URL,
		APIKey:   "hf_test",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "huggingface")
	assert.Contains(t, strings.ToLower(err.Error()), "shape")
}

func TestHuggingFace_Embed_EmptyNestedArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[[]]`))
	}))
	defer srv.Close()

	p := newHuggingFaceProvider(Config{
		Provider: ProviderHuggingFace,
		Model:    "m",
		BaseURL:  srv.URL,
		APIKey:   "hf_test",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty embedding")
}

func TestHuggingFace_Embed_EmptyFlatArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	p := newHuggingFaceProvider(Config{
		Provider: ProviderHuggingFace,
		Model:    "m",
		BaseURL:  srv.URL,
		APIKey:   "hf_test",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty embedding")
}

func TestHuggingFace_Embed_CtxCancellationAborts(t *testing.T) {
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

	p := newHuggingFaceProvider(Config{
		Provider: ProviderHuggingFace,
		Model:    "m",
		BaseURL:  srv.URL,
		APIKey:   "hf_test",
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

func TestHuggingFace_Embed_DefaultBaseURL_AppendsModelToServerlessBase(t *testing.T) {
	// Construct provider with no BaseURL set; assert the default is used at
	// request time AND the model is appended (serverless mode). We swap
	// http.Client.Transport so we never hit a real network.
	captured := captureRoundTripper{}
	p := newHuggingFaceProvider(Config{
		Provider: ProviderHuggingFace,
		Model:    "BAAI/bge-large-en-v1.5",
		APIKey:   "hf_test",
	})
	p.client = &http.Client{Transport: &captured}

	_, _ = p.Embed(context.Background(), "hello") // err expected (no real response)
	require.NotNil(t, captured.req)
	assert.Equal(t,
		"https://router.huggingface.co/hf-inference/models/BAAI/bge-large-en-v1.5",
		captured.req.URL.String(),
	)
}

// serverlessRedirectTransport rewrites the default HuggingFace serverless URL
// to a httptest server so we can exercise the "default BaseURL appends model"
// path against a real handler. The path is preserved (host/scheme swapped) so
// the test can still assert on the constructed request path.
type serverlessRedirectTransport struct {
	srvURL string
}

func (s *serverlessRedirectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// Replace the default scheme+host with the test server's. The path —
	// which encodes /hf-inference/models/<Model> — survives unchanged so the
	// handler can assert on it.
	if !strings.HasPrefix(r.URL.String(), defaultHuggingFaceBaseURL) {
		return nil, errStubTransport
	}
	rewritten := s.srvURL + strings.TrimPrefix(r.URL.String(), "https://router.huggingface.co")
	rebuilt, err := http.NewRequestWithContext(r.Context(), r.Method, rewritten, r.Body)
	if err != nil {
		return nil, err
	}
	rebuilt.Header = r.Header.Clone()
	return http.DefaultTransport.RoundTrip(rebuilt)
}
