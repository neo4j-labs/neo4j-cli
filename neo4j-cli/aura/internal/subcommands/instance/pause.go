// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewPauseCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "pause <id>",
		Short: "Pauses an instance",
		Long: `Starts the pause process of an Aura instance.

Pausing an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

The pause time depends on the amount of data stored in the instance; larger quantities of data will take longer. The exact time this will take is dependent on the size of your data store.

If another operation is being performed on the instance you are trying to pause, an error will be returned that indicates that the pause operation cannot be performed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := fmt.Sprintf("/instances/%s/pause", args[0])

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodPost,
			})
			if err != nil {
				return err
			}

			// NOTE: Instance pause should not return OK (200), it always returns 202
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "status", "tenant_id", "connection_url", "cloud_provider", "region", "type", "memory"})
			}
			return nil
		},
	}
}
