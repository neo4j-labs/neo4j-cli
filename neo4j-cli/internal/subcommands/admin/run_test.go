// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package admin

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
)

// fakeQueryRunner is the test double for queryRunner. It returns the
// configured rows or error without touching a real Bolt connection.
type fakeQueryRunner struct {
	rows []map[string]any
	err  error
}

func (f *fakeQueryRunner) run(_ context.Context, _ *credentials.DbmsCredential, _ string, _ map[string]any) ([]map[string]any, error) {
	return f.rows, f.err
}

// withFakeRunner replaces adminRunnerFn for the duration of t and restores it
// after. The supplied fakeQueryRunner is returned on every call.
func withFakeRunner(t *testing.T, fake *fakeQueryRunner) {
	t.Helper()
	orig := adminRunnerFn
	adminRunnerFn = func(_ *clicfg.Config) queryRunner { return fake }
	t.Cleanup(func() { adminRunnerFn = orig })
}

func newTestCred() *credentials.DbmsCredential {
	return &credentials.DbmsCredential{
		Name:         "test",
		URI:          "neo4j://localhost:7687",
		Username:     "neo4j",
		Password:     "password",
		DatabaseName: "neo4j",
	}
}

func newTestCfg() *clicfg.Config {
	return &clicfg.Config{Version: "test"}
}

func TestRunAdminStatement_HappyPath(t *testing.T) {
	expected := []map[string]any{{"name": "neo4j"}}
	fake := &fakeQueryRunner{rows: expected}
	withFakeRunner(t, fake)

	rows, err := runAdminStatement(context.Background(), newTestCfg(), newTestCred(), "SHOW DATABASES", nil)
	require.NoError(t, err)
	assert.Equal(t, expected, rows)
}

func TestRunAdminStatement_NoRows(t *testing.T) {
	fake := &fakeQueryRunner{rows: []map[string]any{}}
	withFakeRunner(t, fake)

	rows, err := runAdminStatement(context.Background(), newTestCfg(), newTestCred(), "CREATE DATABASE foo IF NOT EXISTS", nil)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestTranslateAdminError_UnsupportedAdmin_EnterpriseHint(t *testing.T) {
	fake := &fakeQueryRunner{err: fmt.Errorf("Neo.ClientError.Statement.UnsupportedAdministrationCommand (CREATE DATABASE is not supported)")}
	withFakeRunner(t, fake)

	_, err := runAdminStatement(context.Background(), newTestCfg(), newTestCred(), "CREATE DATABASE foo", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code) // validation_error
	assert.Contains(t, ce.Message, "requires Enterprise edition")
}

func TestTranslateAdminError_UnsupportedAdmin_AuraHint(t *testing.T) {
	fake := &fakeQueryRunner{err: fmt.Errorf("Neo.ClientError.Statement.UnsupportedAdministrationCommand (not supported, for more info see https://support.neo4j.com/kb/article)")}
	withFakeRunner(t, fake)

	_, err := runAdminStatement(context.Background(), newTestCfg(), newTestCred(), "CREATE DATABASE foo", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code)
	assert.Contains(t, ce.Message, "not supported on Aura — use the Aura Console or API")
}

func TestTranslateAdminError_ArgumentError_NonNativeAuth(t *testing.T) {
	for _, msgFragment := range []string{"non-native", "authentication provider apart from native"} {
		t.Run(msgFragment, func(t *testing.T) {
			fake := &fakeQueryRunner{err: fmt.Errorf("Neo.ClientError.Statement.ArgumentError (%s)", msgFragment)}
			withFakeRunner(t, fake)

			_, err := runAdminStatement(context.Background(), newTestCfg(), newTestCred(), "RENAME USER old TO new", nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, 6, ce.Code)
			assert.Equal(t, "renaming users is not supported on Aura connections (Aura uses a non-native authentication provider)", ce.Message)
		})
	}
}

func TestTranslateAdminError_SetStatus_CommunityEdition(t *testing.T) {
	fake := &fakeQueryRunner{err: fmt.Errorf("'SET STATUS' is not available in community edition")}
	withFakeRunner(t, fake)

	_, err := runAdminStatement(context.Background(), newTestCfg(), newTestCred(), "ALTER USER neo4j SET STATUS SUSPENDED", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code)
	assert.Contains(t, ce.Message, "SET STATUS")
	assert.Contains(t, ce.Message, "community edition")
}

func TestTranslateAdminError_HomeDatabase_CommunityEdition(t *testing.T) {
	fake := &fakeQueryRunner{err: fmt.Errorf("'HOME DATABASE' is not available in community edition")}
	withFakeRunner(t, fake)

	_, err := runAdminStatement(context.Background(), newTestCfg(), newTestCred(), "ALTER USER neo4j SET HOME DATABASE foo", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code)
	assert.Contains(t, ce.Message, "HOME DATABASE")
	assert.Contains(t, ce.Message, "community edition")
}

func TestTranslateAdminError_AlreadyCLIError_PassThrough(t *testing.T) {
	original := clierr.NewNotFoundError("something not found")
	fake := &fakeQueryRunner{err: original}
	withFakeRunner(t, fake)

	_, err := runAdminStatement(context.Background(), newTestCfg(), newTestCred(), "SHOW DATABASES", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code) // not_found — original code preserved
}

func TestTranslateAdminError_GenericError_MappedToValidation(t *testing.T) {
	fake := &fakeQueryRunner{err: errors.New("something unexpected")}
	withFakeRunner(t, fake)

	_, err := runAdminStatement(context.Background(), newTestCfg(), newTestCred(), "SHOW DATABASES", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code) // validation_error
}
