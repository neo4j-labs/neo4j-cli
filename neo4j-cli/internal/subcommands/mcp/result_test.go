// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapCommandResult_SuccessReturnsRedactedStdout(t *testing.T) {
	res := mcp.CommandResult{
		Stdout: `[{"name":"test","uri":"neo4j://user:pass@localhost"}]`,
		Stderr: "",
		Err:    nil,
	}
	result := mcp.MapCommandResult(res, mcp.ResultOptions{})

	assert.False(t, result.IsError, "a successful command must not set isError")

	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok, "content[0] must be TextContent")

	// The URI password must be redacted.
	assert.NotContains(t, tc.Text, "pass")
	assert.Contains(t, tc.Text, "neo4j://user:***@localhost")
}

func TestMapCommandResult_PlainErrorBecomesFatal(t *testing.T) {
	res := mcp.CommandResult{
		Stdout: "",
		Stderr: "something went wrong",
		Err:    errors.New("kaboom"),
	}
	result := mcp.MapCommandResult(res, mcp.ResultOptions{})

	assert.True(t, result.IsError, "a failing command must set isError")
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "Error: kaboom (exit 1)", "plain errors become exit 1")

	// Structured content must exist.
	require.NotNil(t, result.StructuredContent)
	b, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)

	var envelope struct {
		Code     string `json:"code"`
		ExitCode int    `json:"exit_code"`
		Message  string `json:"message"`
	}
	err = json.Unmarshal(b, &envelope)
	require.NoError(t, err)
	assert.Equal(t, "fatal_error", envelope.Code)
	assert.Equal(t, 1, envelope.ExitCode)
	assert.Equal(t, "kaboom", envelope.Message)
}

func TestMapCommandResult_CLIErrorCarriesFullEnvelope(t *testing.T) {
	ce := clierr.NewUsageError("invalid input").
		WithSuggestion("use --name instead").
		WithResource("instance", "abc-123").
		WithTeePath("/tmp/tee")
	res := mcp.CommandResult{
		Stdout: "",
		Stderr: "",
		Err:    ce,
	}

	result := mcp.MapCommandResult(res, mcp.ResultOptions{Args: []string{"create", "--name", "test"}})

	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "Error: invalid input (exit 2)")
	assert.Contains(t, tc.Text, "Suggestion: use --name instead")

	require.NotNil(t, result.StructuredContent)
	b, _ := json.Marshal(result.StructuredContent)

	var envelope struct {
		Code       string `json:"code"`
		ExitCode   int    `json:"exit_code"`
		Message    string `json:"message"`
		Resource   string `json:"resource_type"`
		ResourceID string `json:"resource_id"`
		Suggestion string `json:"suggestion"`
		TeePath    string `json:"tee_path"`
		Retryable  bool   `json:"retryable"`
		Stdout     string `json:"stdout"`
		Stderr     string `json:"stderr"`
		Argv       string `json:"argv"`
	}
	require.NoError(t, json.Unmarshal(b, &envelope))
	assert.Equal(t, "usage_error", envelope.Code)
	assert.Equal(t, 2, envelope.ExitCode)
	assert.Equal(t, "invalid input", envelope.Message)
	assert.Equal(t, "instance", envelope.Resource)
	assert.Equal(t, "abc-123", envelope.ResourceID)
	assert.Equal(t, "use --name instead", envelope.Suggestion)
	assert.Equal(t, "/tmp/tee", envelope.TeePath)
	assert.False(t, envelope.Retryable)
	assert.Contains(t, envelope.Argv, "create")
	assert.Contains(t, envelope.Argv, "test")
}

