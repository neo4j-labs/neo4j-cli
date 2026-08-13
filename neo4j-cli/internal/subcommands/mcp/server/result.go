// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server

import (
	"errors"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/clievents"
	commonoutput "github.com/neo4j/cli/common/output"
)

// DefaultMaxOutputChars is the default truncation bound for tool result text
// content. Mirrors the --max-output-chars default described in the PRD.
const DefaultMaxOutputChars = 8000

// ResultOptions controls how MapCommandResult transforms a CommandResult into
// a CallToolResult.
type ResultOptions struct {
	// MaxOutputChars is the maximum length of produced text content.
	// Strings longer than this are truncated and a continuation hint is
	// appended. When zero, DefaultMaxOutputChars is used.
	MaxOutputChars int
	// Args is the argv of the command that produced this result. It is
	// redacted before being included in structuredContent so model-supplied
	// secrets never escape the call.
	Args []string
}

// MapCommandResult transforms a CommandResult into an MCP CallToolResult.
// On success the text content is the redacted stdout. On failure isError is
// true and the text content is `Error: <message> (exit N)` plus optional
// suggestion and the redacted output tail. StructuredContent carries
// ce.BuildEnvelope() verbatim with stdout, stderr and redacted argv appended.
//
// Every returned string passes through clievents.RedactText then
// output.StripControl because under MCP these strings are copied into the
// model's context and uploaded. This redaction covers JSON key-value pairs,
// URI userinfo, Bearer/Basic auth headers, key=value assignments, and
// any value pre-registered via clievents.RegisterSecretValue. Table cells
// are NOT covered by the shape-based pass — leaves that mint a runtime secret
// and render it in a table must call RegisterSecretValue first.
//
// MapCommandResult never calls clierr.Render, which writes to streams.
func MapCommandResult(res CommandResult, opts ResultOptions) *mcpsdk.CallToolResult {
	maxChars := opts.MaxOutputChars
	if maxChars <= 0 {
		maxChars = DefaultMaxOutputChars
	}

	redactedStdout := sanitize(res.Stdout)
	redactedStderr := sanitize(res.Stderr)

	if res.Err == nil {
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{
					Text: truncate(redactedStdout, maxChars, "", ""),
				},
			},
		}
	}

	// Map the error into structured content.
	var ce *clierr.CLIError
	if !errors.As(res.Err, &ce) {
		ce = clierr.NewFatalError("%s", res.Err.Error())
	}
	envelope := ce.BuildEnvelope()

	// Build text: "Error: <message> (exit N)" then suggestion when present,
	// then the redacted output tail.
	text := fmt.Sprintf("Error: %s (exit %d)", envelope.Error.Message, envelope.Error.ExitCode)
	if envelope.Error.Suggestion != "" {
		text += "\nSuggestion: " + envelope.Error.Suggestion
	}

	// Append redacted output tail. Use stdout when non-empty, otherwise
	// fall back to stderr so the model sees the command's diagnostic output.
	tail := redactedStdout
	if tail == "" && redactedStderr != "" {
		tail = redactedStderr
	}
	if tail != "" {
		hint := "output truncated"
		if ce.TeePath != "" {
			hint = "output truncated; full output at " + ce.TeePath
		}
		text += "\n" + truncate(tail, maxChars-len(text), "", hint)
	}

	// structuredContent carries the full envelope plus redacted streams and
	// args. Stdout and stderr are also bounded by maxChars to keep the JSON
	// response manageable.
	structuredContent := struct {
		Code       string `json:"code"`
		ExitCode   int    `json:"exit_code"`
		Message    string `json:"message"`
		Resource   string `json:"resource_type,omitempty"`
		ResourceID string `json:"resource_id,omitempty"`
		Suggestion string `json:"suggestion,omitempty"`
		TeePath    string `json:"tee_path,omitempty"`
		Retryable  bool   `json:"retryable"`
		Stdout     string `json:"stdout"`
		Stderr     string `json:"stderr"`
		Argv       string `json:"argv"`
	}{
		Code:       envelope.Error.Code,
		ExitCode:   envelope.Error.ExitCode,
		Message:    envelope.Error.Message,
		Resource:   envelope.Error.ResourceType,
		ResourceID: envelope.Error.ResourceID,
		Suggestion: envelope.Error.Suggestion,
		TeePath:    envelope.Error.TeePath,
		Retryable:  envelope.Error.Retryable,
		Stdout:     truncate(redactedStdout, maxChars, "", ""),
		Stderr:     truncate(redactedStderr, maxChars, "", ""),
		Argv:       clievents.RedactArgs(opts.Args),
	}

	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: text},
		},
		StructuredContent: structuredContent,
	}
}

// sanitize passes a string through RedactText then StripControl. Both must
// run in this order: StripControl would neutralise the markers RedactText's
// regexes rely on if it ran first.
func sanitize(s string) string {
	return commonoutput.StripControl(clievents.RedactText(s))
}

// truncate chops s to at most max runes, or returns s unchanged when it fits.
// When truncated, hint (prefixed by a newline) is appended so the model knows
// the full result was larger. An empty hint appends nothing.
func truncate(s string, max int, _, hint string) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	out := string(runes[:max])
	if hint != "" {
		out += "\n" + hint
	}
	return out
}
