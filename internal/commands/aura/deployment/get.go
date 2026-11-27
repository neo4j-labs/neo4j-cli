// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package deployment

import (
	"strings"

	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func newGetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Returns deployment details",
		Long:  "Returns details about a specific Fleet Manager deployment, including its DBMS edition and packaging and the state of its token.",
		Example: `# Get a deployment in the default project
neo4j-cli aura deployment get 22222222-2222-2222-2222-222222222222

# Get a deployment in a specific project
neo4j-cli aura deployment get 22222222-2222-2222-2222-222222222222 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get a deployment as JSON for scripting
neo4j-cli aura deployment get 22222222-2222-2222-2222-222222222222 --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			d, err := auraclient.New(cfg).Deployments().Get(cmd.Context(), auraclient.Scope{OrgID: orgID, ProjectID: projectID}, strings.TrimSpace(args[0]))
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, d.Record, []string{
				"id", "name", "status", "created_by", "connection_url",
				"dbms:edition", "dbms:packaging",
				"token:expiry_time", "token:auto_rotated", "token:creation_time",
			})
			return nil
		},
	}
}