func TestMapCommandResult_RetryableReflectsExitCode(t *testing.T) {
	for _, tc := range []struct {
		name      string
		code      int
		retryable bool
	}{
		{name: "rate limited is retryable", code: 7, retryable: true},
		{name: "upstream error is retryable", code: 8, retryable: true},
		{name: "usage error is not retryable", code: 2, retryable: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := mcp.CommandResult{
				Err: clierr.NewFatalError(""),
			}
			// Override code by constructing the right error type.
			switch tc.code {
			case 2:
				res.Err = clierr.NewUsageError("bad")
			case 7:
				res.Err = clierr.NewRateLimitError("5s", "too fast")
			case 8:
				res.Err = clierr.NewUpstreamError("retry")
			default:
				res.Err = clierr.NewFatalError("boom")
			}

			result := mcp.MapCommandResult(res, mcp.ResultOptions{})
			require.True(t, result.IsError)

			b, _ := json.Marshal(result.StructuredContent)
			var envelope struct {
				Retryable bool `json:"retryable"`
			}
			require.NoError(t, json.Unmarshal(b, &envelope))
			assert.Equal(t, tc.retryable, envelope.Retryable, "exit %d", tc.code)
		})
	}
}

// TestMapCommandResult_RedactsPasswordFromJSON proves the shape-based
// RedactText pass catches a password printed in --format json output, which
// is the same shape docker create emits. The mapper receives neither a secret
// flag nor a RegisterSecretValue call — the JSON field regex alone must catch
// `"password":"<value>"`.
func TestMapCommandResult_RedactsPasswordFromJSON(t *testing.T) {
	const password = "my-secret-password-123"

	exec := newExecutor(t, stubFactory(func() *cobra.Command {
		leaf := &cobra.Command{
			Use: "secret-mint",
			RunE: func(cmd *cobra.Command, _ []string) error {
				// Print JSON matching docker create's --format json output shape.
				// `password` is the output field name, NOT a flag — so RedactArgs
				// does not catch it. The mapper must rely on RedactText's JSON
				// field regex instead.
				_, err := fmt.Fprintf(cmd.OutOrStdout(),
					`[{"name":"test","password":"%s","uri":"neo4j://localhost:7687"}]`+"\n", password)
				return err
			},
		}
		return leaf
	}))

	res := exec.Execute(context.Background(), []string{"secret-mint"})
	require.NoError(t, res.Err)

	result := mcp.MapCommandResult(res, mcp.ResultOptions{MaxOutputChars: 20000})

	assert.False(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.NotContains(t, tc.Text, password, "the JSON password must be redacted")
	assert.Contains(t, tc.Text, `"password":"***"`, "RedactText must rewrite the JSON password to ***")
}

func TestMapCommandResult_ControlBytesAreStripped(t *testing.T) {
	res := mcp.CommandResult{
		Stdout: "normal text\x1b[31mred\x00\ttab\t\r\n",
		Err:    nil,
	}
	result := mcp.MapCommandResult(res, mcp.ResultOptions{MaxOutputChars: 20000})

	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.NotContains(t, tc.Text, "\x1b", "ANSI escape must be stripped")
	assert.NotContains(t, tc.Text, "\x00", "null byte must be stripped")
	assert.Contains(t, tc.Text, "\t", "tab must be preserved")
	assert.Contains(t, tc.Text, "\r\n", "CRLF must be preserved")
	assert.Contains(t, tc.Text, "normal text")
	assert.Contains(t, tc.Text, "red")
}

func TestMapCommandResult_TruncatesLongOutput(t *testing.T) {
	long := strings.Repeat("x", 10000)
	res := mcp.CommandResult{
		Stdout: long,
		Err:    nil,
	}
	result := mcp.MapCommandResult(res, mcp.ResultOptions{MaxOutputChars: 100})

	tc := result.Content[0].(*mcpsdk.TextContent)
	// Success case: no hint since suffix and suffixHint are both empty.
	assert.Len(t, tc.Text, 100, "text must be truncated to MaxOutputChars runes")
	// The first 100 chars must be the original string.
	assert.Equal(t, long[:100], tc.Text[:100])
}

func TestMapCommandResult_TruncationWithTeePath(t *testing.T) {
	long := strings.Repeat("y", 5000)
	ce := clierr.NewFatalError("too much output").WithTeePath("/path/to/tee.out")
	res := mcp.CommandResult{
		Stdout: long,
		Stderr: "",
		Err:    ce,
	}
	result := mcp.MapCommandResult(res, mcp.ResultOptions{MaxOutputChars: 50})

	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "/path/to/tee.out", "tee_path must appear in the truncation hint")

	b, _ := json.Marshal(result.StructuredContent)
	var envelope struct {
		TeePath string `json:"tee_path"`
	}
	require.NoError(t, json.Unmarshal(b, &envelope))
	assert.Equal(t, "/path/to/tee.out", envelope.TeePath)
}

