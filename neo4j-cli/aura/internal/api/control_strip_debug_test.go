// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebug_StripsControlBytesPreservesWhitespace(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/instances", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Body field with ESC, BEL, NUL, DEL plus benign \t \n \r. net/http
		// rejects control bytes in header values, so the control-strip surface
		// is exercised via the response body (writeHeaders shares scrub()).
		w.Write([]byte("{\"name\":\"a\x1b[31mb\x07c\x00d\x7fe\tf\ng\rh\"}")) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	var buf bytes.Buffer
	api.SetDebugWriterForTest(t, &buf)

	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)
	cfg.Aura.SetDebug(true)

	_, status, err := api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	out := buf.String()

	// Control bytes neutralised to "?".
	assert.NotContains(t, out, "\x1b", "ESC must be stripped")
	assert.NotContains(t, out, "\x07", "BEL must be stripped")
	assert.NotContains(t, out, "\x00", "NUL must be stripped")
	assert.NotContains(t, out, "\x7f", "DEL must be stripped")
	assert.Contains(t, out, "a?", "ESC replaced with ?")

	// Benign whitespace preserved (the body field still carries \t\n\r).
	assert.Contains(t, out, "e\tf\ng\rh")
}
