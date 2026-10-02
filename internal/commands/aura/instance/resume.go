// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"
	"github.com/neo4j/cli/internal/aura"
	"strings"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	commonflags "github.com/neo4j/cli/internal/flags"
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

			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			scope := aura.Scope{OrgID: orgID, ProjectID: projectID}
			instances := aura.New(cfg).Instances()
			inst, err := instances.Resume(cmd.Context(), scope, instanceID)
			if err != nil {
				return err
			}
			output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(inst.Record), []string{"id", "name", "project_id", "status", "connection_url", "cloud_provider", "region", "type", "memory"})

			if wait {
				fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for instance to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable
				status, err := instances.WaitWhile(cmd.Context(), scope, inst.ID, aura.InstanceStatusResuming)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "Instance Status:", status) //nolint:errcheck // narration to stderr; write errors are not actionable
			}
			return nil
		},
	}

	commonflags.RegisterWait(cmd, &wait, "Waits until resumed instance is ready.")
	return cmd
}
