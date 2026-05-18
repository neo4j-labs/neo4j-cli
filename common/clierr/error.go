// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clierr

import "fmt"

// CLIError is a typed error carrying a process exit code so that the top-level
// main can map it via errors.As to os.Exit. The closed set of codes mirrors the
// agent-cli-auditor.md §4.1 table and is also advertised by `neo4j-cli
// agent-context`.
//
// CodeName is the machine-readable enum string for the exit code (e.g.
// "not_found" for Code 3) used by the JSON error envelope. Retryable is
// derived from the exit code (true only for rate_limited / upstream_error)
// and flags whether a transient retry might succeed. ResourceType,
// ResourceID, and Suggestion are optional envelope hints populated at the
// call site via WithResource / WithSuggestion.
type CLIError struct {
	Code         int
	CodeName     string
	Message      string
	Err          error
	RetryAfter   string
	Retryable    bool
	ResourceType string
	ResourceID   string
	Suggestion   string
}

func (e *CLIError) Error() string {
	return e.Message
}

func (e *CLIError) Unwrap() error {
	return e.Err
}

// WithResource tags the error with a resource type and id so the JSON
// envelope can surface them. Both fields are optional in the envelope
// (omitempty); pass empty strings to clear. Mutates the receiver in place
// and returns it so callers can chain (e.g.
// `return clierr.NewNotFoundError("...").WithResource("instance", id)`).
func (e *CLIError) WithResource(resourceType, resourceID string) *CLIError {
	e.ResourceType = resourceType
	e.ResourceID = resourceID
	return e
}

// WithSuggestion attaches a short next-action hint to the error. Rendered
// as a `suggestion` field in the JSON envelope (omitempty) and as a second
// stderr line in plaintext mode.
func (e *CLIError) WithSuggestion(s string) *CLIError {
	e.Suggestion = s
	return e
}

// CodeMetadata describes one entry in the closed enum of CLI exit codes.
// Name is the machine-readable enum string (snake_case); Description is a
// short human sentence used by `neo4j-cli agent-context`; Retryable is
// true only for transient failure classes where a retry may succeed.
type CodeMetadata struct {
	Name        string
	Description string
	Retryable   bool
}

// Codes is the canonical closed-enum table keyed by process exit code.
// This is the single source of truth for the eight CLI error categories
// (mirrors agent-cli-auditor.md §4.1). Constructors look up Name and
// Retryable here; `agent-context` rebuilds its error_codes map from
// CodeNames / CodeDescriptions; tests use it as a fixture.
var Codes = map[int]CodeMetadata{
	1: {Name: "fatal_error", Description: "unrecoverable internal failure", Retryable: false},
	2: {Name: "usage_error", Description: "invalid flag, missing argument, or other input rejection", Retryable: false},
	3: {Name: "not_found", Description: "resource doesn't exist", Retryable: false},
	4: {Name: "auth_error", Description: "authentication or authorization failed", Retryable: false},
	5: {Name: "conflict", Description: "request conflicts with current resource state", Retryable: false},
	6: {Name: "validation_error", Description: "input payload rejected by validation", Retryable: false},
	7: {Name: "rate_limited", Description: "upstream signalled rate limit; retry after the hinted delay", Retryable: true},
	8: {Name: "upstream_error", Description: "transient API failure; retry may succeed", Retryable: true},
}

// CodeNames is a flat exit-code → enum-string projection of Codes, exposed
// for consumers (notably `agent-context`) that only need the name map.
var CodeNames = func() map[int]string {
	out := make(map[int]string, len(Codes))
	for code, meta := range Codes {
		out[code] = meta.Name
	}
	return out
}()

// CodeDescriptions is a flat exit-code → human-description projection of
// Codes. Paired with CodeNames so callers can rebuild
// `map[codeName]description` shape without duplicating the table.
var CodeDescriptions = func() map[int]string {
	out := make(map[int]string, len(Codes))
	for code, meta := range Codes {
		out[code] = meta.Description
	}
	return out
}()

// NewFatalError — exit 1 — unrecoverable internal failure.
func NewFatalError(msg string, a ...any) *CLIError {
	return newCLIError(1, msg, a...)
}

// NewUsageError — exit 2 — bad flag, missing arg, malformed invocation.
func NewUsageError(msg string, a ...any) *CLIError {
	return newCLIError(2, msg, a...)
}

// NewNotFoundError — exit 3 — resource doesn't exist.
func NewNotFoundError(msg string, a ...any) *CLIError {
	return newCLIError(3, msg, a...)
}

// NewAuthError — exit 4 — authentication or authorization failed.
func NewAuthError(msg string, a ...any) *CLIError {
	return newCLIError(4, msg, a...)
}

// NewConflictError — exit 5 — request conflicts with current resource state.
func NewConflictError(msg string, a ...any) *CLIError {
	return newCLIError(5, msg, a...)
}

// NewValidationError — exit 6 — input payload rejected by validation.
func NewValidationError(msg string, a ...any) *CLIError {
	return newCLIError(6, msg, a...)
}

// NewRateLimitError — exit 7 — upstream signalled rate limit. retryAfter is the
// server-provided hint (e.g. the Retry-After header value); the message body
// should still mention the value so terminal users see it.
func NewRateLimitError(retryAfter string, msg string, a ...any) *CLIError {
	e := newCLIError(7, msg, a...)
	e.RetryAfter = retryAfter
	return e
}

// NewUpstreamError — exit 8 — upstream/transport failure that a retry may
// resolve (5xx, network, transient Bolt errors).
func NewUpstreamError(msg string, a ...any) *CLIError {
	return newCLIError(8, msg, a...)
}

func newCLIError(code int, msg string, a ...any) *CLIError {
	formatted := fmt.Errorf(msg, a...)
	meta := Codes[code]
	return &CLIError{
		Code:      code,
		CodeName:  meta.Name,
		Retryable: meta.Retryable,
		Message:   formatted.Error(),
		Err:       unwrapFormatted(formatted),
	}
}

// unwrapFormatted preserves any wrapped error chain coming through a `%w` in
// the constructor's format string, so errors.As traversal still finds inner
// errors when callers wrap a CLIError via fmt.Errorf("...: %w", inner).
func unwrapFormatted(err error) error {
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return u.Unwrap()
	}
	return nil
}
