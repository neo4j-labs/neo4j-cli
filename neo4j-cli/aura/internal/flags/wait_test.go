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
	tests := []struct {
		name     string
		args     []string
		wantWait bool
	}{
		{
			name:     "neither flag",
			args:     []string{},
			wantWait: false,
		},
		{
			name:     "wait flag sets bool",
			args:     []string{"--wait"},
			wantWait: true,
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

			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)

			require.NoError(t, cmd.Execute())
			assert.Equal(t, tc.wantWait, wait)
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
