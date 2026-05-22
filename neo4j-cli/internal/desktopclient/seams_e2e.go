// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build e2e_desktop_seams

// Package desktopclient — seams_e2e.go is compiled ONLY under the
// `e2e_desktop_seams` build tag. It wires environment-variable overrides for
// the port probe, salt loader, data-dir resolver, and JWT-origin lookup so a
// CI job can drive the full flow against a local fixture server without
// installing the real Desktop app.
//
// Production binaries are built WITHOUT this tag, so the env vars below are
// inert in any artifact a user runs — the build-tag gate is the strong
// guarantee that no runtime path can reach these overrides in a shipped
// binary.
//
// Env vars honored on init (only when -tags e2e_desktop_seams):
//
//   - NEO4J_CLI_DESKTOP_E2E_PORT         → ProbePort returns it verbatim
//     (no scan, no /api-docs validation). Honors a decimal integer; a
//     non-integer or zero value is treated as "unset" and the production
//     scan still runs.
//   - NEO4J_CLI_DESKTOP_E2E_HTTP_ORIGIN  → replaces the
//     `http://<probeHost>:<port>` origin used both for API URL construction
//     in client.do and for the JWT signing key in signToken. Must match the
//     value the fixture is told to sign with.
//   - NEO4J_CLI_DESKTOP_E2E_SALT         → LoadSalt returns it verbatim
//     instead of reading `<dataDir>/relate.secret.key`. Must match the
//     fixture's `--salt` value.
//   - NEO4J_CLI_DESKTOP_E2E_DATA_DIR     → ResolveDataDir returns it
//     verbatim. Skips the NEO4J_DESKTOP_DATA_PATH env var, the env JSON
//     walk, and the per-OS default.
package desktopclient

import (
	"os"
	"strconv"
)

func init() {
	if v := os.Getenv("NEO4J_CLI_DESKTOP_E2E_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			e2ePortOverride = p
		}
	}
	if v := os.Getenv("NEO4J_CLI_DESKTOP_E2E_HTTP_ORIGIN"); v != "" {
		e2eOriginOverride = v
	}
	if v := os.Getenv("NEO4J_CLI_DESKTOP_E2E_SALT"); v != "" {
		e2eSaltOverride = v
	}
	if v := os.Getenv("NEO4J_CLI_DESKTOP_E2E_DATA_DIR"); v != "" {
		e2eDataDirOverride = v
	}
}
