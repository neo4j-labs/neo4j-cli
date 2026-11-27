// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server

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
	"address",
	"name",
	"status",
	"last_ping",
	"version",
	"plugin_version",
}

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Returns deployment servers",
		Long:  "Returns the servers of the given Fleet Manager deployment.",
		Example: `# List the servers of a deployment in the default project
neo4j-cli aura deployment server list --deployment-id 22222222-2222-2222-2222-222222222222

# List them as JSON for scripting
neo4j-cli aura deployment server list --deployment-id 22222222-2222-2222-2222-222222222222 --format json`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			rows, err := auraclient.New(cfg).Deployments().Servers(cmd.Context(), auraclient.Scope{OrgID: orgID, ProjectID: projectID}, depflag.Get(cmd))
			if err != nil {
				return err
			}
			output.PrintRecords(cmd, cfg, rows, listFields)
			return nil
		},
	}
}
