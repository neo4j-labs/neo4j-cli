// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package workspace

import (
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func newUseCmd(cfg *clicfg.Config) *cobra.Command {
	var organizationId string
	var projectId string

	cmd := &cobra.Command{
		Use:   "use [organizationId/projectId]",
		Short: "Sets the active organization and project workspace",
		Long: `This subcommand sets the active organization and project workspace used by default
in subsequent commands. Accepts either a positional {organizationId}/{projectId} slug
or the --organization-id and --project-id flags (but not both).

The workspace is validated against the Aura API before being persisted.`,
		Annotations: map[string]string{"write": "true"},
		Example: `# Set workspace using positional slug
neo4j-cli aura workspace use 00000000-0000-0000-0000-000000000000/11111111-1111-1111-1111-111111111111 --rw

# Set workspace using flags
neo4j-cli aura workspace use --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Verify the workspace was set after switching
neo4j-cli aura workspace use 00000000-0000-0000-0000-000000000000/11111111-1111-1111-1111-111111111111 --rw && neo4j-cli aura workspace list --format json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hasSlug := len(args) == 1
			hasFlags := organizationId != "" || projectId != ""

			if hasSlug && hasFlags {
				return fmt.Errorf("positional slug and --organization-id/--project-id flags are mutually exclusive")
			}

			var slug string

			if hasSlug {
				slug = args[0]
			} else {
				if organizationId == "" {
					return fmt.Errorf("required flag \"organization-id\" not set")
				}
				if projectId == "" {
					return fmt.Errorf("required flag \"project-id\" not set")
				}
				slug = organizationId + "/" + projectId
			}

			cmd.SilenceUsage = true
			return ValidateAndSetDefaultWorkspace(cfg, slug)
		},
	}

	cmd.Flags().StringVar(&organizationId, "organization-id", "", "Organization ID")
	cmd.Flags().StringVar(&projectId, "project-id", "", "Project ID")

	return cmd
}
