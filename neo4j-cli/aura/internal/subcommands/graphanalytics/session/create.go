// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session

import (
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name          string
		memory        string
		ttl           string
		instance_id   string
		tenant_id     string
		cloudProvider string
		region        string
		await         bool
	)

	const (
		nameFlag          = "name"
		memoryFlag        = "memory"
		ttlFlag           = "ttl"
		instanceIdFlag    = "instance-id"
		tenantIdFlag      = "tenant-id"
		cloudProviderFlag = "cloud-provider"
		regionFlag        = "region"
		awaitFlag         = "await"
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a new Aura Graph Analytics Serverless session",
		Long: `This subcommand gets or creates a Aura Graph Analytics Serverless session. If no Session with a matching name and project/tenant is found, one will be created. A Session is either attached to an AuraDB, or standalone.
				Creating a session is an asynchronous operation that can be awaited with --await.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if instance_id == "" {
				cmd.MarkFlagRequired(cloudProviderFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
				cmd.MarkFlagRequired(regionFlag)        //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

				if cfg.Aura.DefaultTenant() != "" {
					cmd.MarkFlagRequired(tenantIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
				}
			}

			cmd.MarkFlagRequired(nameFlag)   //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
			cmd.MarkFlagRequired(memoryFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"name":   name,
				"memory": memory,
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

			if tenant_id == "" && instance_id == "" {
				body["tenant_id"] = cfg.Aura.DefaultTenant()
			} else if tenant_id != "" {
				body["tenant_id"] = tenant_id
			}

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, "/graph-analytics/sessions", &api.RequestConfig{
				PostBody: body,
				Method:   http.MethodPost,
			})
			if err != nil {
				return err
			}

			// NOTE: Return 202 if new session gets created and 200 if existing session was found
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "tenant_id", "memory", "status", "created_at"})

				if await {
					cmd.Println("Waiting for session to be ready...")

					respData := api.ParseBody(resBody)
					status := respData.AsArray()[0]["status"]
					sessionID := respData.AsArray()[0]["id"].(string)
					if status == "Ready" {
						return nil
					}

					pollResponse, err := api.PollGraphAnalyticsSessionReady(cfg, sessionID, api.GraphAnalyticsSessionWaitingStatus)
					if err != nil {
						return err
					}

					cmd.Println("Session Status:", pollResponse.Data.Status)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&memory, memoryFlag, "", "(required) The size of the session memory in GB.")
	cmd.MarkFlagRequired(memoryFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) The name of the session.")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&tenant_id, tenantIdFlag, "", "The Aura project/tenant ID")

	cmd.Flags().StringVar(&cloudProvider, cloudProviderFlag, "", "The cloud provider hosting the session.")
	cmd.Flags().StringVar(&region, regionFlag, "", "The region where the session is hosted.")

	cmd.Flags().StringVar(&instance_id, instanceIdFlag, "", "The ID of the instance to create the session for.")
	cmd.Flags().StringVar(&ttl, ttlFlag, "", "This optional parameter specifies the time-to-live of the session. The session will be marked as expired if the session was unused for the provided duration.")

	cmd.Flags().BoolVar(&await, awaitFlag, false, "Waits until created session is ready.")

	return cmd
}
