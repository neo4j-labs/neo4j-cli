// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewResumeCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		await bool
	)

	const (
		awaitFlag = "await"
	)

	cmd := &cobra.Command{
		Use:   "resume <id>",
		Short: "Resumes an instance",
		Long: `Starts the resume process of an Aura instance.

Resuming an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to resume, an error will be returned that indicates that resume cannot be performed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := fmt.Sprintf("/instances/%s/resume", args[0])

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodPost,
			})
			if err != nil {
				return err
			}

			// NOTE: Instance resume should not return OK (200), it always returns 202
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "tenant_id", "status", "connection_url", "cloud_provider", "region", "type", "memory"})

				if await {
					cmd.Println("Waiting for instance to be ready...")
					var response api.CreateInstanceResponse
					if err := json.Unmarshal(resBody, &response); err != nil {
						return err
					}

					pollResponse, err := api.PollInstance(cfg, response.Data.Id, api.InstanceStatusResuming)
					if err != nil {
						return err
					}

					cmd.Println("Instance Status:", pollResponse.Data.Status)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&await, awaitFlag, false, "Waits until resumed instance is ready.")
	return cmd
}
