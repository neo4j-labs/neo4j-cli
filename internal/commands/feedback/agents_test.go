// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"context"
	"errors"
	"github.com/neo4j/cli/internal/output"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain keeps every test away from a real `claude` binary: without it the
// enable and disable tests would change the developer's Claude Code config.
func TestMain(m *testing.M) {
	// Output format resolution must not depend on the developer's terminal or
	// on running inside an agent (CLAUDECODE): no explicit --format means json.
	output.IsAgent = func() bool { return false }
	output.StdoutIsTerminal = func() bool { return false }
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	execClaude = func(context.Context, ...string) error { return errors.New("execClaude not stubbed") }
	os.Exit(m.Run())
}

// fakeClaude installs a recording `claude` and returns the calls it receives.
// registered controls the result of `mcp get`; failAdd makes `mcp add` fail.
func fakeClaude(t *testing.T, registered, failAdd bool) *[]string {
	t.Helper()
	var calls []string
	prevLook, prevExec, prevPath := lookPath, execClaude, executablePath
	t.Cleanup(func() { lookPath, execClaude, executablePath = prevLook, prevExec, prevPath })
	lookPath = func(string) (string, error) { return "/usr/bin/claude", nil }
	executablePath = func() (string, error) { return "/opt/neo4j-cli", nil }
	execClaude = func(_ context.Context, args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		switch {
		case args[1] == "get":
			if registered {
				return nil
			}
			return errors.New("not found")
		case args[1] == "add" && failAdd:
			return errors.New("boom")
		}
		return nil
	}
	return &calls
}

func TestRegisterClaudeCode_AddsAtUserScopeReplacingAnyOldEntry(t *testing.T) {
	calls := fakeClaude(t, false, false)
	agents, notes := registerClaudeCode(context.Background(), "basic")
	assert.Equal(t, []string{"claude-code"}, agents)
	assert.Empty(t, notes)
	assert.Equal(t, []string{
		"mcp remove -s user neo4j-feedback",
		"mcp add -s user neo4j-feedback -e NEO4J_CLI_FEEDBACK_INSTRUCTIONS=basic -e NEO4J_CLI_FLAG_FEEDBACK_COMMAND=true -- /opt/neo4j-cli feedback serve",
	}, *calls)
}

func TestRegisterClaudeCode_NoClaudeBinaryPrintsManualCommand(t *testing.T) {
	agents, notes := registerClaudeCode(context.Background(), "basic")
	assert.Empty(t, agents)
	assert.NotNil(t, agents, "an empty list, not null, in the JSON")
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "claude mcp add -s user neo4j-feedback -e NEO4J_CLI_FEEDBACK_INSTRUCTIONS=basic -e NEO4J_CLI_FLAG_FEEDBACK_COMMAND=true --")
	assert.Contains(t, notes[0], "feedback serve")
}

func TestRegisterClaudeCode_AddFailureIsReportedNotFatal(t *testing.T) {
	fakeClaude(t, false, true)
	agents, notes := registerClaudeCode(context.Background(), "basic")
	assert.Empty(t, agents)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "registration failed")
}

func TestUnregisterClaudeCode(t *testing.T) {
	t.Run("removes when registered", func(t *testing.T) {
		calls := fakeClaude(t, true, false)
		assert.Empty(t, unregisterClaudeCode(context.Background()))
		assert.Equal(t, []string{"mcp get neo4j-feedback", "mcp remove -s user neo4j-feedback"}, *calls)
	})
	t.Run("does nothing when not registered", func(t *testing.T) {
		calls := fakeClaude(t, false, false)
		assert.Empty(t, unregisterClaudeCode(context.Background()))
		assert.Equal(t, []string{"mcp get neo4j-feedback"}, *calls)
	})
	t.Run("no claude binary is not an error", func(t *testing.T) {
		assert.Empty(t, unregisterClaudeCode(context.Background()))
	})
}

func TestEnableDisableStatus_Registration(t *testing.T) {
	withSeams(t, false, "")
	calls := fakeClaude(t, true, false)
	cfg := newTestConfig(t)

	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--endpoint", loopback)
	require.NoError(t, err)
	assert.Equal(t, []any{"claude-code"}, parseJSON(t, stdout)["registered_agents"])
	assert.Contains(t, *calls, "mcp add -s user neo4j-feedback -e NEO4J_CLI_FEEDBACK_INSTRUCTIONS=basic -e NEO4J_CLI_FLAG_FEEDBACK_COMMAND=true -- /opt/neo4j-cli feedback serve")

	stdout, _, err = runCmd(t, newTestApp(cfg), "feedback", "status")
	require.NoError(t, err)
	assert.Equal(t, []any{"claude-code"}, parseJSON(t, stdout)["registered_agents"])

	_, _, err = runCmd(t, newTestApp(cfg), "feedback", "disable")
	require.NoError(t, err)
	assert.Contains(t, *calls, "mcp remove -s user neo4j-feedback")
}

