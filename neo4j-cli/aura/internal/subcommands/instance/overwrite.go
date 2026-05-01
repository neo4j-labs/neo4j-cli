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

func NewOverwriteCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		sourceInstanceId string
		sourceSnapshotId string
		await            bool
	)

	const (
		sourceInstanceIdFlag = "source-instance-id"
		sourceSnapshotIdFlag = "source-snapshot-id"
	)

	cmd := &cobra.Command{
		Use:   "overwrite <id>",
		Short: "Starts the process of overwriting the specified instance with data from the source instance provided",
		Long: `Starts the process of overwriting the specified instance with data from the source instance provided.

The overwrite process mimics the 'Clone to existing' functionality of the Aura Console.

If only --source-instance-id is provided, a new snapshot of that instance is created and used for overwriting. Alternatively, you can specify an additional --source-snapshot-id to use a specific snapshot for overwriting, from --source-instance-id provided, otherwise as a snapshot of the instance being overwritten. The snapshot specified must be exportable.
		`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceId := args[0]
			path := fmt.Sprintf("/instances/%s/overwrite", instanceId)

			cmd.SilenceUsage = true

			postBody := make(map[string]any)
			if sourceInstanceId == "" {
				sourceInstanceId = instanceId
			}
			postBody["source_instance_id"] = sourceInstanceId

			if sourceSnapshotId != "" {
				postBody["source_snapshot_id"] = sourceSnapshotId
			}

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:   http.MethodPost,
				PostBody: postBody,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusAccepted {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "tenant_id", "status", "connection_url", "cloud_provider", "region", "type", "memory", "storage", "customer_managed_key_id"})
			}

			if await {
				cmd.Println("Waiting for instance to be ready...")
				pollResponse, err := api.PollInstance(cfg, instanceId, api.InstanceStatusOverwriting)
				if err != nil {
					return err
				}

				cmd.Println("Instance Status:", pollResponse.Data.Status)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&sourceInstanceId, sourceInstanceIdFlag, "", "The ID of the instance to overwrite with, from the source snapshot ID if provided, otherwise takes a new snapshot and overwrites")
	cmd.Flags().StringVar(&sourceSnapshotId, sourceSnapshotIdFlag, "", "The ID of the snapshot to overwrite with, which must be exportable, from the source instance ID if provided, otherwise the argument provided instance")

	cmd.MarkFlagsOneRequired(sourceInstanceIdFlag, sourceSnapshotIdFlag)

	cmd.Flags().BoolVar(&await, "await", false, "Waits until created snapshot is ready")

	return cmd
}
