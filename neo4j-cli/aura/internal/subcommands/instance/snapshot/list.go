// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package snapshot

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	var instanceId string
	var date string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of snapshots",
		Long:  `This subcommand returns a list of available snapshots from the current day.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			path := fmt.Sprintf("/instances/%s/snapshots", instanceId)
			var queryParams map[string]string
			if date != "" {
				queryParams = make(map[string]string)
				queryParams["date"] = date
			}
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:      http.MethodGet,
				QueryParams: queryParams,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"snapshot_id", "instance_id", "profile", "status", "timestamp"})
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "The ID of the instance to list the snapshots of")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
	cmd.Flags().StringVar(&date, "date", "", "An optional date to list snapshots for a given day, defaults to today. Must be formatted with an ISO formatted date string (YYYY-MM-DD)")

	return cmd
}
