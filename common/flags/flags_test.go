// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import (
	"errors"
	"fmt"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
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
			RegisterAuraCredentialFlag(cmd, cfg)

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
			RegisterAuraCredentialFlag(cmd, cfg)

			err := executeWithArgs(t, tc.rootUse, cmd, []string{"resource", "--credential", "missing"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantContains)
			if tc.wantNotContains != "" {
				assert.NotContains(t, err.Error(), tc.wantNotContains)
			}
			assert.True(t, cmd.SilenceUsage,
				"credential-not-found error must set SilenceUsage on the leaf so cobra does not print the full --help block")
		})
	}
}

// TestRegisterOutputFlag_NoShorthand pins CLI-85 REQ-F-001/004: the persistent
// `--format` flag must NOT claim any short-form letter. Locks the freed `-f`
// slot so a future contributor cannot re-introduce a `StringP("format", "f", …)`
// call without this test failing.
func TestRegisterOutputFlag_NoShorthand(t *testing.T) {
	cfg := buildConfig(t, `{"aura":{"credentials":[],"default-credential":""}}`)
	cmd := &cobra.Command{Use: "resource"}
	RegisterOutputFlag(cmd, cfg)

	flag := cmd.PersistentFlags().Lookup("format")
	require.NotNil(t, flag, "--format must be registered as a persistent flag")
	assert.Equal(t, "", flag.Shorthand,
		"--format must have empty Shorthand (CLI-85 freed `-f` for `update --force`)")
}

func TestRegisterOutputFlag_SilencesUsageOnError(t *testing.T) {
	for _, tc := range []struct {
		name             string
		args             []string
		wantErr          bool
		wantSilenceUsage bool
	}{
		{
			name:             "invalid --format errors and silences usage",
			args:             []string{"resource", "--format", "bogus"},
			wantErr:          true,
			wantSilenceUsage: true,
		},
		{
			name:             "valid --format succeeds without silencing usage",
			args:             []string{"resource", "--format", "json"},
			wantErr:          false,
			wantSilenceUsage: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := buildConfig(t, `{"aura":{"credentials":[],"default-credential":""}}`)
			cmd := &cobra.Command{
				Use:  "resource",
				RunE: func(cmd *cobra.Command, args []string) error { return nil },
			}
			RegisterOutputFlag(cmd, cfg)

			err := executeWithArgs(t, "neo4j-cli", cmd, tc.args)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantSilenceUsage, cmd.SilenceUsage)
		})
	}
}

func TestComposeRootPersistentPreRunE_SilencesUsageOnError(t *testing.T) {
	for _, tc := range []struct {
		name             string
		annotations      map[string]string
		args             []string
		wantErrContains  string
		wantSilenceUsage bool
	}{
		{
			name:             "write leaf without --rw errors and silences usage",
			annotations:      map[string]string{"write": "true"},
			args:             []string{"resource"},
			wantErrContains:  "this command writes; pass --rw to allow it",
			wantSilenceUsage: true,
		},
		{
			name:             "invalid --format errors and silences usage",
			args:             []string{"resource", "--format", "bogus"},
			wantErrContains:  "invalid format value specified: bogus",
			wantSilenceUsage: true,
		},
		{
			name:             "happy path does not silence usage",
			args:             []string{"resource", "--format", "json"},
			wantErrContains:  "",
			wantSilenceUsage: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := buildConfig(t, `{"aura":{"credentials":[],"default-credential":""}}`)
			cmd := &cobra.Command{
				Use:         "resource",
				Annotations: tc.annotations,
				RunE:        func(cmd *cobra.Command, args []string) error { return nil },
			}
			RegisterOutputFlag(cmd, cfg)
			RegisterRwFlag(cmd)

			root := &cobra.Command{Use: "neo4j-cli"}
			root.PersistentPreRunE = ComposeRootPersistentPreRunE(cfg)
			root.AddCommand(cmd)
			cobra.EnableTraverseRunHooks = true
			root.SetArgs(tc.args)

			err := root.Execute()
			if tc.wantErrContains == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
			}
			assert.Equal(t, tc.wantSilenceUsage, cmd.SilenceUsage)
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
			RegisterAuraCredentialFlag(cmd, cfg)

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

// TestIsWriteCommand pins the exported annotation reader shared with the MCP
// policy table: only the exact "true" string counts as a write.
func TestIsWriteCommand(t *testing.T) {
	for _, tc := range []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{name: "no annotations", annotations: nil, want: false},
		{name: "write true", annotations: map[string]string{"write": "true"}, want: true},
		{name: "write false", annotations: map[string]string{"write": "false"}, want: false},
		{name: "write TRUE is not true", annotations: map[string]string{"write": "TRUE"}, want: false},
		{name: "other annotation", annotations: map[string]string{"other": "true"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsWriteCommand(&cobra.Command{Use: "x", Annotations: tc.annotations}))
		})
	}
}

