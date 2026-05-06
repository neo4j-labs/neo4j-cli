// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete <id>",
		Args:        cobra.ExactArgs(1),
		Short:       "Delete a Graph Analytics Serverless session",
		Long:        `This subcommand deletes a Graph Analytics Serverless session by id.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := fmt.Sprintf("/graph-analytics/sessions/%s", args[0])

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodDelete,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusAccepted {
				output.PrintBody(cmd, cfg, resBody, []string{"id"})
			}
			return nil
		},
	}
	return cmd
}
