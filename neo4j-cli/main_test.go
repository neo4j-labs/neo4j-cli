// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/neo4j-cli/app"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommandSlug covers the slug derivation main.go feeds to tee.Save:
// CommandPath with the root name stripped and spaces turned into dashes, with a
// "root" fallback for parse failures / bare invocations.
func TestCommandSlug(t *testing.T) {
	fs, err := testfs.GetTestFs("{}", "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "nested leaf", args: []string{"aura", "instance", "list"}, want: "aura-instance-list"},
		{name: "single subcommand", args: []string{"config"}, want: "config"},
		{name: "unknown command falls back to root", args: []string{"definitely-not-a-command"}, want: "root"},
		{name: "no args falls back to root", args: []string{}, want: "root"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := app.NewCmd(cfg)
			assert.Equal(t, tc.want, commandSlug(cmd, tc.args))
		})
	}
}

// TestRecoverPanic_RedactsSecretArgs verifies that the panic-recover path in
// main() never leaks secret-flag values into stdout. The recoverPanic helper
// re-panics so callers complete normal panic flow; the test recovers from that
// inner re-panic and asserts the previously-captured stdout output.
func TestRecoverPanic_RedactsSecretArgs(t *testing.T) {
	const secret = "S3CR3T-DO-NOT-LOG"

	for _, tc := range []struct {
		name string
		args []string
		want string // flag name that must appear redacted
	}{
		{
			name: "client-secret is masked",
			args: []string{"aura", "credential", "add", "--client-secret", secret},
			want: "--client-secret",
		},
		{
			name: "password equals form is masked",
			args: []string{"query", "--password=" + secret},
			want: "--password=",
		},
		{
			name: "api-key is masked",
			args: []string{"embed", "add", "--api-key", secret},
			want: "--api-key",
		},
		{
			name: "instance-password is masked",
			args: []string{"aura", "dataapi", "graphql", "create", "--instance-password", secret},
			want: "--instance-password",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer

			func() {
				defer func() {
					// recoverPanic re-panics; swallow the re-raise so the test
					// can inspect the captured stdout.
					_ = recover()
				}()
				recoverPanic(&out, tc.args, "boom")
			}()

			got := out.String()
			assert.Contains(t, got, tc.want, "redacted output should keep the flag NAME")
			assert.Contains(t, got, "***", "redacted output should contain *** placeholder")
			assert.NotContains(t, got, secret, "redacted output must NOT contain the secret value")
			assert.True(t, strings.Contains(got, "Unexpected error running CLI with args"), "expected the standard prefix line")
		})
	}
}

// TestRecoverPanic_SurfacesPanicValue verifies the defence-in-depth hardening
// added in CLI-146: when the recovered value implements `error`, the panic's
// own message is printed on its own line BEFORE the existing fallback line,
// so unhandled status-code panics from `aura/internal/api/response.go` are
// diagnosable from CLI output alone. Non-error panic values keep the
// single-line behaviour.
func TestRecoverPanic_SurfacesPanicValue(t *testing.T) {
	const fallback = "Unexpected error running CLI with args"

	t.Run("error panic value writes diagnostic line BEFORE fallback line", func(t *testing.T) {
		var out bytes.Buffer
		args := []string{"aura", "instance", "create", "--rw"}
		panicErr := clierr.NewFatalError("unexpected error [status 402] running CLI with args [aura instance create --rw]")

		recoverPanic(&out, args, panicErr)

		got := out.String()
		diagIdx := strings.Index(got, panicErr.Error())
		fallbackIdx := strings.Index(got, fallback)
		require.GreaterOrEqual(t, diagIdx, 0, "diagnostic line must be present")
		require.GreaterOrEqual(t, fallbackIdx, 0, "fallback line must be present")
		assert.Less(t, diagIdx, fallbackIdx, "diagnostic line must precede fallback line")
		assert.Contains(t, got, panicErr.Error()+"\n", "diagnostic line must be terminated with a newline")
	})

	t.Run("non-error panic value writes only the fallback line", func(t *testing.T) {
		var out bytes.Buffer
		args := []string{"aura", "instance", "list"}

		recoverPanic(&out, args, "boom")

		got := out.String()
		assert.True(t, strings.HasPrefix(got, fallback), "non-error panic must NOT add a diagnostic line; got %q", got)
		assert.NotContains(t, got, "boom\n", "non-error panic value must not be written verbatim as a diagnostic line")
	})
}

// TestExitCodeFor verifies the helper used in main() to convert a returned
// error from cmd.Execute into a process exit code.
func TestExitCodeFor(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{name: "nil error returns 0", err: nil, want: 0},
		{name: "plain error returns 1", err: errors.New("boom"), want: 1},
		{name: "fatal CLIError returns 1", err: clierr.NewFatalError("boom"), want: 1},
		{name: "usage CLIError returns 2", err: clierr.NewUsageError("bad flag"), want: 2},
		{name: "not-found CLIError returns 3", err: clierr.NewNotFoundError("missing"), want: 3},
		{name: "auth CLIError returns 4", err: clierr.NewAuthError("no token"), want: 4},
		{name: "conflict CLIError returns 5", err: clierr.NewConflictError("nope"), want: 5},
		{name: "validation CLIError returns 6", err: clierr.NewValidationError("bad input"), want: 6},
		{name: "rate-limit CLIError returns 7", err: clierr.NewRateLimitError("30", "slow down"), want: 7},
		{name: "upstream CLIError returns 8", err: clierr.NewUpstreamError("5xx"), want: 8},
		{
			name: "wrapped CLIError surfaces typed code through errors.As",
			err:  fmt.Errorf("outer: %w", clierr.NewNotFoundError("inner")),
			want: 3,
		},
		{
			name: "doubly-wrapped CLIError still surfaces typed code",
			err:  fmt.Errorf("outer: %w", fmt.Errorf("middle: %w", clierr.NewAuthError("inner"))),
			want: 4,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, exitCodeFor(tc.err))
		})
	}
}

