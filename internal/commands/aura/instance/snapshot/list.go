// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package snapshot

import (
	"github.com/neo4j/cli/internal/auraclient"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	var instanceId string
	var date string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of snapshots",
		Long:  `This subcommand returns a list of available snapshots from the current day.`,
		Example: `# List today's snapshots for an instance
neo4j-cli aura instance snapshot list --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List snapshots for a specific date
neo4j-cli aura instance snapshot list --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --date 2025-01-15

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura instance snapshot list --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if _, err := utils.ResolveAndVerifyInstance(cmd, cfg, instanceId); err != nil {
				return err
			}

			snaps, err := auraclient.New(cfg).Snapshots().List(cmd.Context(), instanceId, date)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(snaps))
			for i, sn := range snaps {
				rows[i] = sn.Record
			}
			output.PrintRecords(cmd, cfg, rows, []string{"snapshot_id", "instance_id", "profile", "status", "timestamp"})
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "The ID of the instance to list the snapshots of")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
	cmd.Flags().StringVar(&date, "date", "", "An optional date to list snapshots for a given day, defaults to today. Must be formatted with an ISO formatted date string (YYYY-MM-DD)")

	return cmd
}
