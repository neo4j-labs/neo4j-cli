// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewResumeCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		instanceId string
		wait       bool
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "resume <id>",
		Short:       "Resume a GraphQL Data API",
		Long: `This command starts the resuming process of an existing GraphQL Data API.

Resuming a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready. Once the status transitions from "resuming" to "ready" you may begin to use your GraphQL Data API.`,
		Example: `# Resume a paused GraphQL Data API (using flags)
neo4j-cli aura graphql resume 11111111 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Resume a GraphQL Data API using a configured default workspace
neo4j-cli aura graphql resume 11111111 --instance-id 00000000 --rw

# Resume a GraphQL Data API and wait until it is ready
neo4j-cli aura graphql resume 11111111 --instance-id 00000000 --wait --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			graphqlId := strings.TrimSpace(args[0])
			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}
			if _, err = utils.FetchAndVerifyInstanceInProject(cfg, instanceId, projectID); err != nil {
				return err
			}
			path := fmt.Sprintf("/instances/%s/data-apis/graphql/%s/resume", instanceId, graphqlId)

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:  http.MethodPost,
				Version: api.AuraApiVersionBeta1,
			})
			if err != nil {
				return err
			}

			// NOTE: resume should not return OK (200), it always returns 202, checking both just in case
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "status", "url"})

				if wait {
					fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for GraphQL Data API to be resumed...") //nolint:errcheck // narration to stderr; write errors are not actionable
					pollResponse, err := api.PollGraphQLDataApi(cfg, instanceId, graphqlId, api.GraphQLDataApiStatusResuming)
					if err != nil {
						return err
					}

					fmt.Fprintln(cmd.ErrOrStderr(), "GraphQL Data API Status:", pollResponse.Data.Status) //nolint:errcheck // narration to stderr; write errors are not actionable
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance to resume the Data API for")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	flags.RegisterWait(cmd, &wait, "Waits until GraphQL Data API is resumed.")

	return cmd
}
