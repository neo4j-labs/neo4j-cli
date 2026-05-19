// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session

import (
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Get a Graph Analytics Serverless session",
		Example: `# Get a session by ID
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Render the session as a TOON table
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format toon

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura graph-analytics session get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json`,
		Long: `This subcommand returns the details of a Graph Analytics Serverless session.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := strings.TrimSpace(args[0])

			cmd.SilenceUsage = true
			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}
			resBody, err := utils.FetchAndVerifySessionInProject(cfg, sessionID, projectID)
			if err != nil {
				// On 404 the API layer's parseResourceFromRequest mis-segments
				// the nested /graph-analytics/sessions/<id> path (extracts
				// "graph-analytic"). Rewrite the context so the user gets
				// session-specific Suggestion text. The ownership-mismatch
				// path is already tagged correctly inside the helper.
				return utils.WithNotFoundContext(err, "graph-analytics-session", sessionID, "Run 'neo4j-cli aura graph-analytics session list --project-id <id>' to see sessions in this project.")
			}

			if resBody != nil {
				responseData := api.ParseBody(resBody)
				renamed := utils.RenameResponseField(responseData, "tenant_id", "project_id")
				output.PrintBodyMap(cmd, cfg, renamed, []string{
					"id",
					"name",
					"memory",
					"status",
					"created_at",
					"user_id",
					"project_id",
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
