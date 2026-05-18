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
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewPauseCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:         "pause <id>",
		Short:       "Pauses an instance",
		Annotations: map[string]string{"write": "true"},
		Long: `Starts the pause process of an Aura instance.

Pausing an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

The pause time depends on the amount of data stored in the instance; larger quantities of data will take longer. The exact time this will take is dependent on the size of your data store.

If another operation is being performed on the instance you are trying to pause, an error will be returned that indicates that the pause operation cannot be performed.`,
		Example: `# Pause an Aura instance
neo4j-cli aura instance pause 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Pause an instance and emit the response as JSON
neo4j-cli aura instance pause 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json

# Pause and pipe the response status through jq
neo4j-cli aura instance pause 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json | jq -r '.data.status'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceID := strings.TrimSpace(args[0])

			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			// Pre-flight ownership check.
			if _, err := utils.FetchAndVerifyInstanceInProject(cfg, instanceID, projectID); err != nil {
				return err
			}

			path := fmt.Sprintf("/instances/%s/pause", instanceID)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodPost,
			})
			if err != nil {
				return err
			}

			// NOTE: Instance pause should not return OK (200), it always returns 202
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				responseData := api.ParseBody(resBody)
				renamed := utils.RenameResponseField(responseData, "tenant_id", "project_id")
				output.PrintBodyMap(cmd, cfg, renamed, []string{"id", "name", "status", "project_id", "connection_url", "cloud_provider", "region", "type", "memory"})
			}
			return nil
		},
	}
}
