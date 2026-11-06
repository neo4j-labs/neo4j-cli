// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package relate provides access to local Relate graph stores — Relate is the
// embedded openCypher engine (formerly CypherLite) — for the `query --relate`
// backend and the `relate` command tree.
//
// The engine itself is cgo: the typed C ABI (librelate_ffi, ABI v2) wrapped by
// github.com/relatedb/relate/clients/go. Importing that client requires the
// Relate native library at BUILD time, so the implementation lives in
// engine.go behind the `relate` build tag (plus CGO_ENABLED=1). Everything in
// this file compiles in every build: OpenStore is nil in an untagged build and
// callers surface NotCompiledError, so flags and subcommands still parse and
// fail closed with a typed message in every build (the same shape Relate's own
// CLI uses for feature bundles).
package relate

import (
	"path/filepath"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
)

// QueryType is a statement's plan-time classification as the engine reports
// it: a read (r), a write with no returned columns (w), a write with returned
// columns (rw), or a schema statement (s). EXPLAIN and PROFILE report the
// wrapped statement's type.
type QueryType string

const (
	QueryTypeRead      QueryType = "r"
	QueryTypeWrite     QueryType = "w"
	QueryTypeReadWrite QueryType = "rw"
	QueryTypeSchema    QueryType = "s"
)

// StatementResult is one statement's outcome: the columns and rows (values
// already converted to the CLI's cell model — dbtype-compatible graph
// entities and containers, ISO-formatted temporals are NOT applied here; the
// query package's coercion layer owns display conversion), the classification
// the --rw guard inspects, the bookmark when the statement committed
// something, and the EXPLAIN/PROFILE plan tree when the statement carried one.
type StatementResult struct {
	Columns   []string
	Rows      [][]any
	QueryType QueryType
	Bookmark  string
	Plan      *Plan
}

// Plan is the driver-free snapshot of a statement's EXPLAIN or PROFILE plan
// tree, in the engine's own shape (relate.plan/v1): operator name, details
// line, identifiers, the planner's estimated rows, the profile-only actuals
// (nil for a plain EXPLAIN), and child operators.
type Plan struct {
	Operator      string
	Details       string
	Identifiers   []string
	EstimatedRows int64
	Actual        *PlanActual
	Children      []Plan
}

// PlanActual is one operator's PROFILE metrics; nil on an EXPLAIN tree.
type PlanActual struct {
	Rows      int64
	DbHits    int64
	TimeMs    float64
	NodeReads int64
}

// Store is one open Relate database with a single connection over it.
//
// Relate allows one connection at a time for a sequential statement flow; the
// store owns that connection for its lifetime. Opening a store takes an
// exclusive lock on the underlying files — a second open of the same path
// fails until Close releases it.
type Store interface {
	// Run executes the statements in order and returns one result per
	// statement, in source order. With atomic=false each statement runs in
	// its own auto-commit transaction (fail-fast: the first error aborts the
	// run, earlier statements stay committed). With atomic=true all
	// statements run inside ONE explicit transaction: any error rolls the
	// whole run back and nothing commits.
	Run(statements []string, params map[string]any, atomic bool) ([]StatementResult, error)
	// Checkpoint writes a durable snapshot of the store and recycles the WAL.
	// It refuses on a read-only-opened store.
	Checkpoint() error
	// Close releases the connection and the store lock. An open explicit
	// transaction is rolled back implicitly (single-level transactions).
	Close()
}

// OpenStore opens the Relate store at path (creating it when missing) with
// one connection in the given session namespace ("neo4j", "system" or
// "system_ontologies"; "" means "neo4j").
//
// It is a package-level seam: the engine implementation in engine.go (build
// tag `relate`) installs the real opener via init; tests install fakes. In a
// build without the tag it is nil and callers must surface NotCompiledError.
var OpenStore func(path, namespace string) (Store, error)

// OpenStoreReadOnly opens an existing Relate store at path read-only: a
// missing store is NOT created, and writes and checkpoints are refused. Same
// seam contract as OpenStore.
var OpenStoreReadOnly func(path, namespace string) (Store, error)

// NotCompiledError is the typed error for invoking an embedded-engine feature
// (--relate, `relate create`, …) in a build that does not include the engine.
// The message names what was attempted and how to get a build that has it.
func NotCompiledError(what string) error {
	return clierr.NewUsageError(
		"%s requires the Relate embedded engine, which is not compiled into this build "+
			"(build with `-tags relate` and CGO_ENABLED=1 to include it)", what)
}

// EngineError mirrors the engine's typed failure (the C ABI's error shape)
// across the package boundary. The client package's own error type is cgo and
// cannot appear in untagged files, so the tagged engine converts every engine
// failure into this type before returning it. Branch on the fields, never on
// the message text.
type EngineError struct {
	// Code is the Neo4j status code (Neo.<Classification>.<Category>.<Title>).
	Code string
	// GQLStatus is the five-character GQLSTATUS.
	GQLStatus string
	// Stage is "compile", "runtime" or "internal".
	Stage string
	// Retryable is true exactly when re-running the same work may succeed
	// (a write-write conflict, a retryable memory cap).
	Retryable bool
	// Message is the engine's message, or the boundary's for an internal failure.
	Message string
}

func (e *EngineError) Error() string { return e.Message }

// StageInternal marks a failure the engine never saw (a broken handle, a
// caught panic, an undecodable argument, a parameter refusal). The remedy is
// to close and reopen the store, not to retry the statement.
const StageInternal = "internal"

// StoreDir is the directory managed stores live under, inside the CLI's
// config/state tree (sibling of config.json and the credential store).
func StoreDir() string {
	return filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "relate")
}

// ResolveStorePath maps a store argument to a filesystem path. A bare name
// ("movies") resolves under the managed store directory; anything that looks
// like a path (absolute, or containing a separator, or starting with ./ ../
// or an explicit ".") is taken verbatim. The inverse of the naming scheme
// `relate create` uses.
func ResolveStorePath(arg string) string {
	if arg == "" {
		return ""
	}
	if filepath.IsAbs(arg) ||
		strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") ||
		arg == "." || arg == ".." ||
		strings.ContainsRune(arg, '/') || strings.ContainsRune(arg, filepath.Separator) {
		return arg
	}
	return filepath.Join(StoreDir(), arg)
}