func TestEnable_RegisterFalseSkipsClaude(t *testing.T) {
	withSeams(t, false, "")
	calls := fakeClaude(t, false, false)
	cfg := newTestConfig(t)
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--endpoint", loopback, "--register=false")
	require.NoError(t, err)
	assert.Empty(t, *calls)
	assert.Equal(t, []any{}, parseJSON(t, stdout)["registered_agents"])
}

func TestServerInstructions(t *testing.T) {
	assert.Equal(t, instructionsBasic, serverInstructions(""), "default is basic")
	assert.Equal(t, instructionsBasic, serverInstructions("basic"))
	assert.Equal(t, instructionsBasic, serverInstructions("something-else"))
	assert.Equal(t, instructionsOptIn, serverInstructions("optin"))
	assert.Empty(t, serverInstructions("none"))
	assert.Contains(t, instructionsOptIn, instructionsBasic)
	assert.False(t, validInstructionsVariant("show"), "the show variants were removed")
	assert.Equal(t, instructionsBasic, serverInstructions("show"), "an unknown variant falls back to basic")
	assert.NotContains(t, instructionsBasic, "show the user", "basic stays as tested")
	for _, v := range []string{instructionsBasic, instructionsOptIn} {
		assert.Contains(t, v, feedbackToolName)
		assert.Contains(t, v, "Never include queries")
	}
}

func TestEnable_InstructionsVariant(t *testing.T) {
	withSeams(t, false, "")
	calls := fakeClaude(t, true, false)
	cfg := newTestConfig(t)

	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--endpoint", loopback, "--instructions", "optin")
	require.NoError(t, err)
	assert.Equal(t, "optin", parseJSON(t, stdout)["instructions"])
	assert.Contains(t, *calls, "mcp add -s user neo4j-feedback -e NEO4J_CLI_FEEDBACK_INSTRUCTIONS=optin -e NEO4J_CLI_FLAG_FEEDBACK_COMMAND=true -- /opt/neo4j-cli feedback serve")

	_, _, err = runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--endpoint", loopback, "--instructions", "loud")
	require.Error(t, err)
	assert.Equal(t, 2, exitCode(err))
}

func TestEnable_DefaultsToBasicInstructions(t *testing.T) {
	withSeams(t, false, "")
	calls := fakeClaude(t, true, false)
	cfg := newTestConfig(t)
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--endpoint", loopback)
	require.NoError(t, err)
	assert.Equal(t, "basic", parseJSON(t, stdout)["instructions"])
	assert.Contains(t, *calls, "mcp add -s user neo4j-feedback -e NEO4J_CLI_FEEDBACK_INSTRUCTIONS=basic -e NEO4J_CLI_FLAG_FEEDBACK_COMMAND=true -- /opt/neo4j-cli feedback serve")
}

func TestEnable_EndpointFromEnv(t *testing.T) {
	withSeams(t, false, "")
	prev := lookupEnv
	t.Cleanup(func() { lookupEnv = prev })
	lookupEnv = func(k string) string {
		if k == endpointEnv {
			return loopback
		}
		return ""
	}
	cfg := newTestConfig(t)
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw")
	require.Error(t, err) // no --accept: prints the disclosure, exits 2
	assert.Equal(t, 2, exitCode(err))
	assert.Contains(t, stdout, loopback)

	stdout, _, err = runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept")
	require.NoError(t, err)
	assert.Equal(t, loopback, parseJSON(t, stdout)["endpoint"])
}

// fakeCodex installs a recording `codex`; registered controls `mcp get`.
func fakeCodex(t *testing.T, registered, failAdd bool) *[]string {
	t.Helper()
	var calls []string
	prevLook, prevExec, prevPath := lookPath, execCodex, executablePath
	t.Cleanup(func() { lookPath, execCodex, executablePath = prevLook, prevExec, prevPath })
	prev := lookPath
	lookPath = func(name string) (string, error) {
		if name == codexBinary {
			return "/usr/bin/codex", nil
		}
		return prev(name)
	}
	executablePath = func() (string, error) { return "/opt/neo4j-cli", nil }
	execCodex = func(_ context.Context, args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		switch {
		case args[1] == "get":
			if registered {
				return nil
			}
			return errors.New("not found")
		case args[1] == "add" && failAdd:
			return errors.New("boom")
		}
		return nil
	}
	return &calls
}

