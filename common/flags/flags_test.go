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

func TestRegisterAuraCredentialFlag_ActiveCredential(t *testing.T) {
	credJSON := credentialsJSON("my-cred", "client-1", "secret-1")

	for _, tc := range []struct {
		name           string
		args           []string
		wantActiveName string // empty means expect nil
	}{
		{
			name:           "flag not set leaves ActiveCredential nil",
			args:           []string{"resource"},
			wantActiveName: "",
		},
		{
			name:           "long form --credential sets ActiveCredential",
			args:           []string{"resource", "--credential", "my-cred"},
			wantActiveName: "my-cred",
		},
		{
			name:           "shorthand -c sets ActiveCredential",
			args:           []string{"resource", "-c", "my-cred"},
			wantActiveName: "my-cred",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := buildConfig(t, credJSON)
			cmd := &cobra.Command{
				Use:  "resource",
				RunE: func(cmd *cobra.Command, args []string) error { return nil },
			}
			flags.RegisterAuraCredentialFlag(cmd, cfg)

			err := executeWithArgs(t, "neo4j-cli", cmd, tc.args)
			require.NoError(t, err)

			if tc.wantActiveName == "" {
				assert.Nil(t, cfg.Aura.ActiveCredential())
			} else {
				require.NotNil(t, cfg.Aura.ActiveCredential())
				assert.Equal(t, tc.wantActiveName, cfg.Aura.ActiveCredential().Name)
			}
		})
	}
}

func TestRegisterAuraCredentialFlag_CredentialNotFound(t *testing.T) {
	emptyCreds := `{"aura":{"credentials":[],"default-credential":""}}`

	for _, tc := range []struct {
		name            string
		rootUse         string
		wantContains    string
		wantNotContains string
	}{
		{
			name:         "neo4j-cli root hints aura subcommand",
			rootUse:      "neo4j-cli",
			wantContains: "neo4j-cli aura credential list",
		},
		{
			name:            "aura-cli root hints standalone command",
			rootUse:         "aura-cli",
			wantContains:    "aura-cli credential list",
			wantNotContains: "neo4j-cli aura credential list",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := buildConfig(t, emptyCreds)
			cmd := &cobra.Command{
				Use:  "resource",
				RunE: func(cmd *cobra.Command, args []string) error { return nil },
			}
			flags.RegisterAuraCredentialFlag(cmd, cfg)

			err := executeWithArgs(t, tc.rootUse, cmd, []string{"resource", "--credential", "missing"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantContains)
			if tc.wantNotContains != "" {
				assert.NotContains(t, err.Error(), tc.wantNotContains)
			}
		})
	}
}

func TestRegisterAuraCredentialFlag_PriorHook(t *testing.T) {
	for _, tc := range []struct {
		name            string
		credJSON        string
		priorErr        error
		args            []string
		wantActiveName  string // empty means expect nil
		wantErrContains string // empty means expect no error
	}{
		{
			name:           "prior hook runs and credential is resolved on success",
			credJSON:       credentialsJSON("my-cred", "client-1", "secret-1"),
			priorErr:       nil,
			args:           []string{"resource", "--credential", "my-cred"},
			wantActiveName: "my-cred",
		},
		{
			name:            "prior hook error aborts credential resolution",
			credJSON:        `{"aura":{"credentials":[],"default-credential":""}}`,
			priorErr:        errors.New("prior hook failed"),
			args:            []string{"resource", "--credential", "any"},
			wantErrContains: "prior hook failed",
			wantActiveName:  "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := buildConfig(t, tc.credJSON)
			priorCalled := false
			cmd := &cobra.Command{
				Use: "resource",
				PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
					priorCalled = true
					return tc.priorErr
				},
				RunE: func(cmd *cobra.Command, args []string) error { return nil },
			}
			flags.RegisterAuraCredentialFlag(cmd, cfg)

			err := executeWithArgs(t, "neo4j-cli", cmd, tc.args)

			assert.True(t, priorCalled, "prior hook should always be called")

			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}

			if tc.wantActiveName == "" {
				assert.Nil(t, cfg.Aura.ActiveCredential())
			} else {
				require.NotNil(t, cfg.Aura.ActiveCredential())
				assert.Equal(t, tc.wantActiveName, cfg.Aura.ActiveCredential().Name)
			}
		})
	}
}
