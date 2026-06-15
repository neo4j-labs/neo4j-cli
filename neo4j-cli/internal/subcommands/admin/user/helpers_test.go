// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonflags "github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
)

// fakeExecFn is the test-double type for adminutil.ExecFn. Tests construct a
// value of this type to control the rows/error returned by userExecFn.
type fakeExecFn func(ctx context.Context, cfg *clicfg.Config, conn *dbconn.Conn, cypher string, params map[string]any) ([]map[string]any, error)

// withFakeExecFn replaces userExecFn for the duration of t with fake and
// restores the original value in t.Cleanup.
func withFakeExecFn(t *testing.T, fake fakeExecFn) {
	t.Helper()
	orig := userExecFn
	userExecFn = adminutil.ExecFn(fake)
	t.Cleanup(func() { userExecFn = orig })
}

// withFakeStdinIsTTY replaces dbconn.StdinIsTTY for the duration of t with
// the supplied value and restores the original in t.Cleanup.
func withFakeStdinIsTTY(t *testing.T, isTTY bool) {
	t.Helper()
	orig := dbconn.StdinIsTTY
	dbconn.StdinIsTTY = func() bool { return isTTY }
	t.Cleanup(func() { dbconn.StdinIsTTY = orig })
}

// withFakePasswordReader replaces dbconn.PasswordReader for the duration of t
// with a function that returns pw (and no error) and restores the original in
// t.Cleanup.
func withFakePasswordReader(t *testing.T, pw string, err error) {
	t.Helper()
	orig := dbconn.PasswordReader
	dbconn.PasswordReader = func() (string, error) { return pw, err }
	t.Cleanup(func() { dbconn.PasswordReader = orig })
}

// testConn returns a *dbconn.Conn for use in tests. The connection params are
// never used because tests always override userExecFn with a fake.
func testConn() *dbconn.Conn {
	return &dbconn.Conn{
		URI:      "neo4j://localhost:7687",
		Username: "neo4j",
		Password: "test",
	}
}

// setFakeExecFn replaces userExecFn with a function that always returns the
// provided rows and error, and restores the original in t.Cleanup.
func setFakeExecFn(t *testing.T, rows []map[string]any, execErr error) {
	t.Helper()
	orig := userExecFn
	userExecFn = adminutil.ExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		return rows, execErr
	})
	t.Cleanup(func() { userExecFn = orig })
}

// setStdinIsTTY replaces dbconn.StdinIsTTY for the duration of t.
func setStdinIsTTY(t *testing.T, isTTY bool) {
	t.Helper()
	orig := dbconn.StdinIsTTY
	dbconn.StdinIsTTY = func() bool { return isTTY }
	t.Cleanup(func() { dbconn.StdinIsTTY = orig })
}

// setPasswordReader replaces dbconn.PasswordReader for the duration of t.
func setPasswordReader(t *testing.T, pw string, err error) {
	t.Helper()
	orig := dbconn.PasswordReader
	dbconn.PasswordReader = func() (string, error) { return pw, err }
	t.Cleanup(func() { dbconn.PasswordReader = orig })
}

// newTestCfg returns a clicfg.Config backed by a fresh in-memory filesystem.
func newTestCfg() *clicfg.Config {
	return clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
}

// fakeResponse bundles a rows slice and an error for sequential fake exec calls.
type fakeResponse struct {
	rows []map[string]any
	err  error
}

// ─── TestNormalizeUserRow ───────────────────────────────────────────────────

func TestNormalizeUserRow_BothNull_NormalizedToDefaults(t *testing.T) {
	row := map[string]any{
		"user":      "alice",
		"roles":     nil,
		"suspended": nil,
	}
	got := normalizeUserRow(row)
	assert.Equal(t, []any{}, got["roles"], "nil roles should become empty slice")
	assert.Equal(t, false, got["suspended"], "nil suspended should become false")
}

func TestNormalizeUserRow_BothNonNull_PassThrough(t *testing.T) {
	roles := []any{"admin", "reader"}
	row := map[string]any{
		"user":      "bob",
		"roles":     roles,
		"suspended": true,
	}
	got := normalizeUserRow(row)
	assert.Equal(t, roles, got["roles"], "non-nil roles should pass through unchanged")
	assert.Equal(t, true, got["suspended"], "non-nil suspended should pass through unchanged")
}

func TestNormalizeUserRow_NullRoles_NonNullSuspended(t *testing.T) {
	row := map[string]any{
		"user":      "carol",
		"roles":     nil,
		"suspended": true,
	}
	got := normalizeUserRow(row)
	assert.Equal(t, []any{}, got["roles"], "nil roles should become empty slice")
	assert.Equal(t, true, got["suspended"], "non-nil suspended should pass through unchanged")
}

