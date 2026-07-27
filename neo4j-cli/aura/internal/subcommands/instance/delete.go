// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

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

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "delete <id>",
		Short:       "Deletes an instance",
		Annotations: map[string]string{"write": "true"},
		Long: `Starts the deletion process of an Aura instance.

Deleting an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to delete, an error will be returned that indicates that deletion cannot be performed.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Delete an instance by ID
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Delete an instance and emit the response as JSON
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json

# Delete and pipe the response status through jq
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json | jq -r '.data.status'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			instanceID := strings.TrimSpace(args[0])

			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			if err := utils.ValidateResourceID("instance", instanceID); err != nil {
				return err
			}

			if err := confirm.Require(cmd, instanceID); err != nil {
				return err
			}

			path := api.ScopedInstancePath(orgID, projectID, instanceID)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:  http.MethodDelete,
				Version: api.AuraApiVersion2,
			})

			if err != nil {
				return err
			}
			// NOTE: Instance delete should not return OK (200), it always returns 202
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				responseData := api.ParseBody(resBody)
				normalized := utils.NormalizeV2Beta1Response(responseData)
				output.PrintBodyMap(cmd, cfg, normalized, []string{"id", "name", "project_id", "status", "connection_url", "cloud_provider", "region", "type", "memory"})
			}

			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
