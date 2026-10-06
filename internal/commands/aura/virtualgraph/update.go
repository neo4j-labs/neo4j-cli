// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph

import (
	"github.com/neo4j/cli/internal/auraclient"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func newUpdateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name          string
		memory        string
		importModelID string
	)

	const (
		nameFlag          = "name"
		memoryFlag        = "memory"
		importModelIDFlag = "import-model-id"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "update <id>",
		Short:       "Updates a virtual graph",
		Long: `This subcommand renames a virtual graph, resizes its memory allocation, and/or updates its graph model schema from a Data Importer model.

Updating a virtual graph is an asynchronous operation: the API acknowledges the request and applies it in the background, so the details printed on completion may still show the pre-update state. Re-run the get subcommand to observe the applied change.`,
		Example: `# Rename a virtual graph
neo4j-cli aura virtual-graph update ge82059a --name renamed-analytics --rw --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Resize a virtual graph to 8Gi of memory
neo4j-cli aura virtual-graph update ge82059a --memory 8Gi --rw

# Update the graph model schema from a Data Importer model, emitting JSON for scripting
neo4j-cli aura virtual-graph update ge82059a --import-model-id im-xyz789 --rw --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			virtualGraphID := strings.TrimSpace(args[0])

			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			vg, err := auraclient.New(cfg).VirtualGraphs().Update(cmd.Context(), auraclient.Scope{OrgID: orgID, ProjectID: projectID}, virtualGraphID, auraclient.VirtualGraphPatch{
				Name:          name,
				Memory:        memory,
				ImportModelID: importModelID,
			})
			if err != nil {
				return err
			}

			printVirtualGraph(cmd, cfg, vg)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "The new name of the virtual graph (maximum 30 characters).")

	cmd.Flags().StringVar(&memory, memoryFlag, "", "The new memory allocation, for example 8Gi. Must be one of the values from 'aura virtual-graph allowed-configs'.")

	cmd.Flags().StringVar(&importModelID, importModelIDFlag, "", "The ID of a graph data model stored in Data Importer. When provided, the virtual graph's graph model schema is updated from this model.")

	cmd.MarkFlagsOneRequired(nameFlag, memoryFlag, importModelIDFlag)

	return cmd
}