func TestNormalizeUserRow_NonNullRoles_NullSuspended(t *testing.T) {
	roles := []any{"writer"}
	row := map[string]any{
		"user":      "dave",
		"roles":     roles,
		"suspended": nil,
	}
	got := normalizeUserRow(row)
	assert.Equal(t, roles, got["roles"], "non-nil roles should pass through unchanged")
	assert.Equal(t, false, got["suspended"], "nil suspended should become false")
}

// ─── TestOutputUser ────────────────────────────────────────────────────────

func TestOutputUser_ZeroRows_NoOutputNoError(t *testing.T) {
	setFakeExecFn(t, []map[string]any{}, nil)

	cfg := newTestCfg()
	conn := &dbconn.Conn{URI: "neo4j://localhost:7687", Username: "neo4j", Password: "test"}
	cmd := &cobra.Command{}
	commonflags.RegisterOutputFlag(cmd, cfg)
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(bytes.NewBuffer(nil))

	err := outputUser(cmd, cfg, conn, "alice")
	require.NoError(t, err)
	assert.Empty(t, out.String(), "no output expected for zero rows")
}

func TestOutputUser_OneRow_PrintsUser(t *testing.T) {
	rows := []map[string]any{
		{"user": "alice", "roles": []any{"admin"}, "passwordChangeRequired": false, "suspended": false},
	}
	setFakeExecFn(t, rows, nil)

	cfg := newTestCfg()
	conn := &dbconn.Conn{URI: "neo4j://localhost:7687", Username: "neo4j", Password: "test"}
	cmd := &cobra.Command{}
	commonflags.RegisterOutputFlag(cmd, cfg)
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"--format", "json"})
	require.NoError(t, cmd.Execute())

	err := outputUser(cmd, cfg, conn, "alice")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "alice", "output should contain the username")
}

func TestOutputUser_ExecError_ReturnsError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt unreachable")
	setFakeExecFn(t, nil, execErr)

	cfg := newTestCfg()
	conn := &dbconn.Conn{URI: "neo4j://localhost:7687", Username: "neo4j", Password: "test"}
	cmd := &cobra.Command{}
	commonflags.RegisterOutputFlag(cmd, cfg)
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))

	err := outputUser(cmd, cfg, conn, "alice")
	require.Error(t, err)
	assert.Equal(t, execErr, err)
}

// ─── TestPromptUserPassword ────────────────────────────────────────────────

// newCmdWithPasswordFlag builds a minimal *cobra.Command with flagName registered.
func newCmdWithPasswordFlag(flagName string) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String(flagName, "", "password flag")
	return cmd
}

func TestPromptUserPassword_ExplicitValue_ReturnedImmediately(t *testing.T) {
	// StdinIsTTY / PasswordReader must NOT be called — don't override them.
	cmd := newCmdWithPasswordFlag("set-password")
	require.NoError(t, cmd.Flags().Set("set-password", "mypassword"))

	got, err := promptUserPassword(cmd, "set-password")
	require.NoError(t, err)
	assert.Equal(t, "mypassword", got)
}

func TestPromptUserPassword_TTY_PromptsPrintsToStderr(t *testing.T) {
	setStdinIsTTY(t, true)
	setPasswordReader(t, "pw", nil)

	cmd := newCmdWithPasswordFlag("set-password")
	errBuf := bytes.NewBuffer(nil)
	cmd.SetErr(errBuf)

	got, err := promptUserPassword(cmd, "set-password")
	require.NoError(t, err)
	assert.Equal(t, "pw", got)
	assert.Contains(t, errBuf.String(), "Password: ", "should print prompt to stderr")
}

func TestPromptUserPassword_NonTTY_ReturnsUsageError(t *testing.T) {
	setStdinIsTTY(t, false)

	cmd := newCmdWithPasswordFlag("set-password")
	cmd.SetErr(bytes.NewBuffer(nil))

	_, err := promptUserPassword(cmd, "set-password")
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError")
	assert.Equal(t, 2, ce.Code, "usage error should have exit code 2")
	assert.Contains(t, ce.Message, "set-password")
}

func TestPromptUserPassword_TTY_ReaderError_WrappedError(t *testing.T) {
	setStdinIsTTY(t, true)
	readerErr := errors.New("reader interrupted")
	setPasswordReader(t, "", readerErr)

	cmd := newCmdWithPasswordFlag("set-password")
	cmd.SetErr(bytes.NewBuffer(nil))

	_, err := promptUserPassword(cmd, "set-password")
	require.Error(t, err)
	assert.ErrorIs(t, err, readerErr, "reader error should be wrapped in returned error")
}
