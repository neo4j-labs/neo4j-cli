// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewOverwriteCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		sourceInstanceId string
		sourceSnapshotId string
		wait             bool
	)

	const (
		sourceInstanceIdFlag = "source-instance-id"
		sourceSnapshotIdFlag = "source-snapshot-id"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "overwrite <id>",
		Short:       "Starts the process of overwriting the specified instance with data from the source instance provided",
		Long: `Starts the process of overwriting the specified instance with data from the source instance provided.

The overwrite process mimics the 'Clone to existing' functionality of the Aura Console.

If only --source-instance-id is provided, a new snapshot of that instance is created and used for overwriting. Alternatively, you can specify an additional --source-snapshot-id to use a specific snapshot for overwriting, from --source-instance-id provided, otherwise as a snapshot of the instance being overwritten. The snapshot specified must be exportable.
		`,
		Example: `# Overwrite an instance with a fresh snapshot of a source instance
neo4j-cli aura instance overwrite 00000000-0000-0000-0000-000000000000 --source-instance-id 11111111-1111-1111-1111-111111111111 --rw

# Overwrite using a specific exportable snapshot and wait until ready
neo4j-cli aura instance overwrite 00000000-0000-0000-0000-000000000000 --source-instance-id 11111111-1111-1111-1111-111111111111 --source-snapshot-id 22222222-2222-2222-2222-222222222222 --wait --rw

# Overwrite and emit JSON for scripting
neo4j-cli aura instance overwrite 00000000-0000-0000-0000-000000000000 --source-instance-id 11111111-1111-1111-1111-111111111111 --rw --format json`,
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

			if wait {
				fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for instance to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable
				pollResponse, err := api.PollInstance(cfg, instanceId, api.InstanceStatusOverwriting)
				if err != nil {
					return err
				}

				fmt.Fprintln(cmd.ErrOrStderr(), "Instance Status:", pollResponse.Data.Status) //nolint:errcheck // narration to stderr; write errors are not actionable
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&sourceInstanceId, sourceInstanceIdFlag, "", "The ID of the instance to overwrite with, from the source snapshot ID if provided, otherwise takes a new snapshot and overwrites")
	cmd.Flags().StringVar(&sourceSnapshotId, sourceSnapshotIdFlag, "", "The ID of the snapshot to overwrite with, which must be exportable, from the source instance ID if provided, otherwise the argument provided instance")

	cmd.MarkFlagsOneRequired(sourceInstanceIdFlag, sourceSnapshotIdFlag)

	flags.RegisterWait(cmd, &wait, "Waits until created snapshot is ready")

	return cmd
}
