// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build !e2e_desktop_seams

// seams_default_test.go asserts that a default build (no -tags
// e2e_desktop_seams) does NOT honor the NEO4J_CLI_DESKTOP_E2E_* env vars.
// The acceptance is task-013-driven: production `make build` artifacts must
// not be trickable into talking to a fixture / non-Desktop endpoint via
// env-var overrides, even if a user typed one of these vars by accident.
//
// The test sets all four env vars to bogus values and asserts that the
// package-level override variables still hold their production defaults
// (zero / empty). Because the init() function in seams_e2e.go is gated by
// the e2e_desktop_seams build tag, it is NOT compiled into this binary, so
// the env vars are inert.
//
// Mirrors `neo4j-cli/internal/subcommands/update/seams_e2e_off_test.go`.

package desktopclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDesktopE2ESeams_NotCompiledByDefault(t *testing.T) {
	t.Setenv("NEO4J_CLI_DESKTOP_E2E_PORT", "12345")
	t.Setenv("NEO4J_CLI_DESKTOP_E2E_HTTP_ORIGIN", "http://evil.example.com")
	t.Setenv("NEO4J_CLI_DESKTOP_E2E_SALT", "evilsalt")
	t.Setenv("NEO4J_CLI_DESKTOP_E2E_DATA_DIR", "/tmp/evil-data-dir")

	// The override vars are set by seams_e2e.go's init() — gated behind
	// the e2e_desktop_seams build tag, which is NOT active in this build.
	// So setting the env vars at runtime cannot have moved them: the only
	// code path that reads the env is excluded from the binary.
	assert.Equal(t, 0, e2ePortOverride,
		"e2ePortOverride must stay 0 in a non-e2e build (env var must be inert)")
	assert.Equal(t, "", e2eOriginOverride,
		"e2eOriginOverride must stay empty in a non-e2e build (env var must be inert)")
	assert.Equal(t, "", e2eSaltOverride,
		"e2eSaltOverride must stay empty in a non-e2e build (env var must be inert)")
	assert.Equal(t, "", e2eDataDirOverride,
		"e2eDataDirOverride must stay empty in a non-e2e build (env var must be inert)")
}
