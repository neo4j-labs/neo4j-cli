// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clierr

import "fmt"

// CLIError is a typed error carrying a process exit code so that the top-level
// main can map it via errors.As to os.Exit. The closed set of codes mirrors the
// agent-cli-auditor.md §4.1 table and is also advertised by `neo4j-cli
// agent-context`.
type CLIError struct {
	Code       int
	Message    string
	Err        error
	RetryAfter string
}

func (e *CLIError) Error() string {
	return e.Message
}

func (e *CLIError) Unwrap() error {
	return e.Err
}

// NewFatalError — exit 1 — unrecoverable internal failure.
func NewFatalError(msg string, a ...any) error {
	return newCLIError(1, msg, a...)
}

// NewUsageError — exit 2 — bad flag, missing arg, malformed invocation.
func NewUsageError(msg string, a ...any) error {
	return newCLIError(2, msg, a...)
}

// NewNotFoundError — exit 3 — resource doesn't exist.
func NewNotFoundError(msg string, a ...any) error {
	return newCLIError(3, msg, a...)
}

// NewAuthError — exit 4 — authentication or authorization failed.
func NewAuthError(msg string, a ...any) error {
	return newCLIError(4, msg, a...)
}

// NewConflictError — exit 5 — request conflicts with current resource state.
func NewConflictError(msg string, a ...any) error {
	return newCLIError(5, msg, a...)
}

// NewValidationError — exit 6 — input payload rejected by validation.
func NewValidationError(msg string, a ...any) error {
	return newCLIError(6, msg, a...)
}

// NewRateLimitError — exit 7 — upstream signalled rate limit. retryAfter is the
// server-provided hint (e.g. the Retry-After header value); the message body
// should still mention the value so terminal users see it.
func NewRateLimitError(retryAfter string, msg string, a ...any) error {
	e := newCLIError(7, msg, a...)
	e.RetryAfter = retryAfter
	return e
}

// NewUpstreamError — exit 8 — upstream/transport failure that a retry may
// resolve (5xx, network, transient Bolt errors).
func NewUpstreamError(msg string, a ...any) error {
	return newCLIError(8, msg, a...)
}

func newCLIError(code int, msg string, a ...any) *CLIError {
	formatted := fmt.Errorf(msg, a...)
	return &CLIError{
		Code:    code,
		Message: formatted.Error(),
		Err:     unwrapFormatted(formatted),
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
