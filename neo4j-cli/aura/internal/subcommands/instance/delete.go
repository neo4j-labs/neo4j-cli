// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Deletes an instance",
		Annotations: map[string]string{"write": "true"},
		Long: `Starts the deletion process of an Aura instance.

Deleting an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to delete, an error will be returned that indicates that deletion cannot be performed.`,
		Example: `# Delete an instance by ID
neo4j-cli aura instance delete 00000000-0000-0000-0000-000000000000 --rw

# Delete an instance and emit the response as JSON
neo4j-cli aura instance delete 00000000-0000-0000-0000-000000000000 --rw --format json

# Delete and pipe the response status through jq
neo4j-cli aura instance delete 00000000-0000-0000-0000-000000000000 --rw --format json | jq -r '.data.status'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceId := strings.TrimSpace(args[0])
			path := fmt.Sprintf("/instances/%s", instanceId)
			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodDelete,
			})

			if err != nil {
				return err
			}
			// NOTE: Instance delete should not return OK (200), it always returns 202
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "tenant_id", "status", "connection_url", "cloud_provider", "region", "type", "memory"})
			}

			return nil
		},
	}
}
