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

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete <id>",
		Args:        cobra.ExactArgs(1),
		Short:       "Delete a Graph Analytics Serverless session",
		Example: `# Delete a session by ID
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --rw

# Delete a session and emit JSON for scripting
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --rw --format json

# Delete a session, suppressing all stdout output
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --rw > /dev/null`,
		Long: `This subcommand deletes a Graph Analytics Serverless session by id.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionId := strings.TrimSpace(args[0])
			path := fmt.Sprintf("/graph-analytics/sessions/%s", sessionId)

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
