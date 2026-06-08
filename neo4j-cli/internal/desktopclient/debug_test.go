// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clievents"
	"github.com/stretchr/testify/assert"
)

func TestDebugRequest_EmitsPrefixesHeadersAndRedactsBody(t *testing.T) {
	// The "credentials" key is not caught by RedactText's shape matching, so the
	// create leaf registers the password explicitly — mirror that here.
	clievents.RegisterSecretValue("s3cr3t")

	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)
	SetDebugForTest(t, true)

	header := http.Header{}
	header.Set(HeaderClientID, "client-1")
	header.Set("Content-Type", "application/json")

	DebugRequestForTest(http.MethodPost, "http://localhost:44222/fastify/api/desktop/dbmss", header,
		[]byte(`{"name":"x","credentials":"s3cr3t"}`))

	out := buf.String()
	assert.Contains(t, out, "[desktop-debug] > POST")
	assert.Contains(t, out, "/fastify/api/desktop/dbmss")
	assert.Contains(t, out, "Content-Type: application/json")
	assert.NotContains(t, out, "s3cr3t")
	assert.Contains(t, out, "***")
}

func TestDebugRequest_RedactsPasswordBody(t *testing.T) {
	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)
	SetDebugForTest(t, true)

	DebugRequestForTest(http.MethodPost, "http://localhost:44222/fastify/api/connections", http.Header{},
		[]byte(`{"name":"c","password":"hunter2"}`))

	out := buf.String()
	assert.NotContains(t, out, "hunter2")
	assert.Contains(t, out, "***")
}

func TestDebugRequest_RedactsRegisteredTokenInHeader(t *testing.T) {
	const token = "eyJhbGciOiJIUzI1NiJ9.fake-jwt-token-value.signature"
	clievents.RegisterSecretValue(token)

	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)
	SetDebugForTest(t, true)

	header := http.Header{}
	header.Set(HeaderAPIToken, token)

	DebugRequestForTest(http.MethodGet, "http://localhost:44222/fastify/api/dbmss", header, nil)

	out := buf.String()
	assert.NotContains(t, out, token)
	// http.Header.Set canonicalises X-API-Token to X-Api-Token.
	assert.Contains(t, out, "X-Api-Token:")
	assert.Contains(t, out, "***")
}

func TestDebugResponse_EmitsStatusBodyAndElapsed(t *testing.T) {
	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)
	SetDebugForTest(t, true)

	DebugResponseForTest(http.StatusOK, http.Header{"Content-Type": []string{"application/json"}},
		[]byte(`{"id":"abc"}`), 12*time.Millisecond)

	out := buf.String()
	assert.Contains(t, out, "[desktop-debug] < 200")
	assert.Contains(t, out, `{"id":"abc"}`)
	assert.Contains(t, out, "elapsed")
}

func TestDebugInfo_StripsControlBytes(t *testing.T) {
	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)
	SetDebugForTest(t, true)

	DebugInfoForTest("probe \x1b[31mGET\x07 %s", "http://localhost:44222/fastify/api-docs")

	out := buf.String()
	assert.Contains(t, out, "[desktop-debug] ")
	assert.NotContains(t, out, "\x1b")
	assert.NotContains(t, out, "\x07")
	assert.Contains(t, out, "/fastify/api-docs")
}

func TestDebugHelpers_OffPathEmitNothing(t *testing.T) {
	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)
	SetDebugForTest(t, false)

	DebugRequestForTest(http.MethodGet, "http://localhost:44222/fastify/api/dbmss",
		http.Header{HeaderAPIToken: []string{"tok"}}, []byte(`{"password":"x"}`))
	DebugResponseForTest(http.StatusOK, http.Header{}, []byte(`{"id":"a"}`), time.Second)
	DebugInfoForTest("probe %s", "http://localhost:44222")

	assert.Empty(t, buf.String())
	assert.False(t, strings.Contains(buf.String(), "[desktop-debug]"))
}
