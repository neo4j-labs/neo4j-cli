// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoJSONRequest_HappyPathBodyCap asserts the helper caps a successful
// response body read at maxResponseBodyBytes so a compromised proxy or
// attacker-controlled BaseURL returning a multi-GB 200 OK cannot exhaust
// memory. Writing >cap bytes that aren't valid JSON forces a downstream
// decode failure (the truncation point lands inside a run of zero bytes);
// the read itself is bounded.
func TestDoJSONRequest_HappyPathBodyCap(t *testing.T) {
	const oversize = maxResponseBodyBytes + (1 << 20) // cap + 1 MiB

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Stream zero bytes well past the cap. If the helper were unbounded
		// the test process would buffer the full payload into memory; with
		// the cap in place it stops at maxResponseBodyBytes and the decode
		// below fails fast.
		buf := make([]byte, 64<<10)
		written := 0
		for written < oversize {
			n, err := w.Write(buf)
			if err != nil {
				return
			}
			written += n
		}
	}))
	defer srv.Close()

	raw, err := doJSONRequest(
		context.Background(),
		srv.Client(),
		"test",
		http.MethodPost,
		srv.URL,
		map[string]any{"k": "v"},
		nil,
		"neo4j-cli/test",
	)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(raw), maxResponseBodyBytes, "happy-path read must not exceed the cap")
	assert.Equal(t, maxResponseBodyBytes, len(raw), "expected exactly cap bytes when server writes past it")

	// The truncated payload isn't valid JSON, so the caller's decode step
	// surfaces a decode error rather than an OOM — verify that downstream
	// failure mode here.
	var out struct{}
	decodeErr := json.NewDecoder(strings.NewReader(string(raw))).Decode(&out)
	require.Error(t, decodeErr)
}

// TestDoJSONRequest_HappyPathUnderCap confirms the cap is a ceiling, not a
// floor: a normal-sized response is read in full.
func TestDoJSONRequest_HappyPathUnderCap(t *testing.T) {
	body := []byte(`{"ok":true}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	raw, err := doJSONRequest(
		context.Background(),
		srv.Client(),
		"test",
		http.MethodPost,
		srv.URL,
		map[string]any{"k": "v"},
		nil,
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, body, raw)
}
