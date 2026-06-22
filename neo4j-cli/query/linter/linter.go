// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package linter lints Cypher offline by running an esbuild bundle of
// cypher-language-support's lintCypherQuery (vendored cypherLint.js, see
// README.md) inside the goja JavaScript engine. It reports syntax and
// semantic problems without any database connection, and adds schema-aware
// checks (unknown labels/relationship types, path directionality) when a
// DbSchema is supplied.
package linter

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/dop251/goja"
)

//go:embed cypherLint.js
var cypherLintJS string

// Version selects the Cypher language dialect the analyzer lints against.
type Version string

const (
	Cypher5  Version = "CYPHER 5"
	Cypher25 Version = "CYPHER 25"
)

// DbSchema is the database-schema subset lintCypherQuery consumes. It is a
// wire struct mirroring cypher-language-support's DbSchema (camelCase JSON
// keys, exempt from the snake_case output rule like desktopclient/types.go):
// it is marshalled and handed to the JS engine, never printed.
//
// Schema-aware diagnostics activate per key: unknown-label/relType warnings
// need BOTH Labels and RelationshipTypes; path-directionality warnings need
// GraphSchema. PropertyKeys produces no diagnostic yet (it feeds upstream
// autocompletion today) but is plumbed through because it is next on the
// upstream linter roadmap. DefaultLanguage participates in Cypher version
// resolution: query prologue > DefaultLanguage > the version argument.
// Parameters switches the analyzer's parameter checking on: when set, every
// $param absent from the map is an error; when nil, parameter errors are
// suppressed entirely (parameters are assumed to be supplied externally).
//
// Procedures and Functions are upstream's ScopedRegistry shape: Cypher
// dialect ("CYPHER 5"/"CYPHER 25") → name → the raw SHOW PROCEDURES/SHOW
// FUNCTIONS row. Their presence makes unknown procedures/functions errors
// and deprecated ones warnings, and feeds CALL/YIELD signature resolution.
// Two sharp edges, both upstream:
//   - The existence check consults only the resolved dialect's key. A
//     populated Procedures/Functions map that is missing a dialect key makes
//     EVERY procedure/function "unknown" for queries resolving to that
//     dialect — populate both keys or neither.
//   - The semantic-analysis wrapper swallows all exceptions, so registry
//     entries the TeaVM signature resolver cannot digest silently disable
//     every semantic check. Pass complete SHOW ... YIELD * rows through
//     verbatim (as cypher-language-support's metadata poller does), never a
//     hand-built subset.
type DbSchema struct {
	Labels            []string            `json:"labels,omitempty"`
	RelationshipTypes []string            `json:"relationshipTypes,omitempty"`
	PropertyKeys      []string            `json:"propertyKeys,omitempty"`
	GraphSchema       []GraphSchemaRel    `json:"graphSchema,omitempty"`
	DefaultLanguage   string              `json:"defaultLanguage,omitempty"`
	Parameters        map[string]any      `json:"parameters,omitempty"`
	Procedures        map[string]Registry `json:"procedures,omitempty"`
	Functions         map[string]Registry `json:"functions,omitempty"`
}

// Registry maps a procedure/function name to its raw SHOW PROCEDURES / SHOW
// FUNCTIONS row (driver-coerced, JSON-marshalable). Rows pass through to the
// analyzer untouched — see the DbSchema doc for why subsetting is unsafe.
type Registry map[string]map[string]any

// GraphSchemaRel is one (from)-[relType]->(to) triple from
// `CALL db.schema.visualization()`, the shape findPathIssues expects.
type GraphSchemaRel struct {
	From    string `json:"from"`
	To      string `json:"to"`
	RelType string `json:"relType"`
}

// Position is a location in the linted query, 0-indexed for line, column and
// byte offset — exactly as the analyzer reports it. Callers that present
// positions to humans typically convert line/column to 1-indexed.
type Position struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Diagnostic is a single problem found in the query. Severity is "error"
// (syntax or semantic error) or "warning" (analyzer notification).
type Diagnostic struct {
	Severity string
	Message  string
	Start    Position
	End      Position
}

