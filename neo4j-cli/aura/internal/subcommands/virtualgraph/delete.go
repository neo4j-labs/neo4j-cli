// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph

import (
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete <id>",
		Short:       "Deletes a virtual graph",
		Long: `Starts the deletion process of a virtual graph.

Deleting a virtual graph is an asynchronous operation, and is idempotent: deleting an id that does not exist in the project succeeds. The underlying data source is not affected.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Delete a virtual graph by ID
neo4j-cli aura virtual-graph delete ge82059a --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Delete a virtual graph using a configured default workspace, emitting JSON for scripting
neo4j-cli aura virtual-graph delete ge82059a --rw --yes --force --format json

# Delete a virtual graph, suppressing all stdout output
neo4j-cli aura virtual-graph delete ge82059a --rw --yes --force > /dev/null`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			virtualGraphID := strings.TrimSpace(args[0])

			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			if err := utils.ValidateResourceID(resourceName, virtualGraphID); err != nil {
				return err
			}

			if err := confirm.Require(cmd, virtualGraphID); err != nil {
				return err
			}

			path := api.ScopedVirtualGraphPath(orgID, projectID, virtualGraphID)
			_, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:  http.MethodDelete,
				Version: api.AuraApiVersion2,
			})
			if err != nil {
				return err
			}

			// DELETE acknowledges with 202 and an empty body, and a follow-up GET
			// 404s, so echo the accepted id rather than printing nothing or
			// re-reading a resource that is already gone.
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(map[string]any{"id": virtualGraphID}), []string{"id"})
			}

			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
