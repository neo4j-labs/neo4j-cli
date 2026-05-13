// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags_test

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
)

func TestRegisterWait(t *testing.T) {
	const deprecationMsg = "Flag --await has been deprecated, use --wait instead"

	tests := []struct {
		name           string
		args           []string
		wantWait       bool
		wantDeprecated bool
	}{
		{
			name:           "neither flag",
			args:           []string{},
			wantWait:       false,
			wantDeprecated: false,
		},
		{
			name:           "wait flag sets bool with no deprecation",
			args:           []string{"--wait"},
			wantWait:       true,
			wantDeprecated: false,
		},
		{
			name:           "await alias sets bool and prints deprecation",
			args:           []string{"--await"},
			wantWait:       true,
			wantDeprecated: true,
		},
		{
			name:           "both flags set bool and print deprecation",
			args:           []string{"--wait", "--await"},
			wantWait:       true,
			wantDeprecated: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var wait bool
			cmd := &cobra.Command{
				Use:  "test",
				RunE: func(cmd *cobra.Command, args []string) error { return nil },
			}
			flags.RegisterWait(cmd, &wait, "Waits until operation completes.")

			// pflag writes deprecation warnings via the FlagSet's own output, which
			// defaults to os.Stderr. Capture it explicitly.
			var stderr bytes.Buffer
			cmd.Flags().SetOutput(&stderr)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&stderr)
			cmd.SetArgs(tc.args)

			require.NoError(t, cmd.Execute())
			assert.Equal(t, tc.wantWait, wait)
			if tc.wantDeprecated {
				assert.Contains(t, stderr.String(), deprecationMsg)
			} else {
				assert.NotContains(t, stderr.String(), deprecationMsg)
			}
		})
	}
}

func TestRegisterWait_HidesAliasFromHelp(t *testing.T) {
	var wait bool
	cmd := &cobra.Command{Use: "test"}
	flags.RegisterWait(cmd, &wait, "Waits until operation completes.")

	usage := cmd.Flags().FlagUsages()
	assert.Contains(t, usage, "--wait")
	assert.NotContains(t, usage, "--await")
}