// The vendored artifact is an esbuild IIFE bundle (--format=iife
// --global-name=cypherLint): evaluating it defines a single `cypherLint`
// global object carrying the exports. goja evaluates scripts only (no ES
// modules), and the global-name form needs no postprocessing — unlike the
// minified identifiers inside the bundle, the global's name and its export
// keys are stable across esbuild versions and rebuilds.
const artifactGlobal = "cypherLint"

// prelude shims the handful of browser/node globals and post-ES2017
// built-ins the bundle expects that goja does not provide. WeakRef is a
// strong-reference polyfill: the engine lives for one CLI invocation, so
// never collecting the referents is a bounded, acceptable leak. The Set and
// iterator polyfills cover the ES2025 methods labelTreeWalking.ts uses
// (union/intersection on real Sets, forEach on built-in iterators) —
// esbuild's --target only lowers syntax, not library methods.
const prelude = `
if (typeof process === 'undefined') {
    globalThis.process = { versions: {} };
}
if (typeof WeakRef === 'undefined') {
    globalThis.WeakRef = class WeakRef {
        constructor(v) { this._v = v; }
        deref() { return this._v; }
    };
}
if (typeof FinalizationRegistry === 'undefined') {
    globalThis.FinalizationRegistry = class FinalizationRegistry {
        constructor(cb) {}
        register() {}
        unregister() {}
    };
}
if (typeof console === 'undefined') {
    globalThis.console = { log: function(){}, warn: function(){}, error: function(){} };
}
if (typeof setTimeout === 'undefined') {
    globalThis.setTimeout = function(fn) { fn(); return 0; };
    globalThis.clearTimeout = function() {};
}
(function() {
    if (typeof Set.prototype.union !== 'function') {
        Set.prototype.union = function(other) { const r = new Set(this); for (const v of other) r.add(v); return r; };
    }
    if (typeof Set.prototype.intersection !== 'function') {
        Set.prototype.intersection = function(other) { const r = new Set(); for (const v of this) if (other.has(v)) r.add(v); return r; };
    }
    const iterProto = Object.getPrototypeOf(Object.getPrototypeOf([][Symbol.iterator]()));
    if (typeof iterProto.forEach !== 'function') {
        iterProto.forEach = function(fn) { let i = 0; for (const v of this) fn(v, i++); };
    }
})();
`

// glue adapts lintCypherQuery to a stable JSON contract on the JS side.
// lintCypherQuery errors on EVERY $param absent from dbSchema.parameters, so
// when no parameters were declared the parameter-not-defined errors are
// filtered with the upstream-exported isNotParamError predicate — mirroring
// how the language server and react-codemirror suppress them when not
// connected. With declared parameters the check runs for real. LSP severity
// 1 is an error; 2/3/4 (warning/info/hint) are all reported as
// notifications.
const glue = `
globalThis.lintJson = function(query, version, schemaJson) {
    const schema = schemaJson ? JSON.parse(schemaJson) : {};
    if (!schema.defaultLanguage) schema.defaultLanguage = version;
    const r = cypherLint.lintCypherQuery(query, schema, { consoleCommandsEnabled: false });
    const diags = schema.parameters ? r.diagnostics : r.diagnostics.filter(cypherLint.isNotParamError);
    const elem = (d) => ({
        message: d.message,
        start: { offset: d.offsets.start, line: d.range.start.line, column: d.range.start.character },
        end: { offset: d.offsets.end, line: d.range.end.line, column: d.range.end.character },
    });
    return JSON.stringify({
        errors: diags.filter((d) => d.severity === 1).map(elem),
        notifications: diags.filter((d) => d.severity !== 1).map(elem),
    });
};
`

// engine wraps the shared goja runtime. goja.Runtime is not safe for
// concurrent use, so every call into the VM holds mu.
type engine struct {
	mu   sync.Mutex
	vm   *goja.Runtime
	lint goja.Callable
}

