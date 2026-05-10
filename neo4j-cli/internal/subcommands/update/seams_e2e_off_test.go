// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build !e2e_seams

// seams_e2e_off_test.go asserts that a default build (no -tags e2e_seams)
// does NOT honor the NEO4J_CLI_UPDATE_E2E_* env vars. The acceptance is
// REQ-driven (task-017): production `make build` artifacts must not be
// trickable into talking to a non-GitHub host via env-var overrides.
//
// The test sets all three env vars to bogus values and asserts that the
// package-level seams still hold their production defaults. Because the
// init() function in seams_e2e.go is gated by the e2e_seams build tag, it
// is NOT compiled into this binary, so the env vars are inert.

package update

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestE2ESeams_NotCompiledByDefault(t *testing.T) {
	t.Setenv("NEO4J_CLI_UPDATE_E2E_API_URL", "http://evil.example.com")
	t.Setenv("NEO4J_CLI_UPDATE_E2E_DL_URL", "http://evil.example.com")
	t.Setenv("NEO4J_CLI_UPDATE_E2E_ALLOW_HTTP", "1")

	// Force a re-read of package state by referencing the seams. Their values
	// are baked at process start (init order), and since seams_e2e.go's init()
	// is excluded from this build, the env vars above can't have moved them.
	assert.Equal(t, "https://api.github.com", apiBaseURL,
		"apiBaseURL must stay at the production default in a non-e2e build")
	assert.Equal(t, "https://github.com", dlBaseURL,
		"dlBaseURL must stay at the production default in a non-e2e build")
	assert.True(t, requireHTTPS,
		"requireHTTPS must stay true in a non-e2e build")
	_, hasLoopback := allowedDownloadHosts["127.0.0.1"]
	assert.False(t, hasLoopback,
		"127.0.0.1 must NOT be in the download allowlist in a non-e2e build")
	_, hasLocalhost := allowedDownloadHosts["localhost"]
	assert.False(t, hasLocalhost,
		"localhost must NOT be in the download allowlist in a non-e2e build")
}
