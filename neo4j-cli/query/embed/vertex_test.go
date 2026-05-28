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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/neo4j/cli/common/clierr"
)

// withTokenSource replaces findDefaultTokenSource for the duration of the
// returned restore func. Tests use a static token source so no real ADC
// lookup ever runs.
func withTokenSource(t *testing.T, ts oauth2.TokenSource, err error) {
	t.Helper()
	prev := findDefaultTokenSource
	findDefaultTokenSource = func(context.Context) (oauth2.TokenSource, error) {
		return ts, err
	}
	t.Cleanup(func() { findDefaultTokenSource = prev })
}

// withURLHostPrefix overrides the canonical Vertex host so the provider
// targets an httptest.Server. The override is undone via t.Cleanup.
func withURLHostPrefix(t *testing.T, prefix string) {
	t.Helper()
	prev := vertexURLHostPrefix
	vertexURLHostPrefix = func(string) string { return prefix }
	t.Cleanup(func() { vertexURLHostPrefix = prev })
}

func staticTokenSource(token string) oauth2.TokenSource {
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
}

func TestVertex_Embed_MissingProject(t *testing.T) {
	p := newVertexProvider(Config{
		Provider:       ProviderVertex,
		Model:          "gemini-embedding-001",
		VertexLocation: "us-central1",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vertex project")
	assert.Contains(t, err.Error(), "--vertex-project")
	assert.Contains(t, err.Error(), "credential embed add")

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code, "missing vertex project is a usage error")
}

func TestVertex_Embed_MissingLocation(t *testing.T) {
	p := newVertexProvider(Config{
		Provider:      ProviderVertex,
		Model:         "gemini-embedding-001",
		VertexProject: "my-project",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vertex location")
	assert.Contains(t, err.Error(), "--vertex-location")

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code, "missing vertex location is a usage error")
}

func TestVertex_Embed_MissingADC(t *testing.T) {
	withTokenSource(t, nil, errors.New("could not find default credentials"))

	p := newVertexProvider(Config{
		Provider:       ProviderVertex,
		Model:          "gemini-embedding-001",
		VertexProject:  "my-project",
		VertexLocation: "us-central1",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vertex")
	assert.Contains(t, err.Error(), "Application Default Credentials")
	assert.Contains(t, err.Error(), "gcloud auth application-default login")
	assert.Contains(t, err.Error(), "GOOGLE_APPLICATION_CREDENTIALS")

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 4, ce.Code, "ADC failure is an auth error")
}

func TestVertex_Embed_HappyPath_NoDimensions(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"predictions":[{"embeddings":{"values":[0.1,0.2,0.3]}}]}`))
	}))
	defer srv.Close()

	withTokenSource(t, staticTokenSource("tok-test"), nil)
	withURLHostPrefix(t, srv.URL)

	p := newVertexProvider(Config{
		Provider:       ProviderVertex,
		Model:          "gemini-embedding-001",
		VertexProject:  "my-project",
		VertexLocation: "us-central1",
		UserAgent:      "neo4j-cli/vtest",
	})
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, got)

	assert.Equal(t,
		"/v1/projects/my-project/locations/us-central1/publishers/google/models/gemini-embedding-001:predict",
		gotPath)
	assert.Equal(t, "Bearer tok-test", gotAuth)
	assert.Equal(t, "neo4j-cli/vtest", gotUA)

	// task_type is snake_case (Vertex convention) — distinct from Gemini.
	instances, ok := gotBody["instances"].([]any)
	require.True(t, ok, "instances should be an array")
	require.Len(t, instances, 1)
	first, _ := instances[0].(map[string]any)
	assert.Equal(t, "hello", first["content"])
	assert.Equal(t, "RETRIEVAL_QUERY", first["task_type"])

	// parameters omitted entirely when Dimensions == 0.
	_, hasParams := gotBody["parameters"]
	assert.False(t, hasParams, "parameters should be omitted when Dimensions == 0")
}

func TestVertex_Embed_DimensionsSendParameters(t *testing.T) {
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"predictions":[{"embeddings":{"values":[0.3,0.4]}}]}`))
	}))
	defer srv.Close()

	withTokenSource(t, staticTokenSource("tok-test"), nil)
	withURLHostPrefix(t, srv.URL)

	p := newVertexProvider(Config{
		Provider:       ProviderVertex,
		Model:          "gemini-embedding-001",
		VertexProject:  "my-project",
		VertexLocation: "us-central1",
		Dimensions:     768,
	})
	got, err := p.Embed(context.Background(), "hello")
	require.NoError(t, err)
	// No L2-normalization on vertex — vector returned verbatim.
	assert.Equal(t, []float32{0.3, 0.4}, got)

	params, ok := gotBody["parameters"].(map[string]any)
	require.True(t, ok, "parameters should be present when Dimensions > 0")
	assert.EqualValues(t, 768, params["outputDimensionality"])
}

