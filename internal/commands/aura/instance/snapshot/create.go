// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package snapshot

import (
	"fmt"
	"github.com/neo4j/cli/internal/aura"

	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	commonflags "github.com/neo4j/cli/internal/flags"
	"github.com/spf13/cobra"
)

func NewCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var instanceId string
	var wait bool

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Takes an on-demand snapshot",
		Long: `This subcommand starts the on-demand snapshot creation process for an Aura instance.
Creating a snapshot is an asynchronous operation. You can poll the current status of this operation by periodically getting the snapshots details for the instance ID using the get subcommand.
The time taken to complete a snapshot depends on the amount of data stored in the instance; larger quantities of data will take longer. The exact time this will take is dependent on the size of your data store.`,
		Example: `# Take an on-demand snapshot of an instance
neo4j-cli aura instance snapshot create --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Take a snapshot and wait until it is ready
neo4j-cli aura instance snapshot create --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --wait --rw

# Take a snapshot and emit JSON for scripting
neo4j-cli aura instance snapshot create --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if _, err := utils.ResolveAndVerifyInstance(cmd, cfg, instanceId); err != nil {
				return err
			}

			snap, err := aura.New(cfg).Snapshots().Create(cmd.Context(), instanceId)
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, snap.Record, []string{"snapshot_id"})

			if wait {
				fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for snapshot to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable
				status, err := aura.New(cfg).Snapshots().WaitWhilePending(cmd.Context(), instanceId, snap.ID)
				if err != nil {
					return err
				}

				fmt.Fprintln(cmd.ErrOrStderr(), "Snapshot Status:", status) //nolint:errcheck // narration to stderr; write errors are not actionable
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance to create a snapshot of")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	commonflags.RegisterWait(cmd, &wait, "Waits until created snapshot is ready.")

	return cmd
}
