// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// credentialsJSON returns a credentials JSON string with one named credential.
func credentialsJSON(name, clientID, clientSecret string) string {
	return fmt.Sprintf(`{
		"aura": {
			"credentials": [{"name":%q,"client-id":%q,"client-secret":%q,"access-token":"","token-expiry":0}],
			"default-credential": %q
		}
	}`, name, clientID, clientSecret, name)
}

// buildConfig creates a minimal *clicfg.Config backed by an in-memory filesystem.
func buildConfig(t *testing.T, credJSON string) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"json"}`, credJSON)
	require.NoError(t, err)
	return clicfg.NewConfig(fs, "test", clicfg.AuraScope)
}

// executeWithArgs wires a root command (containing cmd as a child), parses args,
// and returns the error from Execute.
func executeWithArgs(t *testing.T, rootUse string, cmd *cobra.Command, args []string) error {
	t.Helper()
	root := &cobra.Command{Use: rootUse}
	root.AddCommand(cmd)
	cobra.EnableTraverseRunHooks = true
	root.SetArgs(args)
	return root.Execute()
}

func TestRegisterAuraCredentialFlag_FlagNotSet_NilActiveCredential(t *testing.T) {
	credJSON := credentialsJSON("my-cred", "client-1", "secret-1")
	cfg := buildConfig(t, credJSON)

	cmd := &cobra.Command{
		Use:  "resource",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	flags.RegisterAuraCredentialFlag(cmd, cfg)

	err := executeWithArgs(t, "neo4j-cli", cmd, []string{"resource"})
	require.NoError(t, err)
	assert.Nil(t, cfg.Aura.ActiveCredential(), "ActiveCredential should remain nil when --credential is not supplied")
}

func TestRegisterAuraCredentialFlag_CredentialFound_SetsActiveCredential(t *testing.T) {
	credJSON := credentialsJSON("my-cred", "client-1", "secret-1")
	cfg := buildConfig(t, credJSON)

	cmd := &cobra.Command{
		Use:  "resource",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	flags.RegisterAuraCredentialFlag(cmd, cfg)

	err := executeWithArgs(t, "neo4j-cli", cmd, []string{"resource", "--credential", "my-cred"})
	require.NoError(t, err)

	active := cfg.Aura.ActiveCredential()
	require.NotNil(t, active)
	assert.Equal(t, "my-cred", active.Name)
}

func TestRegisterAuraCredentialFlag_ShorthandAccepted(t *testing.T) {
	credJSON := credentialsJSON("my-cred", "client-1", "secret-1")
	cfg := buildConfig(t, credJSON)

	cmd := &cobra.Command{
		Use:  "resource",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	flags.RegisterAuraCredentialFlag(cmd, cfg)

	err := executeWithArgs(t, "neo4j-cli", cmd, []string{"resource", "-c", "my-cred"})
	require.NoError(t, err)

	active := cfg.Aura.ActiveCredential()
	require.NotNil(t, active)
	assert.Equal(t, "my-cred", active.Name)
}

func TestRegisterAuraCredentialFlag_CredentialNotFound_Neo4jCliHint(t *testing.T) {
	cfg := buildConfig(t, `{"aura":{"credentials":[],"default-credential":""}}`)

	cmd := &cobra.Command{
		Use:  "resource",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	flags.RegisterAuraCredentialFlag(cmd, cfg)

	err := executeWithArgs(t, "neo4j-cli", cmd, []string{"resource", "--credential", "missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "neo4j-cli aura credential list")
}

func TestRegisterAuraCredentialFlag_CredentialNotFound_AuraCliHint(t *testing.T) {
	cfg := buildConfig(t, `{"aura":{"credentials":[],"default-credential":""}}`)

	cmd := &cobra.Command{
		Use:  "resource",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	flags.RegisterAuraCredentialFlag(cmd, cfg)

	err := executeWithArgs(t, "aura-cli", cmd, []string{"resource", "--credential", "missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aura-cli credential list")
	assert.NotContains(t, err.Error(), "neo4j-cli aura credential list")
}

func TestRegisterAuraCredentialFlag_WrapsExistingPersistentPreRunE(t *testing.T) {
	credJSON := credentialsJSON("my-cred", "client-1", "secret-1")
	cfg := buildConfig(t, credJSON)

	priorCalled := false
	cmd := &cobra.Command{
		Use: "resource",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			priorCalled = true
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	flags.RegisterAuraCredentialFlag(cmd, cfg)

	err := executeWithArgs(t, "neo4j-cli", cmd, []string{"resource", "--credential", "my-cred"})
	require.NoError(t, err)
	assert.True(t, priorCalled, "prior PersistentPreRunE should have been called")
	require.NotNil(t, cfg.Aura.ActiveCredential())
	assert.Equal(t, "my-cred", cfg.Aura.ActiveCredential().Name)
}

func TestRegisterAuraCredentialFlag_PriorHookError_AbortsBefore_CredentialResolution(t *testing.T) {
	cfg := buildConfig(t, `{"aura":{"credentials":[],"default-credential":""}}`)

	priorErr := errors.New("prior hook failed")
	cmd := &cobra.Command{
		Use: "resource",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return priorErr
		},
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	flags.RegisterAuraCredentialFlag(cmd, cfg)

	err := executeWithArgs(t, "neo4j-cli", cmd, []string{"resource", "--credential", "any"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prior hook failed")
	// ActiveCredential should not be set because prior hook aborted
	assert.Nil(t, cfg.Aura.ActiveCredential())
}
