// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package linter lints Cypher offline by running the TeaVM-compiled
// semantic-analysis JavaScript artifact (vendored semanticAnalysis.js, see
// README.md) inside the goja JavaScript engine. It reports both syntax and
// semantic problems without any database connection.
package linter

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

//go:embed semanticAnalysis.js
var semanticAnalysisJS string

// Version selects the Cypher language dialect the analyzer lints against.
type Version string

const (
	Cypher5  Version = "CYPHER 5"
	Cypher25 Version = "CYPHER 25"
)

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

// The vendored artifact is an ES module; goja evaluates scripts only, so the
// trailing export statement is rewritten into globalThis assignments before
// evaluation. The marker is the artifact's exact final statement — if an
// artifact refresh renames the minified identifiers, rewriteExports fails
// loudly and the README documents the fix.
const (
	exportMarker      = "export{C as updateSignatureResolver,D as analyzeQuery};"
	exportReplacement = "globalThis.updateSignatureResolver=C;globalThis.analyzeQuery=D;"
)

func rewriteExports(src string) (string, error) {
	if !strings.Contains(src, exportMarker) {
		return "", fmt.Errorf(
			"semanticAnalysis.js export marker not found: vendored artifact shape changed (see neo4j-cli/query/linter/README.md)")
	}
	return strings.Replace(src, exportMarker, exportReplacement, 1), nil
}

// prelude shims the handful of browser/node globals the TeaVM output expects
// that goja does not provide. WeakRef is a strong-reference polyfill: the
// engine lives for one CLI invocation, so never collecting the referents is a
// bounded, acceptable leak.
const prelude = `
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
`

// glue marshals analyzer results to a JSON string on the JS side. The result
// objects expose their data via prototype getters, so exporting the raw value
// to Go would only surface minified internals — this mirrors how
// semanticAnalysisWrapper.ts consumes the artifact upstream.
const glue = `
globalThis.lintJson = function(query, version) {
    const r = analyzeQuery(query, version);
    const elem = (e) => ({
        message: e.message,
        start: { offset: e.startPosition.offset, line: e.startPosition.line, column: e.startPosition.column },
        end: { offset: e.endPosition.offset, line: e.endPosition.line, column: e.endPosition.column },
    });
    return JSON.stringify({
        errors: Array.from(r.errors).map(elem),
        notifications: Array.from(r.notifications).map(elem),
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
	code, err := rewriteExports(semanticAnalysisJS)
	if err != nil {
		return nil, fmt.Errorf("linter: %w", err)
	}
	vm := goja.New()
	if _, err := vm.RunString(prelude); err != nil {
		return nil, fmt.Errorf("linter: prelude: %w", err)
	}
	prog, err := goja.Compile("semanticAnalysis.js", code, false)
	if err != nil {
		return nil, fmt.Errorf("linter: compile semanticAnalysis.js: %w", err)
	}
	if _, err := vm.RunProgram(prog); err != nil {
		return nil, fmt.Errorf("linter: evaluate semanticAnalysis.js: %w", err)
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

// Lint analyzes the query against the given Cypher version and returns all
// diagnostics sorted by start offset (errors before warnings on ties). The
// first call pays the engine initialization cost (~2.5s); subsequent calls in
// the same process are much cheaper.
func Lint(query string, version Version) ([]Diagnostic, error) {
	e, err := getEngine()
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	v, err := e.lint(goja.Undefined(), e.vm.ToValue(query), e.vm.ToValue(string(version)))
	if err != nil {
		return nil, fmt.Errorf("linter: semantic analysis failed: %w", err)
	}
	var raw rawResult
	if err := json.Unmarshal([]byte(v.String()), &raw); err != nil {
		return nil, fmt.Errorf("linter: decode analysis result: %w", err)
	}

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
