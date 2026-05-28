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

func TestRegisterOrgProjectFlags_OrgID(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantOrg string
	}{
		{
			name:    "no flag",
			args:    []string{},
			wantOrg: "",
		},
		{
			name:    "organization-id flag",
			args:    []string{"--organization-id", "org-123"},
			wantOrg: "org-123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:  "test",
				RunE: func(cmd *cobra.Command, args []string) error { return nil },
			}
			flags.RegisterOrgProjectFlags(cmd)

			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)

			require.NoError(t, cmd.Execute())
			got, err := cmd.PersistentFlags().GetString(flags.OrgIDFlag)
			require.NoError(t, err)
			assert.Equal(t, tc.wantOrg, got)
		})
	}
}

func TestRegisterOrgProjectFlags_NoTenantIDFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	flags.RegisterOrgProjectFlags(cmd)

	f := cmd.PersistentFlags().Lookup("tenant-id")
	assert.Nil(t, f, "--tenant-id must not be registered after CLI-126 removal")
}

func TestRegisterOrgProjectFlags_ProjectID(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantProject string
	}{
		{
			name:        "no flag",
			args:        []string{},
			wantProject: "",
		},
		{
			name:        "project-id flag",
			args:        []string{"--project-id", "proj-456"},
			wantProject: "proj-456",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{
				Use:  "test",
				RunE: func(cmd *cobra.Command, args []string) error { return nil },
			}
			flags.RegisterOrgProjectFlags(cmd)

			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)

			require.NoError(t, cmd.Execute())
			got, err := cmd.PersistentFlags().GetString(flags.ProjectIDFlag)
			require.NoError(t, err)
			assert.Equal(t, tc.wantProject, got)
		})
	}
}