func TestEnforceWriteGate(t *testing.T) {
	for _, tc := range []struct {
		name        string
		annotations map[string]string
		rw          bool
		wantErr     string
	}{
		{
			name: "write leaf without rw errors",
			annotations: map[string]string{
				"write": "true",
			},
			wantErr: "this command writes; pass --rw to allow it",
		},
		{
			name: "write leaf with rw succeeds",
			annotations: map[string]string{
				"write": "true",
			},
			rw: true,
		},
		{
			name: "read leaf without rw succeeds",
		},
		{
			name: "non true annotation without rw succeeds",
			annotations: map[string]string{
				"write": "false",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:         "leaf",
				Annotations: tc.annotations,
			}
			RegisterRwFlag(cmd)
			cmd.Flags().AddFlagSet(cmd.PersistentFlags())
			if tc.rw {
				require.NoError(t, cmd.Flags().Set("rw", "true"))
			}

			err := EnforceWriteGate(cmd)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.EqualError(t, err, tc.wantErr)
		})
	}
}

// TestEnforceWriteGate_GateMatrix exercises the 4-step rule (REQ-F-004) by
// toggling the detectAgent and stdoutIsTerminal seams per row. Default seams
// in plain `go test` runs are no-agent / no-TTY, so other tests asserting the
// usage error keep matching without modification.
func TestEnforceWriteGate_GateMatrix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rw      bool
		agent   bool
		tty     bool
		wantErr string
	}{
		{name: "rw=true + agent + TTY → allow", rw: true, agent: true, tty: true},
		{name: "rw=true + no agent + no TTY → allow", rw: true, agent: false, tty: false},
		{name: "rw=false + agent + TTY → gate fires", rw: false, agent: true, tty: true, wantErr: "this command writes; pass --rw to allow it"},
		{name: "rw=false + agent + no TTY → gate fires", rw: false, agent: true, tty: false, wantErr: "this command writes; pass --rw to allow it"},
		{name: "rw=false + no agent + TTY → allow (NEW)", rw: false, agent: false, tty: true},
		{name: "rw=false + no agent + no TTY → gate fires", rw: false, agent: false, tty: false, wantErr: "this command writes; pass --rw to allow it"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setGateSeams(t, tc.agent, tc.tty)

			cmd := &cobra.Command{
				Use:         "leaf",
				Annotations: map[string]string{"write": "true"},
			}
			RegisterRwFlag(cmd)
			cmd.Flags().AddFlagSet(cmd.PersistentFlags())
			if tc.rw {
				require.NoError(t, cmd.Flags().Set("rw", "true"))
			}

			err := EnforceWriteGate(cmd)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.EqualError(t, err, tc.wantErr)
		})
	}
}

// setGateSeams points the detectAgent and stdoutIsTerminal seams at fixed
// values for the duration of the test. Default seams in plain `go test` runs
// are no-agent / no-TTY.
func setGateSeams(t *testing.T, agent, tty bool) {
	t.Helper()

	origAgent := detectAgent
	origTTY := stdoutIsTerminal
	detectAgent = func() bool { return agent }
	stdoutIsTerminal = func() bool { return tty }
	t.Cleanup(func() {
		detectAgent = origAgent
		stdoutIsTerminal = origTTY
	})
}

// TestRequireWriteAccess covers the precedence rules directly, without an
// annotation.
func TestRequireWriteAccess(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rw      bool
		agent   bool
		tty     bool
		wantErr string
	}{
		{name: "rw=true → allow", rw: true, agent: true, tty: false},
		{name: "agent without rw → gate fires", agent: true, tty: true, wantErr: "this command writes; pass --rw to allow it"},
		{name: "TTY without agent → allow", tty: true},
		{name: "no agent, no TTY → gate fires", wantErr: "this command writes; pass --rw to allow it"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setGateSeams(t, tc.agent, tc.tty)

			cmd := &cobra.Command{Use: "leaf"}
			RegisterRwFlag(cmd)
			cmd.Flags().AddFlagSet(cmd.PersistentFlags())
			if tc.rw {
				require.NoError(t, cmd.Flags().Set("rw", "true"))
			}

			err := RequireWriteAccess(cmd)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.EqualError(t, err, tc.wantErr)

			var ce *clierr.CLIError
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, 2, ce.Code)
		})
	}
}

// TestRequireWriteAccess_NoRwFlag covers the nil-flag path: cmd.Flag("rw")
// returns nil when no ancestor registered the flag.
func TestRequireWriteAccess_NoRwFlag(t *testing.T) {
	setGateSeams(t, false, true)

	require.NoError(t, RequireWriteAccess(&cobra.Command{Use: "leaf"}))
}

func TestRegisterRwFlagHelp(t *testing.T) {
	cmd := &cobra.Command{Use: "root"}
	RegisterRwFlag(cmd)

	flag := cmd.PersistentFlags().Lookup("rw")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "Allow write operations. Auto-applied in interactive terminals; required when running under an agent harness or non-interactive script.", flag.Usage)
}
