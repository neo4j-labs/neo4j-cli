// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clierr

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Envelope is the top-level wrapper for the JSON error envelope emitted on
// stdout when a command fails and `--format=json` is in effect. The shape is
// a public contract per agent-cli-auditor.md §3.3; field names use snake_case
// and optional fields use `omitempty` so the envelope stays compact when
// hints aren't populated.
type Envelope struct {
	Error EnvelopeBody `json:"error"`
}

// EnvelopeBody is the body of the Envelope. `retryable` is always present;
// `resource_type`, `resource_id`, and `suggestion` are omitted when empty.
type EnvelopeBody struct {
	Code         string `json:"code"`
	ExitCode     int    `json:"exit_code"`
	Message      string `json:"message"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	Suggestion   string `json:"suggestion,omitempty"`
	Retryable    bool   `json:"retryable"`
}

// BuildEnvelope projects a *CLIError into its on-the-wire Envelope. Pure
// function: no I/O, no allocation beyond the struct literal. Callers needing
// to mutate the envelope before marshalling can take the return by value.
func (e *CLIError) BuildEnvelope() Envelope {
	return Envelope{
		Error: EnvelopeBody{
			Code:         e.CodeName,
			ExitCode:     e.Code,
			Message:      e.Message,
			ResourceType: e.ResourceType,
			ResourceID:   e.ResourceID,
			Suggestion:   e.Suggestion,
			Retryable:    e.Retryable,
		},
	}
}

// Render writes the error to the appropriate streams based on `format`.
//
//   - format == "json": marshal BuildEnvelope() to stdout (trailing newline)
//     and write a one-line `Error: <message> (exit <N>)` summary to stderr.
//   - any other format (including "", "default", "table", "toon", unknown
//     values): write the one-line summary to stderr; if Suggestion is set
//     write it on a second stderr line; stdout is untouched.
//
// Errors that are not *CLIError (directly or via errors.As) are wrapped as
// if they came from NewFatalError — exit 1, code "fatal_error", retryable
// false. nil err is a no-op.
func Render(err error, stdout, stderr io.Writer, format string) {
	if err == nil {
		return
	}

	var ce *CLIError
	if !errors.As(err, &ce) {
		ce = NewFatalError("%s", err.Error())
	}

	if format == "json" {
		// Marshal to stdout first; if marshalling somehow fails, fall back
		// to the plaintext path so the user still sees something useful.
		if buf, mErr := json.Marshal(ce.BuildEnvelope()); mErr == nil {
			_, _ = stdout.Write(buf)
			_, _ = io.WriteString(stdout, "\n")
			_, _ = fmt.Fprintf(stderr, "Error: %s (exit %d)\n", ce.Message, ce.Code)
			return
		}
	}

	_, _ = fmt.Fprintf(stderr, "Error: %s (exit %d)\n", ce.Message, ce.Code)
	if ce.Suggestion != "" {
		_, _ = fmt.Fprintf(stderr, "%s\n", ce.Suggestion)
	}
}
