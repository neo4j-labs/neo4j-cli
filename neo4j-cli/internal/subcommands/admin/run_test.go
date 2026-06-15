// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clierr"
)

func TestRunAdminStatement_HappyPath(t *testing.T) {
	expected := []map[string]any{{"name": "neo4j"}}
	fake := &fakeQueryRunner{rows: expected}
	withFakeRunner(t, fake)

	rows, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "SHOW DATABASES", nil)
	require.NoError(t, err)
	assert.Equal(t, expected, rows)
}

func TestRunAdminStatement_NoRows(t *testing.T) {
	fake := &fakeQueryRunner{rows: []map[string]any{}}
	withFakeRunner(t, fake)

	rows, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "CREATE DATABASE foo IF NOT EXISTS", nil)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestTranslateAdminError_UnsupportedAdmin_EnterpriseHint(t *testing.T) {
	fake := &fakeQueryRunner{err: &neo4j.Neo4jError{Code: "Neo.ClientError.Statement.UnsupportedAdministrationCommand", Msg: "CREATE DATABASE is not supported"}}
	withFakeRunner(t, fake)

	_, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "CREATE DATABASE foo", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code) // validation_error
	assert.Contains(t, ce.Message, "requires Enterprise edition")
}

func TestTranslateAdminError_UnsupportedAdmin_AuraHint(t *testing.T) {
	fake := &fakeQueryRunner{err: &neo4j.Neo4jError{Code: "Neo.ClientError.Statement.UnsupportedAdministrationCommand", Msg: "not supported, for more info see https://support.neo4j.com/kb/article"}}
	withFakeRunner(t, fake)

	_, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "CREATE DATABASE foo", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code)
	assert.Contains(t, ce.Message, "not supported on Aura — use the Aura Console or API")
}

func TestTranslateAdminError_ArgumentError_NonNativeAuth(t *testing.T) {
	for _, msgFragment := range []string{"non-native", "authentication provider apart from native"} {
		t.Run(msgFragment, func(t *testing.T) {
			fake := &fakeQueryRunner{err: &neo4j.Neo4jError{Code: "Neo.ClientError.Statement.ArgumentError", Msg: msgFragment}}
			withFakeRunner(t, fake)

			_, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "RENAME USER old TO new", nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, 6, ce.Code)
			assert.Equal(t, "renaming users is not supported on Aura connections (Aura uses a non-native authentication provider)", ce.Message)
		})
	}
}

func TestTranslateAdminError_SetStatus_CommunityEdition(t *testing.T) {
	fake := &fakeQueryRunner{err: &neo4j.Neo4jError{Code: "Neo.ClientError.General.UnknownError", Msg: "'SET STATUS' is not available in community edition"}}
	withFakeRunner(t, fake)

	_, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "ALTER USER neo4j SET STATUS SUSPENDED", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code)
	assert.Contains(t, ce.Message, "SET STATUS")
	assert.Contains(t, ce.Message, "community edition")
}

func TestTranslateAdminError_HomeDatabase_CommunityEdition(t *testing.T) {
	fake := &fakeQueryRunner{err: &neo4j.Neo4jError{Code: "Neo.ClientError.General.UnknownError", Msg: "'HOME DATABASE' is not available in community edition"}}
	withFakeRunner(t, fake)

	_, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "ALTER USER neo4j SET HOME DATABASE foo", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code)
	assert.Contains(t, ce.Message, "HOME DATABASE")
	assert.Contains(t, ce.Message, "community edition")
}

func TestTranslateAdminError_ExecutionFailed_CommunityEdition(t *testing.T) {
	fake := &fakeQueryRunner{err: &neo4j.Neo4jError{
		Code: "Neo.DatabaseError.Statement.ExecutionFailed",
		Msg:  "Failed to alter the specified user 'x': 'SET STATUS' is not available in community edition.",
	}}
	withFakeRunner(t, fake)

	_, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "ALTER USER x SET STATUS SUSPENDED", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code)
	assert.False(t, ce.Retryable)
	assert.Contains(t, ce.Message, "not available in community edition")
}

func TestTranslateAdminError_ExecutionFailed_NonCommunity_MappedToUpstream(t *testing.T) {
	fake := &fakeQueryRunner{err: &neo4j.Neo4jError{
		Code: "Neo.DatabaseError.Statement.ExecutionFailed",
		Msg:  "Some unexpected server-side execution failure unrelated to edition.",
	}}
	withFakeRunner(t, fake)

	_, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "SHOW DATABASES", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 8, ce.Code) // upstream_error
	assert.True(t, ce.Retryable)
}

