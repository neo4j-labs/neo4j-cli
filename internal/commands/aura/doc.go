// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package aura is the `neo4j-cli aura` command tree: one directory per resource
// (instance, agent, graphql, ...), one file per action.
//
// A command parses its flags into a scope and a small request, calls the Aura
// client — auraclient.New(cfg).<Resource>().<Operation>(...) — and prints the
// record it returns with the output package (PrintRecord / PrintRecords). It
// does not talk HTTP and does not import auraclient/transport; the one exception
// is the `aura api` passthrough (see architecture_test.go).
//
// Alongside the commands: flags (reusable flag types), output (rendering),
// utils (org/project resolution) and testutils (the AuraTestHelper that drives a
// command against a mock Aura server).
package aura
