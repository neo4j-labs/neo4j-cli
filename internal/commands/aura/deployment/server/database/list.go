// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/deployment/internal/depflag"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

var listFields = []string{
	"id",
	"name",
	"type",
	"current_status",
	"last_committed_txn",
	"replication_lag",
	"role",
	"writer",
}

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Returns deployment server databases",
		Long:  "Returns the databases of the given Fleet Manager deployment server.",
		Example: `# List the databases of a deployment server in the default project
neo4j-cli aura deployment server database list --deployment-id 22222222-2222-2222-2222-222222222222 --server-id SERVER_ID

# List them as JSON for scripting
neo4j-cli aura deployment server database list --deployment-id 22222222-2222-2222-2222-222222222222 --server-id SERVER_ID --format json`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}
			serverID, _ := cmd.Flags().GetString("server-id")

			rows, err := auraclient.New(cfg).Deployments().ServerDatabases(cmd.Context(), auraclient.Scope{OrgID: orgID, ProjectID: projectID}, depflag.Get(cmd), serverID)
			if err != nil {
				return err
			}
			output.PrintRecords(cmd, cfg, rows, listFields)
			return nil
		},
	}
}
