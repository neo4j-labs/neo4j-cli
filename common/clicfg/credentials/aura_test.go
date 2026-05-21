// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"errors"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuraCredentials_GetDefault_NoDefault_AuthError locks the reclassification
// from task-004: AuraCredentials.GetDefault() is exclusively called from the
// Aura HTTP request path; "default credential not set" therefore means
// "auth missing" (exit 4), not a usage error.
func TestAuraCredentials_GetDefault_NoDefault_AuthError(t *testing.T) {
	c := &credentials.AuraCredentials{}

	cred, err := c.GetDefault()
	require.Error(t, err)
	assert.Nil(t, cred)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 4, ce.Code)
	assert.Contains(t, ce.Error(), "default credential not set")
}

// TestAuraCredentials_AddOrUpdateFromToken covers the three acceptance-criteria
// scenarios: adding a new credential, updating an existing credential in-place,
// and default-setting behaviour when DefaultCredential is empty vs non-empty.
func TestAuraCredentials_AddOrUpdateFromToken(t *testing.T) {
	const (
		expiresIn int64 = 3600
		// tolerance baked into AddOrUpdateFromToken
		toleranceSec = 60
	)

	newCreds := func() (*credentials.Credentials, error) {
		fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
		if err != nil {
			return nil, err
		}
		return credentials.NewCredentials(fs, clicfg.ConfigPrefix), nil
	}

	t.Run("add new credential — stored and returned without error", func(t *testing.T) {
		cfg, err := newCreds()
		require.NoError(t, err)

		before := time.Now().UnixMilli()
		addErr := cfg.Aura.AddOrUpdateFromToken("login", "cid-1", "tok-abc", expiresIn)
		after := time.Now().UnixMilli()

		require.NoError(t, addErr)
		require.Len(t, cfg.Aura.Credentials, 1)

		cred := cfg.Aura.Credentials[0]
		assert.Equal(t, "login", cred.Name)
		assert.Equal(t, "cid-1", cred.ClientId)
		assert.Equal(t, "", cred.ClientSecret, "ClientSecret must be left empty")
		assert.Equal(t, "tok-abc", cred.AccessToken)

		minExpiry := before + (expiresIn-toleranceSec)*1000
		maxExpiry := after + (expiresIn-toleranceSec)*1000
		assert.GreaterOrEqual(t, cred.TokenExpiry, minExpiry)
		assert.LessOrEqual(t, cred.TokenExpiry, maxExpiry)
	})

	t.Run("add new credential — becomes default when DefaultCredential is empty", func(t *testing.T) {
		cfg, err := newCreds()
		require.NoError(t, err)
		assert.Equal(t, "", cfg.Aura.DefaultCredential)

		require.NoError(t, cfg.Aura.AddOrUpdateFromToken("login", "cid-1", "tok-abc", expiresIn))
		assert.Equal(t, "login", cfg.Aura.DefaultCredential)
	})

	t.Run("add new credential — always overwrites existing default", func(t *testing.T) {
		cfg, err := newCreds()
		require.NoError(t, err)

		// Seed an existing credential that is the current default.
		require.NoError(t, cfg.Aura.Add("existing", "cid-0", "secret-0"))
		require.Equal(t, "existing", cfg.Aura.DefaultCredential)

		require.NoError(t, cfg.Aura.AddOrUpdateFromToken("login", "cid-1", "tok-abc", expiresIn))

		assert.Equal(t, "login", cfg.Aura.DefaultCredential, "login credential must always become the default")
		require.Len(t, cfg.Aura.Credentials, 2)
	})

	t.Run("update existing credential — default is set to the updated credential", func(t *testing.T) {
		cfg, err := newCreds()
		require.NoError(t, err)

		// Seed two credentials; make "other" the current default.
		require.NoError(t, cfg.Aura.Add("other", "cid-x", "secret-x"))
		require.NoError(t, cfg.Aura.Add("login", "cid-1", "secret-1"))
		require.NoError(t, cfg.Aura.SetDefault("other"))
		require.Equal(t, "other", cfg.Aura.DefaultCredential)

		require.NoError(t, cfg.Aura.AddOrUpdateFromToken("login", "cid-1", "tok-new", expiresIn))

		assert.Equal(t, "login", cfg.Aura.DefaultCredential, "updating 'login' must make it the default")
	})

	t.Run("update existing credential — AccessToken and TokenExpiry updated in-place", func(t *testing.T) {
		cfg, err := newCreds()
		require.NoError(t, err)

		// First call: creates the credential.
		require.NoError(t, cfg.Aura.AddOrUpdateFromToken("login", "cid-1", "tok-first", expiresIn))
		require.Len(t, cfg.Aura.Credentials, 1)

		// Second call: updates in-place.
		before := time.Now().UnixMilli()
		require.NoError(t, cfg.Aura.AddOrUpdateFromToken("login", "cid-1", "tok-second", expiresIn))
		after := time.Now().UnixMilli()

		require.Len(t, cfg.Aura.Credentials, 1, "no duplicate should be appended")
		cred := cfg.Aura.Credentials[0]
		assert.Equal(t, "tok-second", cred.AccessToken)

		minExpiry := before + (expiresIn-toleranceSec)*1000
		maxExpiry := after + (expiresIn-toleranceSec)*1000
		assert.GreaterOrEqual(t, cred.TokenExpiry, minExpiry)
		assert.LessOrEqual(t, cred.TokenExpiry, maxExpiry)
	})
}

// TestAuraCredentials_GetDefault_NoDefaultErrorBody locks the wording of the
// "default credential not set" error returned by AuraCredentials.GetDefault()
// when no default is configured. Asserts the substring invariants from the
// CLI-80 PRD (REQ-F-002 through REQ-F-005) so a future careless string edit
// fails locally rather than in user-facing copy.
func TestAuraCredentials_GetDefault_NoDefaultErrorBody(t *testing.T) {
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)
	cfg := credentials.NewCredentials(fs, clicfg.ConfigPrefix)

	cred, err := cfg.Aura.GetDefault()

	require.Error(t, err)
	assert.Nil(t, cred)

	msg := err.Error()
	assert.Contains(t, msg, "https://console.neo4j.io/account", "REQ-F-002: primary Aura Console minting URL")
	assert.Contains(t, msg, "https://neo4j.com/docs/aura/api/authentication/", "REQ-F-003: working docs URL")
	assert.Contains(t, msg, "credential aura-client add", "REQ-F-004: canonical shipped subcommand path")
	assert.NotContains(t, msg, "https://neo4j.com/docs/aura/classic/platform/api/authentication", "REQ-F-005: legacy broken URL absent")
}
