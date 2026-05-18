// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package customermanagedkey

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Deletes a customer managed key",
		Annotations: map[string]string{"write": "true"},
		Long: `Deletes a Customer Managed Key from Aura.

Note that you can only delete a Key if it is not being used by any instances, otherwise you will get an error with the reason field set to encryption-key-is-active.`,
		Example: `# Delete a customer managed key by ID
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Delete a key and emit JSON for scripting
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json

# Delete and confirm by piping the response through jq
neo4j-cli aura customer-managed-key delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json | jq -r '.data.deleted'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmkID := args[0]

			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			// Pre-flight ownership check.
			if _, err := utils.FetchAndVerifyCMKInProject(cfg, cmkID, projectID); err != nil {
				return err
			}

			path := fmt.Sprintf("/customer-managed-keys/%s", cmkID)
			_, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodDelete,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusNoContent {
				fmt.Fprintf(cmd.ErrOrStderr(), "customer-managed-key %s deleted\n", cmkID) //nolint:errcheck // narration to stderr; write errors are not actionable
				output.PrintBodyMap(cmd, cfg,
					api.NewSingleValueResponseData(map[string]any{"deleted": true, "id": cmkID}),
					[]string{"deleted", "id"})
			}

			return nil
		},
	}
}
