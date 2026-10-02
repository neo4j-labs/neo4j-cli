// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package snapshot

import (
	"github.com/neo4j/cli/internal/aura"
	"strings"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	var instanceId string

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get details of a snapshot",
		Long:  `This endpoint returns details about a specific snapshot.`,
		Example: `# Get details of a snapshot by its ID
neo4j-cli aura instance snapshot get 22222222-2222-2222-2222-222222222222 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details and emit JSON for scripting
neo4j-cli aura instance snapshot get 22222222-2222-2222-2222-222222222222 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json

# Check whether a snapshot is exportable via jq
neo4j-cli aura instance snapshot get 22222222-2222-2222-2222-222222222222 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json | jq -r '.data.exportable'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if _, err := utils.ResolveAndVerifyInstance(cmd, cfg, instanceId); err != nil {
				return err
			}

			snapshotId := strings.TrimSpace(args[0])
			snap, err := aura.New(cfg).Snapshots().Get(cmd.Context(), instanceId, snapshotId)
			if err != nil {
				return err
			}
			output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(snap.Record), []string{"snapshot_id", "instance_id", "profile", "status", "timestamp", "exportable"})
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "The ID of the instance to get the snapshot details of")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