// TestMapCommandResult_RedactedArgsInStructuredContent checks that
// model-supplied secrets passed as flags are redacted from the argv field.
func TestMapCommandResult_RedactedArgsInStructuredContent(t *testing.T) {
	res := mcp.CommandResult{
		Err: clierr.NewUsageError("bad flag"),
	}
	result := mcp.MapCommandResult(res, mcp.ResultOptions{
		Args: []string{"create", "--password", "hunter2", "--name", "test"},
	})

	b, _ := json.Marshal(result.StructuredContent)
	var envelope struct {
		Argv string `json:"argv"`
	}
	require.NoError(t, json.Unmarshal(b, &envelope))
	assert.Contains(t, envelope.Argv, "--password ***")
	assert.NotContains(t, envelope.Argv, "hunter2", "flag value must be redacted")
	assert.Contains(t, envelope.Argv, "create")
	assert.Contains(t, envelope.Argv, "--name test")
}

// TestMapCommandResult_StdoutFallbackToStderrOnError proves that when stdout
// is empty and stderr has diagnostic output, the error text includes stderr.
func TestMapCommandResult_StdoutFallbackToStderrOnError(t *testing.T) {
	res := mcp.CommandResult{
		Stdout: "",
		Stderr: "container exited with code 1",
		Err:    clierr.NewUpstreamError("docker failed"),
	}
	result := mcp.MapCommandResult(res, mcp.ResultOptions{})

	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "container exited with code 1", "stderr must be used as output tail")
}

// TestMapCommandResult_NoSecretsThroughSanitize ensures that secrets embedded
// in various formats are all redacted by the sanitize path.
func TestMapCommandResult_NoSecretsThroughSanitize(t *testing.T) {
	// Register a known secret value so the literal-match pass catches it.
	secret := "sk-live-abc123def456"
	clievents.RegisterSecretValue(secret)

	output := `{
		"uri": "bolt://user:hunter2@host:7687",
		"token": "sk-live-abc123def456",
		"auth": "Bearer secret-token"
	}`
	res := mcp.CommandResult{
		Stdout: output,
		Err:    nil,
	}
	result := mcp.MapCommandResult(res, mcp.ResultOptions{MaxOutputChars: 20000})

	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.NotContains(t, tc.Text, "hunter2")
	assert.NotContains(t, tc.Text, "sk-live-abc123def456")
	assert.NotContains(t, tc.Text, "secret-token")
	assert.Contains(t, tc.Text, "***")
}

// TestMapCommandResult_ZeroMaxOutputCharsUsesDefault proves that omitting
// MaxOutputChars falls back to DefaultMaxOutputChars without panicking.
func TestMapCommandResult_ZeroMaxOutputCharsUsesDefault(t *testing.T) {
	// Build output slightly larger than the default so truncation kicks in.
	text := strings.Repeat("z", mcp.DefaultMaxOutputChars+100)
	res := mcp.CommandResult{
		Stdout: text,
		Err:    nil,
	}
	result := mcp.MapCommandResult(res, mcp.ResultOptions{MaxOutputChars: 0})

	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Len(t, tc.Text, mcp.DefaultMaxOutputChars,
		"zero MaxOutputChars must use the default")
	// The output should be the first DefaultMaxOutputChars runes unchanged.
	assert.Equal(t, text[:mcp.DefaultMaxOutputChars], tc.Text)
}
