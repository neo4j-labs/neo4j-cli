// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"path/filepath"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCredentials_RecoversFromCorruptFile verifies that a malformed
// credentials.json no longer panics with a raw json error. Instead the
// file is backed up to credentials.json.corrupt-<unix-ts>, the in-memory
// state is reset to empty credentials, the on-disk file is rewritten as
// an empty JSON document, and a clierr.FatalError naming the backup path
// is surfaced (via panic) on the failing invocation. A subsequent
// invocation succeeds with empty credentials.
func TestNewCredentials_RecoversFromCorruptFile(t *testing.T) {
	// 1-byte garbage file (json.Unmarshal rejects it).
	const garbage = "x"

	fs, err := testfs.GetTestFs("{}", garbage)
	require.NoError(t, err)

	credsPath := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json")

	// Pin nowUnix so we know the exact backup path.
	const fixedTs = int64(1700000000)
	credentials.SetNowUnixForTest(t, func() int64 { return fixedTs })

	// First invocation: must panic with a clierr.FatalError-shaped error
	// that names the backup path. The recover branch in the caller (main)
	// would print and exit; here we recover ourselves.
	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		_ = credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	}()

	require.NotNil(t, recovered, "expected NewCredentials to panic on corrupt file")
	panicErr, ok := recovered.(error)
	require.True(t, ok, "panic value must be an error, got %T", recovered)

	// Backup file must exist with the original (pre-reset) contents.
	expectedBackupPath := credsPath + ".corrupt-1700000000"
	assert.Contains(t, panicErr.Error(), expectedBackupPath, "fatal-error message must name the backup file path")
	assert.Contains(t, panicErr.Error(), "corrupt", "fatal-error message must mention corruption")

	backup, readErr := afero.ReadFile(fs, expectedBackupPath)
	require.NoError(t, readErr, "backup file must exist on disk")
	assert.Equal(t, garbage, string(backup), "backup file must contain the original (corrupt) bytes")

	// Second invocation: must succeed with empty credentials.
	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NotNil(t, creds.Aura)
	require.NotNil(t, creds.Dbms)
	require.NotNil(t, creds.Embed)
	assert.Empty(t, creds.Aura.Credentials, "aura credentials should be empty after recovery")
	assert.Empty(t, creds.Dbms.Credentials, "dbms credentials should be empty after recovery")
	assert.Empty(t, creds.Embed.Credentials, "embed credentials should be empty after recovery")
}
