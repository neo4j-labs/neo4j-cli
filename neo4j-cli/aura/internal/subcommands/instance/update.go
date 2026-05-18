// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewUpdateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		memory string
		name   string
	)

	const (
		memoryFlag = "memory"
		nameFlag   = "name"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "update <id>",
		Short:       "Updates an instance",
		Long: `This command allows you to rename and/or resize an Aura instance.

Resizing an instance is an asynchronous operation. The instance remains available throughout.`,
		Example: `# Rename an Aura instance
neo4j-cli aura instance update 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --name my-renamed-instance --rw

# Resize an Aura instance to 8GB of memory
neo4j-cli aura instance update 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --memory 8GB --rw

# Rename and resize, emitting JSON for scripting
neo4j-cli aura instance update 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --name my-renamed-instance --memory 8GB --rw --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceID := args[0]

			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			// Pre-flight ownership check.
			if _, err := utils.FetchAndVerifyInstanceInProject(cfg, instanceID, projectID); err != nil {
				return err
			}

			body := map[string]any{}

			if memory != "" {
				body["memory"] = memory
			}

			if name != "" {
				body["name"] = name
			}

			path := fmt.Sprintf("/instances/%s", instanceID)

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:   http.MethodPatch,
				PostBody: body,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				responseData := api.ParseBody(resBody)
				renamed := utils.RenameResponseField(responseData, "tenant_id", "project_id")
				output.PrintBodyMap(cmd, cfg, renamed, []string{"id", "name", "project_id", "status", "connection_url", "cloud_provider", "region", "type", "memory"})
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&memory, memoryFlag, "", "The size of the instance memory in GB.")

	cmd.Flags().StringVar(&name, nameFlag, "", "The name of the instance (any UTF-8 characters with no trailing or leading whitespace).")

	cmd.MarkFlagsOneRequired(memoryFlag, nameFlag)

	return cmd
}