func TestTranslateAdminError_SyntaxError_UnsupportedCypherVersion(t *testing.T) {
	for _, msg := range []string{
		"Invalid input 'CYPHER': expected ...",
		"Cypher version not supported",
	} {
		t.Run(msg, func(t *testing.T) {
			fake := &fakeQueryRunner{err: &neo4j.Neo4jError{
				Code: "Neo.ClientError.Statement.SyntaxError",
				Msg:  msg,
			}}
			withFakeRunner(t, fake)

			_, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "SHOW DATABASES", nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, 6, ce.Code) // validation_error
			assert.Contains(t, ce.Message, "admin commands require Neo4j 2025.x or later")
		})
	}
}

func TestTranslateAdminError_SyntaxError_Generic_MappedToValidation(t *testing.T) {
	fake := &fakeQueryRunner{err: &neo4j.Neo4jError{
		Code: "Neo.ClientError.Statement.SyntaxError",
		Msg:  "Invalid input 'WRONG': expected something else",
	}}
	withFakeRunner(t, fake)

	_, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "INVALID CYPHER", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code)                 // validation_error
	assert.NotContains(t, ce.Message, "2025.x") // generic syntax error, not version error
}

func TestTranslateAdminError_Forbidden_InsufficientPrivileges(t *testing.T) {
	fake := &fakeQueryRunner{err: &neo4j.Neo4jError{Code: "Neo.ClientError.Security.Forbidden", Msg: "Create user is not allowed for user 'readonly' with roles [public]."}}
	withFakeRunner(t, fake)

	_, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "CREATE USER alice SET PASSWORD 'pw' SET PASSWORD CHANGE REQUIRED", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code) // validation_error
	assert.Equal(t, "insufficient privileges: the connected user does not have permission to manage users (requires admin role)", ce.Message)
}

func TestTranslateAdminError_AlreadyCLIError_PassThrough(t *testing.T) {
	original := clierr.NewNotFoundError("something not found")
	fake := &fakeQueryRunner{err: original}
	withFakeRunner(t, fake)

	_, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "SHOW DATABASES", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code) // not_found — original code preserved
}

func TestTranslateAdminError_GenericError_MappedToValidation(t *testing.T) {
	fake := &fakeQueryRunner{err: errors.New("something unexpected")}
	withFakeRunner(t, fake)

	_, err := RunAdminStatement(context.Background(), newTestCfg(), newTestConn(), "SHOW DATABASES", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code) // validation_error
}

func TestRedactParams(t *testing.T) {
	tests := []struct {
		key      string
		value    any
		redacted bool
	}{
		{key: "password", value: "secret123", redacted: true},
		{key: "Password", value: "secret123", redacted: true},
		{key: "user_password", value: "secret123", redacted: true},
		{key: "passwd", value: "secret123", redacted: true},
		{key: "PASSWD", value: "secret123", redacted: true},
		{key: "pwd", value: "secret123", redacted: true},
		{key: "myPwd", value: "secret123", redacted: true},
		{key: "secret", value: "topsecret", redacted: true},
		{key: "mySecret", value: "topsecret", redacted: true},
		{key: "token", value: "tok123", redacted: true},
		{key: "accessToken", value: "tok123", redacted: true},
		{key: "key", value: "keyval", redacted: true},
		{key: "apiKey", value: "keyval", redacted: true},
		{key: "credential", value: "cred", redacted: true},
		{key: "myCredential", value: "cred", redacted: true},
		{key: "name", value: "alice", redacted: false},
		{key: "role", value: "admin", redacted: false},
		{key: "database", value: "neo4j", redacted: false},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			params := map[string]any{tc.key: tc.value}
			result := redactParams(params)
			if tc.redacted {
				assert.Equal(t, "***", result[tc.key], "expected value for key %q to be redacted", tc.key)
			} else {
				assert.Equal(t, tc.value, result[tc.key], "expected value for key %q to pass through unchanged", tc.key)
			}
		})
	}
}

func TestRedactParams_EmptyMap(t *testing.T) {
	result := redactParams(map[string]any{})
	assert.Empty(t, result)
}

func TestRedactParams_NilMap(t *testing.T) {
	result := redactParams(nil)
	assert.Nil(t, result)
}
