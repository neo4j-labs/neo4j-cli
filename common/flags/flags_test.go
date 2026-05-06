// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestRegisterRwFlagHelp(t *testing.T) {
	cmd := &cobra.Command{Use: "root"}
	RegisterRwFlag(cmd)

	flag := cmd.PersistentFlags().Lookup("rw")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
	assert.Equal(t, "Allow write operations. Required for any command that mutates state (Aura API, local config, credentials, skills, write cypher).", flag.Usage)
}
