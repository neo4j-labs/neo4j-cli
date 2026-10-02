// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build e2e_seams

// Package update — seams_e2e.go is compiled ONLY under the `e2e_seams` build
// tag. It wires environment-variable overrides for the URL bases and the
// HTTPS pin so a CI job can drive the full download → verify → swap flow
// against a local fixture server (test/e2e/update_fixture) without touching
// real GitHub release assets.
//
// Production binaries are built WITHOUT this tag, so the env vars below are
// inert in any artifact a user runs. The build-tag gate is the strong
// guarantee — there is no runtime path that can reach these overrides in a
// shipped binary.
//
// Env vars honored on init (only when -tags e2e_seams):
//
//   - NEO4J_CLI_UPDATE_E2E_API_URL    → overrides apiBaseURL
//   - NEO4J_CLI_UPDATE_E2E_DL_URL     → overrides dlBaseURL
//   - NEO4J_CLI_UPDATE_E2E_ALLOW_HTTP → if non-empty, drops requireHTTPS and
//     allows 127.0.0.1 / localhost to be the download host. The fixture
//     server uses an httptest.Server (HTTP only), so plain-HTTP loopback is
//     a hard requirement for the e2e harness.
//
// See `swap.go` near the `requireHTTPS` declaration for the production
// guard and AGENTS.md "Windows CI gotchas" for the rename-to-`.old` dance
// this harness exercises on Windows runners.
package update

import "os"

func init() {
	if v := os.Getenv("NEO4J_CLI_UPDATE_E2E_API_URL"); v != "" {
		apiBaseURL = v
	}
	if v := os.Getenv("NEO4J_CLI_UPDATE_E2E_DL_URL"); v != "" {
		dlBaseURL = v
	}
	if os.Getenv("NEO4J_CLI_UPDATE_E2E_ALLOW_HTTP") != "" {
		requireHTTPS = false
		allowedDownloadHosts["127.0.0.1"] = struct{}{}
		allowedDownloadHosts["localhost"] = struct{}{}
	}
}