func TestRegisterCodex_AddsWithExplicitEnvReplacingAnyOldEntry(t *testing.T) {
	calls := fakeCodex(t, false, false)
	agents, notes := registerCodex(context.Background(), "basic")
	assert.Equal(t, []string{"codex"}, agents)
	assert.Empty(t, notes)
	assert.Equal(t, []string{
		"mcp remove neo4j-feedback",
		"mcp add neo4j-feedback --env NEO4J_CLI_FEEDBACK_INSTRUCTIONS=basic --env NEO4J_CLI_FLAG_FEEDBACK_COMMAND=true -- /opt/neo4j-cli feedback serve",
	}, *calls)
}

func TestRegisterCodex_FailureIsReportedWithManualCommand(t *testing.T) {
	fakeCodex(t, false, true)
	agents, notes := registerCodex(context.Background(), "basic")
	assert.Empty(t, agents)
	assert.NotNil(t, agents)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "Codex registration failed")
	assert.Contains(t, notes[0], "codex mcp add neo4j-feedback --env")
}

func TestRegisterAgents_BothInstalled(t *testing.T) {
	cc := fakeClaude(t, false, false)
	cx := fakeCodex(t, false, false)
	agents, notes := registerAgents(context.Background(), "basic")
	assert.Equal(t, []string{"claude-code", "codex"}, agents)
	assert.Empty(t, notes)
	assert.Equal(t, 1, added(*cc))
	assert.Len(t, *cx, 2)
}

func TestRegisterAgents_OnlyCodexInstalledIsSilentAboutClaude(t *testing.T) {
	cx := fakeCodex(t, false, false)
	agents, notes := registerAgents(context.Background(), "basic")
	assert.Equal(t, []string{"codex"}, agents)
	assert.Empty(t, notes, "a missing agent is not an error")
	assert.Len(t, *cx, 2)
}

func TestRegisterAgents_NoneInstalledExplainsBothManualCommands(t *testing.T) {
	agents, notes := registerAgents(context.Background(), "basic")
	assert.Empty(t, agents)
	assert.NotNil(t, agents)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "claude mcp add -s user neo4j-feedback")
	assert.Contains(t, notes[0], "codex mcp add neo4j-feedback")
}

func TestStatusDisableAndRegister_IncludeCodex(t *testing.T) {
	withSeams(t, false, "")
	cc := fakeClaude(t, true, false)
	cx := fakeCodex(t, true, false)
	cfg := newTestConfig(t)

	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "register", "--rw", "--endpoint", loopback)
	require.NoError(t, err)
	assert.Equal(t, []any{"claude-code", "codex"}, parseJSON(t, stdout)["registered_agents"])

	stdout, _, err = runCmd(t, newTestApp(cfg), "feedback", "status")
	require.NoError(t, err)
	assert.Equal(t, []any{"claude-code", "codex"}, parseJSON(t, stdout)["registered_agents"])

	_, _, err = runCmd(t, newTestApp(cfg), "feedback", "disable")
	require.NoError(t, err)
	assert.Contains(t, *cc, "mcp remove -s user neo4j-feedback")
	assert.Contains(t, *cx, "mcp remove neo4j-feedback")
}

func TestSelfPath_PrefersTheStablePathOnPATHOverTheVersionedTarget(t *testing.T) {
	dir := t.TempDir()
	cellar := filepath.Join(dir, "Cellar", "neo4j-cli", "1.0", "bin")
	require.NoError(t, os.MkdirAll(cellar, 0o755))
	real := filepath.Join(cellar, "neo4j-cli")
	require.NoError(t, os.WriteFile(real, []byte("x"), 0o755))
	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	stable := filepath.Join(binDir, "neo4j-cli")
	require.NoError(t, os.Symlink(real, stable))

	prevExe, prevLook := executablePath, lookPath
	t.Cleanup(func() { executablePath, lookPath = prevExe, prevLook })
	executablePath = func() (string, error) { return stable, nil }
	lookPath = func(string) (string, error) { return stable, nil }
	want, _ := filepath.EvalSymlinks(stable)
	assert.Equal(t, stable, selfPath(), "the symlink on PATH, not the versioned target %s", want)

	// A different binary on the PATH is ignored.
	other := filepath.Join(binDir, "other")
	require.NoError(t, os.WriteFile(other, []byte("y"), 0o755))
	lookPath = func(string) (string, error) { return other, nil }
	got := selfPath()
	assert.Equal(t, want, got)

	// Not on the PATH at all: the resolved real path.
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	assert.Equal(t, want, selfPath())
}

func TestRegisterClaudeCode_FailedReplaceSaysTheOldEntryIsGone(t *testing.T) {
	fakeClaude(t, true, true) // an entry exists, and adding the new one fails
	agents, notes := registerClaudeCode(context.Background(), "basic")
	assert.Empty(t, agents)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "registration failed")
	assert.Contains(t, notes[0], "previous registration was removed")
}