// TestRender_JSONMode exercises the json-format branch wired into main.go:
// stdout holds a parseable envelope, stderr the one-line summary.
func TestRender_JSONMode(t *testing.T) {
	var stdout, stderr bytes.Buffer

	clierr.Render(clierr.NewUsageError("bad flag"), &stdout, &stderr, "json")

	var env clierr.Envelope
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &env), "stdout must be valid JSON envelope")
	assert.Equal(t, "usage_error", env.Error.Code)
	assert.Equal(t, 2, env.Error.ExitCode)
	assert.Equal(t, "bad flag", env.Error.Message)
	assert.False(t, env.Error.Retryable)

	assert.Equal(t, "Error: bad flag (exit 2)\n", stderr.String())
}

// TestRender_PlaintextMode exercises the default branch: stdout untouched,
// stderr gets `Error: ... (exit N)` plus an optional suggestion line.
func TestRender_PlaintextMode(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := clierr.NewNotFoundError("instance missing").WithSuggestion("check the id")
	clierr.Render(err, &stdout, &stderr, "default")

	assert.Empty(t, stdout.String(), "plaintext mode must not write to stdout")
	assert.Equal(t, "Error: instance missing (exit 3)\ncheck the id\n", stderr.String())
}

// TestResolveFormatForRender covers the flag-parse-fallback used by main.go
// to pick a --format value when PersistentPreRunE never ran (e.g. cobra
// rejected a flag before format binding). When the bound value is already
// concrete the bound value is returned untouched.
func TestResolveFormatForRender(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		bound string
		want  string
	}{
		{
			name:  "bound concrete value passes through",
			args:  []string{"aura", "instance", "list", "--format=table"},
			bound: "json",
			want:  "json",
		},
		{
			name:  "default bound + --format=json peeks",
			args:  []string{"aura", "instance", "list", "--bad-flag", "--format=json"},
			bound: "default",
			want:  "json",
		},
		{
			name:  "default bound + space-separated --format peeks",
			args:  []string{"aura", "instance", "list", "--format", "toon", "--bad-flag"},
			bound: "default",
			want:  "toon",
		},
		{
			name:  "default bound + no --format stays default",
			args:  []string{"aura", "instance", "list", "--bad-flag"},
			bound: "default",
			want:  "default",
		},
		{
			name:  "default bound + invalid --format value stays default",
			args:  []string{"aura", "instance", "list", "--format=bogus"},
			bound: "default",
			want:  "default",
		},
		{
			name:  "empty bound + --format=json peeks",
			args:  []string{"--format=json", "anything"},
			bound: "",
			want:  "json",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveFormatForRender(tc.args, tc.bound))
		})
	}
}

// TestConfirmCancellation_EndToEnd exercises the integration of the confirm
// helper with the main.go chokepoint: drive a destructive leaf (`credential
// aura-client remove`) with TTY=true and empty stdin so the prompt cancels,
// then assert the bits the chokepoint relies on:
//   - cmd.Execute returns confirm.ErrCancelled (so the main.go check fires).
//   - exitCodeFor would map any leftover error to non-zero, but the main.go
//     chokepoint short-circuits before exitCodeFor, exiting 0.
//   - The helper wrote "cancelled." to stderr (no further output needed).
//   - SilenceErrors is set on the leaf so cobra does NOT prepend "Error: ".
//   - stdout is empty (no envelope, no narration).
func TestConfirmCancellation_EndToEnd(t *testing.T) {
	// Force TTY for the duration of this test so confirm.Require prompts.
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return true }))

	fs, err := testfs.GetTestFs("{}", `{
		"aura": {
			"credentials": [
				{"name": "work", "client-id": "id", "client-secret": "sec"}
			],
			"default-credential": "work"
		}
	}`)
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	cmd := app.NewCmd(cfg)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader("")) // empty stdin → immediate EOF → cancellation
	cmd.SetArgs([]string{"credential", "aura-client", "remove", "work", "--rw"})

	execErr := cmd.Execute()

	require.Error(t, execErr)
	require.True(t, errors.Is(execErr, confirm.ErrCancelled),
		"cmd.Execute must return confirm.ErrCancelled so main.go's chokepoint can exit 0; got %v", execErr)

	// main.go's chokepoint runs `os.Exit(0)` for this error class — verify the
	// exitCodeFor mapping would otherwise produce non-zero (proves the
	// chokepoint adds real behaviour rather than being a no-op).
	assert.NotEqual(t, 0, exitCodeFor(execErr), "without the chokepoint exitCodeFor would map ErrCancelled to non-zero")

	assert.Contains(t, stderr.String(), "cancelled.", "helper must narrate the cancellation to stderr")
	assert.NotContains(t, stderr.String(), "Error:", "cobra's default 'Error:' prefix must be suppressed via SilenceErrors")
	assert.Empty(t, stdout.String(), "stdout must stay empty on cancellation")

	// The credential must still be present — cancellation aborts before any
	// mutation.
	cred, err := cfg.Credentials.Aura.Get("work")
	require.NoError(t, err)
	assert.Equal(t, "id", cred.ClientId)
}
