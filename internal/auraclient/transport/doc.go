// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package transport is the HTTP implementation behind the auraclient service
// layer: it authenticates (OAuth client credentials, with a token cache), builds
// and sends requests to the Aura API, maps HTTP statuses to *clierr.CLIError exit
// codes, pages and polls, and traces requests under --debug with secrets
// redacted.
//
// Only auraclient imports this package. The one exception is the `aura api`
// passthrough command, which sends arbitrary requests (MakeRawRequest) to
// endpoints the CLI has no model of. Every other command goes through
// auraclient, and an architecture test fails the build if one imports this
// package.
//
// This package is the swap point for the Aura SDK: replacing it means
// re-implementing the auraclient interfaces, not changing any command.
package transport