func TestVertex_Embed_Non2xxWrapsStatusNoTokenLeak(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"Permission denied"}}`))
	}))
	defer srv.Close()

	withTokenSource(t, staticTokenSource("tok-secret-do-not-leak"), nil)
	withURLHostPrefix(t, srv.URL)

	p := newVertexProvider(Config{
		Provider:       ProviderVertex,
		Model:          "gemini-embedding-001",
		VertexProject:  "my-project",
		VertexLocation: "us-central1",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vertex")
	assert.Contains(t, err.Error(), "403")
	assert.Contains(t, err.Error(), "Permission denied")
	assert.NotContains(t, err.Error(), "tok-secret-do-not-leak")
}

func TestVertex_Embed_EmptyPredictions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"predictions":[]}`))
	}))
	defer srv.Close()

	withTokenSource(t, staticTokenSource("tok-test"), nil)
	withURLHostPrefix(t, srv.URL)

	p := newVertexProvider(Config{
		Provider:       ProviderVertex,
		Model:          "m",
		VertexProject:  "p",
		VertexLocation: "us-central1",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty embedding")
}

func TestVertex_Embed_MalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"predictions":[{"embeddings":{"values":[`)) // truncated JSON
	}))
	defer srv.Close()

	withTokenSource(t, staticTokenSource("tok-test"), nil)
	withURLHostPrefix(t, srv.URL)

	p := newVertexProvider(Config{
		Provider:       ProviderVertex,
		Model:          "m",
		VertexProject:  "p",
		VertexLocation: "us-central1",
	})
	_, err := p.Embed(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vertex")
	assert.Contains(t, strings.ToLower(err.Error()), "decode")
}

func TestVertex_Embed_DefaultURLPrefix(t *testing.T) {
	withTokenSource(t, staticTokenSource("tok-test"), nil)

	captured := captureRoundTripper{}
	p := newVertexProvider(Config{
		Provider:       ProviderVertex,
		Model:          "gemini-embedding-001",
		VertexProject:  "my-project",
		VertexLocation: "us-central1",
	})
	p.client = &http.Client{Transport: &captured}

	_, _ = p.Embed(context.Background(), "hello") // err expected (no real response)
	require.NotNil(t, captured.req)
	assert.Equal(t,
		"https://us-central1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-central1/publishers/google/models/gemini-embedding-001:predict",
		captured.req.URL.String())
}

func TestVertex_Embed_BaseURLIgnored(t *testing.T) {
	// Even when cfg.BaseURL is set, the provider builds its URL from
	// project/location and does NOT honour BaseURL.
	withTokenSource(t, staticTokenSource("tok-test"), nil)

	captured := captureRoundTripper{}
	p := newVertexProvider(Config{
		Provider:       ProviderVertex,
		Model:          "gemini-embedding-001",
		VertexProject:  "my-project",
		VertexLocation: "us-central1",
		BaseURL:        "https://example.com/should-be-ignored",
	})
	p.client = &http.Client{Transport: &captured}

	_, _ = p.Embed(context.Background(), "hello")
	require.NotNil(t, captured.req)
	assert.NotContains(t, captured.req.URL.String(), "example.com")
	assert.Contains(t, captured.req.URL.String(), "us-central1-aiplatform.googleapis.com")
}

func TestVertex_New_ReturnsVertexProvider(t *testing.T) {
	p, err := New(Config{Provider: ProviderVertex, Model: "gemini-embedding-001"})
	require.NoError(t, err)
	_, ok := p.(*vertexProvider)
	assert.True(t, ok, "New(ProviderVertex) should yield *vertexProvider")
}
