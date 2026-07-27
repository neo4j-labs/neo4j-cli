// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name          string
		memory        string
		ttl           string
		instance_id   string
		cloudProvider string
		region        string
		wait          bool
	)

	const (
		nameFlag          = "name"
		memoryFlag        = "memory"
		ttlFlag           = "ttl"
		instanceIdFlag    = "instance-id"
		cloudProviderFlag = "cloud-provider"
		regionFlag        = "region"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Creates a new Aura Graph Analytics Serverless session",
		Example: `# Create a standalone session in a specific project on AWS
neo4j-cli aura graph-analytics session create --rw --name my-session --memory 8GB --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --cloud-provider aws --region us-east-1

# Create a session attached to an existing Aura instance and wait until ready
neo4j-cli aura graph-analytics session create --rw --name attached-session --memory 8GB --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --wait

# Create a session with a TTL and emit JSON for scripting
neo4j-cli aura graph-analytics session create --rw --name scripted-session --memory 4GB --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --ttl 1h --format json`,
		Long: `This subcommand gets or creates a Aura Graph Analytics Serverless session. If no Session with a matching name and project is found, one will be created. A Session is either attached to an AuraDB, or standalone.
Creating a session is an asynchronous operation that can be waited for with --wait.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if instance_id == "" {
				cmd.MarkFlagRequired(cloudProviderFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
				cmd.MarkFlagRequired(regionFlag)        //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			body := map[string]any{
				"name":      name,
				"memory":    memory,
				"tenant_id": projectID,
			}

			if ttl != "" {
				body["ttl"] = ttl
			}

			if instance_id != "" {
				body["instance_id"] = instance_id
			}

			if cloudProvider != "" {
				body["cloud_provider"] = cloudProvider
			}

			if region != "" {
				body["region"] = region
			}
			path := api.ScopedSessionsPath(orgID, projectID)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				PostBody: body,
				Method:   http.MethodPost,
				Version:  api.AuraApiVersion2,
			})
			if err != nil {
				return err
			}

			// NOTE: Return 202 if new session gets created and 200 if existing session was found
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				responseData := api.ParseBody(resBody)
				normalized := utils.NormalizeV2Beta1Response(responseData)
				output.PrintBodyMap(cmd, cfg, normalized, []string{"id", "name", "project_id", "memory", "status", "created_at"})

				if wait {
					fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for session to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable

					status := normalized.AsArray()[0]["status"]
					sessionID := normalized.AsArray()[0]["id"].(string)
					if status == "Ready" {
						return nil
					}

					pollResponse, err := api.PollGraphAnalyticsSessionReady(cfg, orgID, projectID, sessionID, api.GraphAnalyticsSessionWaitingStatus)
					if err != nil {
						return err
					}

					fmt.Fprintln(cmd.ErrOrStderr(), "Session Status:", pollResponse.Data.Status) //nolint:errcheck // narration to stderr; write errors are not actionable
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&memory, memoryFlag, "", "(required) The size of the session memory in GB.")
	cmd.MarkFlagRequired(memoryFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) The name of the session.")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&cloudProvider, cloudProviderFlag, "", "The cloud provider hosting the session.")
	cmd.Flags().StringVar(&region, regionFlag, "", "The region where the session is hosted.")

	cmd.Flags().StringVar(&instance_id, instanceIdFlag, "", "The ID of the instance to create the session for.")
	cmd.Flags().StringVar(&ttl, ttlFlag, "", "This optional parameter specifies the time-to-live of the session. The session will be marked as expired if the session was unused for the provided duration.")

	flags.RegisterWait(cmd, &wait, "Waits until created session is ready.")

	return cmd
}
