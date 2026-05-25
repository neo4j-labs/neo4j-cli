// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete <id>",
		Args:        cobra.ExactArgs(1),
		Short:       "Delete a Graph Analytics Serverless session",
		Example: `# Delete a session by ID
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Delete a session and emit JSON for scripting
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json

# Delete a session, suppressing all stdout output
neo4j-cli aura graph-analytics session delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force > /dev/null`,
		Long: `This subcommand deletes a Graph Analytics Serverless session by id.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := strings.TrimSpace(args[0])

			cmd.SilenceUsage = true
			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			if _, err := utils.FetchAndVerifySessionInProject(cfg, sessionID, projectID); err != nil {
				return err
			}

			if err := confirm.Require(cmd, sessionID); err != nil {
				return err
			}

			path := fmt.Sprintf("/graph-analytics/sessions/%s", sessionID)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodDelete,
			})
			if err != nil {
				// On 404 the API layer's parseResourceFromRequest mis-segments
				// the nested /graph-analytics/sessions/<id> path (extracts
				// "graph-analytic"). Rewrite the context so the user gets
				// session-specific Suggestion text. The preflight
				// FetchAndVerifySessionInProject already covers the
				// ownership-mismatch path.
				return utils.WithNotFoundContext(err, "graph-analytics-session", sessionID, "Run 'neo4j-cli aura graph-analytics session list --project-id <id>' to see sessions in this project.")
			}

			if statusCode == http.StatusAccepted {
				output.PrintBody(cmd, cfg, resBody, []string{"id"})
			}
			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
