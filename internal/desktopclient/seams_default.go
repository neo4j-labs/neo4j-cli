// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build !e2e_desktop_seams

// Package desktopclient — seams_default.go is compiled into every NON-e2e
// build. It exists as a no-op counterpart to seams_e2e.go: without the
// `e2e_desktop_seams` build tag, the env-var-driven overrides for ProbePort /
// LoadSalt / ResolveDataDir / origin are NOT wired, so the production scan +
// disk read + env walk run unchanged.
//
// The package-level override variables (e2ePortOverride, e2eOriginOverride,
// e2eSaltOverride, e2eDataDirOverride) themselves live in discovery.go and
// are always compiled in — only the init() that assigns to them from env
// vars is build-tag-gated.
package desktopclient
