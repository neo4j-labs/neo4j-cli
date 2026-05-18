// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Get a Graph Analytics Serverless session",
		Example: `# Get a session by ID
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000

# Render the session as a TOON table
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --format toon

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --format json`,
		Long: `This subcommand returns the details of a Graph Analytics Serverless session.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionId := strings.TrimSpace(args[0])
			path := fmt.Sprintf("/graph-analytics/sessions/%s", sessionId)

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodGet,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{
					"id",
					"name",
					"memory",
					"status",
					"created_at",
					"user_id",
					"tenant_id",
					"cloud_provider",
					"region",
					"host",
					"expiry_date",
					"instance_id",
				})
			}
			return nil
		},
	}
	return cmd
}