var (
	engineOnce sync.Once
	sharedEng  *engine
	engineErr  error
)

func getEngine() (*engine, error) {
	engineOnce.Do(func() { sharedEng, engineErr = newEngine() })
	return sharedEng, engineErr
}

func newEngine() (*engine, error) {
	vm := goja.New()
	if _, err := vm.RunString(prelude); err != nil {
		return nil, fmt.Errorf("linter: prelude: %w", err)
	}
	prog, err := goja.Compile("cypherLint.js", cypherLintJS, false)
	if err != nil {
		return nil, fmt.Errorf("linter: compile cypherLint.js: %w", err)
	}
	if _, err := vm.RunProgram(prog); err != nil {
		return nil, fmt.Errorf("linter: evaluate cypherLint.js: %w", err)
	}
	if v := vm.Get(artifactGlobal); v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, fmt.Errorf(
			"linter: %s global missing after artifact evaluation: vendored artifact not built with --global-name=%s (see neo4j-cli/query/linter/README.md)",
			artifactGlobal, artifactGlobal)
	}
	if _, err := vm.RunString(glue); err != nil {
		return nil, fmt.Errorf("linter: glue: %w", err)
	}
	lint, ok := goja.AssertFunction(vm.Get("lintJson"))
	if !ok {
		return nil, fmt.Errorf("linter: lintJson is not a function after artifact evaluation")
	}
	return &engine{vm: vm, lint: lint}, nil
}

// rawElement / rawResult mirror the JSON shape produced by the glue function.
type rawElement struct {
	Message string   `json:"message"`
	Start   Position `json:"start"`
	End     Position `json:"end"`
}

type rawResult struct {
	Errors        []rawElement `json:"errors"`
	Notifications []rawElement `json:"notifications"`
}

// Lint analyzes the query and returns all diagnostics sorted by start offset
// (errors before warnings on ties). A nil schema lints schema-less, exactly
// as before schema support existed; a non-nil schema additionally activates
// the schema-aware checks its populated keys enable. The version argument is
// the fallback Cypher dialect: a `CYPHER 5`/`CYPHER 25` prologue in the
// query wins, then schema.DefaultLanguage, then version. The first call pays
// the engine initialization cost (~1s) and the analyzer's lazy setup (first
// lint ~2.5s total); subsequent calls in the same process take ~0.3-0.6s.
func Lint(query string, version Version, schema *DbSchema) ([]Diagnostic, error) {
	e, err := getEngine()
	if err != nil {
		return nil, err
	}

	schemaJSON := ""
	if schema != nil {
		b, err := json.Marshal(schema)
		if err != nil {
			return nil, fmt.Errorf("linter: marshal schema: %w", err)
		}
		schemaJSON = string(b)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	v, err := e.lint(goja.Undefined(), e.vm.ToValue(query), e.vm.ToValue(string(version)), e.vm.ToValue(schemaJSON))
	if err != nil {
		return nil, fmt.Errorf("linter: semantic analysis failed: %w", err)
	}
	var raw rawResult
	if err := json.Unmarshal([]byte(v.String()), &raw); err != nil {
		return nil, fmt.Errorf("linter: decode analysis result: %w", err)
	}

	// Append order is load-bearing: errors go in before warnings so the
	// stable sort below resolves equal-offset ties errors-first.
	diags := make([]Diagnostic, 0, len(raw.Errors)+len(raw.Notifications))
	for _, el := range raw.Errors {
		diags = append(diags, Diagnostic{Severity: "error", Message: el.Message, Start: el.Start, End: el.End})
	}
	for _, el := range raw.Notifications {
		diags = append(diags, Diagnostic{Severity: "warning", Message: el.Message, Start: el.Start, End: el.End})
	}
	sort.SliceStable(diags, func(i, j int) bool { return diags[i].Start.Offset < diags[j].Start.Offset })
	return diags, nil
}
