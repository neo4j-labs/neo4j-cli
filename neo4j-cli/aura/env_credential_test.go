// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runEnvCredentialResolution mounts the aura tree under a stand-in neo4j-cli root
// with EnableTraverseRunHooks=true (mirroring the shipped surface) so the
// aura-root PersistentPreRunE runs. It replaces the leaf RunE to capture the
// active credential after all hooks have run, returning that credential, the
// in-memory FS, and any execution error.
func runEnvCredentialResolution(t *testing.T, fs afero.Fs, args ...string) (*credentials.AuraCredential, afero.Fs, error) {
	t.Helper()

	if fs == nil {
		var err error
		fs, err = testfs.GetDefaultTestFs()
		require.NoError(t, err)
	}
	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)

	var captured *credentials.AuraCredential
	auraCmd := NewCmd(cfg)
	auraCmd.Use = "aura"

	leaf, _, err := auraCmd.Find([]string{"instance", "list"})
	require.NoError(t, err)
	leaf.RunE = func(_ *cobra.Command, _ []string) error {
		captured = cfg.Aura.ActiveCredential()
		return nil
	}

	root := &cobra.Command{Use: "neo4j-cli"}
	root.AddCommand(auraCmd)
	prev := cobra.EnableTraverseRunHooks
	cobra.EnableTraverseRunHooks = true
	t.Cleanup(func() { cobra.EnableTraverseRunHooks = prev })

	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"aura", "instance", "list"}, args...))
	execErr := root.Execute()

	return captured, fs, execErr
}

func TestEnvCredentialSynthesized(t *testing.T) {
	t.Setenv("NEO4J_CLI_ACCEPT_ENV_VARS", "1")
	t.Setenv(credentials.EnvAuraClientID, "env-client-id")
	t.Setenv(credentials.EnvAuraClientSecret, "env-client-secret")

	cred, fs, err := runEnvCredentialResolution(t, nil)
	require.NoError(t, err)
	require.NotNil(t, cred, "an ephemeral credential must be synthesized with no stored credential present")
	assert.Equal(t, "env-client-id", cred.ClientId)
	assert.Equal(t, "env-client-secret", cred.ClientSecret)

	assertCredentialsUntouched(t, fs)
}

func TestEnvCredentialMissingSecret(t *testing.T) {
	t.Setenv("NEO4J_CLI_ACCEPT_ENV_VARS", "1")
	t.Setenv(credentials.EnvAuraClientID, "env-client-id")

	_, _, err := runEnvCredentialResolution(t, nil)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "missing-secret must be a usage error")
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), credentials.EnvAuraClientSecret)
}

func TestEnvCredentialMissingID(t *testing.T) {
	t.Setenv("NEO4J_CLI_ACCEPT_ENV_VARS", "1")
	t.Setenv(credentials.EnvAuraClientSecret, "env-client-secret")

	_, _, err := runEnvCredentialResolution(t, nil)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "missing-id must be a usage error")
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), credentials.EnvAuraClientID)
}

func TestEnvCredentialOverriddenByExplicitFlag(t *testing.T) {
	t.Setenv("NEO4J_CLI_ACCEPT_ENV_VARS", "1")
	t.Setenv(credentials.EnvAuraClientID, "env-client-id")
	t.Setenv(credentials.EnvAuraClientSecret, "env-client-secret")

	fs, err := testfs.GetTestFs("{}", `{
		"aura": {
			"credentials": [{"name":"stored","client-id":"stored-client","client-secret":"stored-secret","access-token":"","token-expiry":0}],
			"default-credential": "stored"
		}
	}`)
	require.NoError(t, err)

	cred, _, err := runEnvCredentialResolution(t, fs, "--credential", "stored")
	require.NoError(t, err)
	require.NotNil(t, cred)
	assert.Equal(t, "stored-client", cred.ClientId, "explicit --credential must take precedence over env vars")
}

func TestEnvCredentialIgnoredWhenGateOff(t *testing.T) {
	t.Setenv(credentials.EnvAuraClientID, "env-client-id")
	t.Setenv(credentials.EnvAuraClientSecret, "env-client-secret")

	cred, fs, err := runEnvCredentialResolution(t, nil)
	require.NoError(t, err)
	assert.Nil(t, cred, "Aura env vars must be ignored when accept-env-vars is off")

	assertCredentialsUntouched(t, fs)
}

// assertCredentialsUntouched confirms the synthesized credential never reaches
// the on-disk credentials store (the in-memory FS stands in for both disk and
// keyring here — the keyring is not invoked at all on this path).
func assertCredentialsUntouched(t *testing.T, fs afero.Fs) {
	t.Helper()
	credentialsPath := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json")
	b, err := afero.ReadFile(fs, credentialsPath)
	require.NoError(t, err)
	assert.JSONEq(t, "{}", string(b), "credentials.json must remain empty")
}
