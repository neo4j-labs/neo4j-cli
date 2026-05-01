// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package snapshot

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var instanceId string
	var await bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Takes an on-demand snapshot",
		Long: `This subcommand starts the on-demand snapshot creation process for an Aura instance.
Creating a snapshot is an asynchronous operation. You can poll the current status of this operation by periodically getting the snapshots details for the instance ID using the get subcommand.
The time taken to complete a snapshot depends on the amount of data stored in the instance; larger quantities of data will take longer. The exact time this will take is dependent on the size of your data store.`,
		RunE: utils.WrapRunE(cfg, func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			path := fmt.Sprintf("/instances/%s/snapshots", instanceId)

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodPost,
			})

			if err != nil {
				return err
			}

			if statusCode == http.StatusAccepted {
				output.PrintBody(cmd, cfg, resBody, []string{"snapshot_id"})

				if await {
					cmd.Println("Waiting for snapshot to be ready...")
					var response api.CreateSnapshotResponse
					if err := json.Unmarshal(resBody, &response); err != nil {
						return err
					}

					// Snapshot is not ready after pending
					pollResponse, err := api.PollSnapshot(cfg, instanceId, response.Data.SnapshotId)
					if err != nil {
						return err
					}

					cmd.Println("Snapshot Status:", pollResponse.Data.Status)
				}
			}
			return nil
		}),
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance to create a snapshot of")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().BoolVar(&await, "await", false, "Waits until created snapshot is ready.")

	return cmd
}
