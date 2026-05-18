// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"encoding/json"
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
		wait bool
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "resume <id>",
		Short:       "Resumes an instance",
		Long: `Starts the resume process of an Aura instance.

Resuming an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to resume, an error will be returned that indicates that resume cannot be performed.`,
		Example: `# Resume a paused Aura instance
neo4j-cli aura instance resume 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Resume and wait until the instance is ready
neo4j-cli aura instance resume 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --wait --rw

# Resume and emit the response as JSON for scripting
neo4j-cli aura instance resume 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceID := strings.TrimSpace(args[0])

			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true

			// Pre-flight ownership check.
			if _, err := utils.FetchAndVerifyInstanceInProject(cfg, instanceID, projectID); err != nil {
				return err
			}

			path := fmt.Sprintf("/instances/%s/resume", instanceID)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodPost,
			})
			if err != nil {
				return err
			}

			// NOTE: Instance resume should not return OK (200), it always returns 202
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				responseData := api.ParseBody(resBody)
				renamed := utils.RenameResponseField(responseData, "tenant_id", "project_id")
				output.PrintBodyMap(cmd, cfg, renamed, []string{"id", "name", "project_id", "status", "connection_url", "cloud_provider", "region", "type", "memory"})

				if wait {
					fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for instance to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable
					var response api.CreateInstanceResponse
					if err := json.Unmarshal(resBody, &response); err != nil {
						return err
					}

					pollResponse, err := api.PollInstance(cfg, response.Data.Id, api.InstanceStatusResuming)
					if err != nil {
						return err
					}

					fmt.Fprintln(cmd.ErrOrStderr(), "Instance Status:", pollResponse.Data.Status) //nolint:errcheck // narration to stderr; write errors are not actionable
				}
			}
			return nil
		},
	}

	flags.RegisterWait(cmd, &wait, "Waits until resumed instance is ready.")
	return cmd
}
