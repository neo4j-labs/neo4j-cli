// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"encoding/json"
	"errors"
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
		version              string
		region               string
		memory               flags.Memory
		name                 string
		_type                flags.InstanceType
		tenantId             string
		cloudProvider        flags.CloudProvider
		customerManagedKeyId string
		vectorOptimized      bool
		graphAnalyticsPlugin bool
		await                bool
	)

	const (
		versionFlag              = "version"
		regionFlag               = "region"
		memoryFlag               = "memory"
		nameFlag                 = "name"
		typeFlag                 = "type"
		tenantIdFlag             = "tenant-id"
		cloudProviderFlag        = "cloud-provider"
		customerManagedKeyIdFlag = "customer-managed-key-id"
		vectorOptimizedFlag      = "vector-optimized"
		graphAnalyticsPluginFlag = "graph-analytics-plugin"
		awaitFlag                = "await"
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a new instance",
		Long: `This subcommand starts the creation process of an Aura instance.

Creating an instance is an asynchronous operation that can be awaited with --await. Supported instance configurations for your tenant can be obtained by calling the tenant get subcommand.

You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand. Once the status transitions from "creating" to "running" you may begin to use your instance.

This subcommand returns your instance ID, initial credentials, connection URL along with your tenant id, cloud provider, region, instance type, and the instance name for you to use once the instance is running. It is important to store these initial credentials until you have the chance to login to your running instance and change them.

You must also provide a --cloud-provider flag with the subcommand, which specifies which cloud provider the instances will be hosted in. The acceptable values for this field are gcp, aws, or azure.

For Enterprise instances you can specify a --customer-managed-key-id flag to use a Customer Managed Key for encryption.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if _type != "free-db" {
				cmd.MarkFlagRequired(memoryFlag)        //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
				cmd.MarkFlagRequired(regionFlag)        //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
				cmd.MarkFlagRequired(cloudProviderFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
			} else {
				if memory != "" {
					return fmt.Errorf(`invalid argument "%s" for "--memory" flag: must not be set when "--type" flag is set to "free-db"`, memory)
				}
				if region != "" {
					return fmt.Errorf(`invalid argument "%s" for "--region" flag: must not be set when "--type" flag is set to "free-db"`, region)
				}
				if cloudProvider != "" {
					return fmt.Errorf(`invalid argument "%s" for "--cloud-provider" flag: must not be set when "--type" flag is set to "free-db"`, cloudProvider)
				}
			}

			if version != "4" && version != "5" {
				return fmt.Errorf(`invalid argument "%s" for "--version" flag: must be one of "4" or "5"`, version)
			}

			if graphAnalyticsPlugin && _type != "professional-db" {
				return errors.New(`"--graph-analytics-plugin" flag can only be set when "--type" flag is set to "professional-db"`)
			}

			if cfg.Aura.DefaultTenant() == "" {
				cmd.MarkFlagRequired(tenantIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
			}

			return nil
		},
		RunE: utils.WrapRunE(cfg, func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"version":        version,
				"region":         region,
				"name":           name,
				"type":           _type,
				"cloud_provider": cloudProvider,
			}

			if tenantId == "" {
				body["tenant_id"] = cfg.Aura.DefaultTenant()
			} else {
				body["tenant_id"] = tenantId
			}

			if _type == "free-db" {
				body["memory"] = "1GB"
				body["region"] = "europe-west1"
				body["cloud_provider"] = "gcp"
				body["version"] = "5"
			} else {
				body["memory"] = memory
				body["region"] = region
				body["vector_optimized"] = vectorOptimized
			}

			if _type == "professional-db" {
				body["graph_analytics_plugin"] = graphAnalyticsPlugin
			}

			if customerManagedKeyId != "" {
				body["customer_managed_key_id"] = customerManagedKeyId
			}

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, "/instances", &api.RequestConfig{
				PostBody: body,
				Method:   http.MethodPost,
			})
			if err != nil {
				return err
			}

			// NOTE: Instance create should not return OK (200), it always returns 202, checking both just in case
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "tenant_id", "connection_url", "username", "password", "cloud_provider", "region", "type"})

				if await {
					cmd.Println("Waiting for instance to be ready...")
					var response api.CreateInstanceResponse
					if err := json.Unmarshal(resBody, &response); err != nil {
						return err
					}

					pollResponse, err := api.PollInstance(cfg, response.Data.Id, api.InstanceStatusCreating)
					if err != nil {
						return err
					}

					cmd.Println("Instance Status:", pollResponse.Data.Status)
				}
			}

			return nil
		}),
	}

	cmd.Flags().StringVar(&version, versionFlag, "5", "The Neo4j version of the instance.")

	cmd.Flags().StringVar(&region, regionFlag, "", "The region where the instance is hosted.")

	cmd.Flags().Var(&memory, memoryFlag, "The size of the instance memory in GB.")

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) The name of the instance (any UTF-8 characters with no trailing or leading whitespace).")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().Var(&_type, typeFlag, "(required) The type of the instance.")
	cmd.MarkFlagRequired(typeFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&tenantId, tenantIdFlag, "", "The Aura tenant/project ID")

	cmd.Flags().Var(&cloudProvider, cloudProviderFlag, "The cloud provider hosting the instance.")

	cmd.Flags().StringVar(&customerManagedKeyId, customerManagedKeyIdFlag, "", "An optional customer managed key to be used for instance creation.")

	cmd.Flags().BoolVar(&vectorOptimized, vectorOptimizedFlag, false, "An optional vector optimization configuration to be set during instance creation")

	cmd.Flags().BoolVar(&graphAnalyticsPlugin, graphAnalyticsPluginFlag, false, "An optional graph analytics plugin configuration to be set during instance creation")

	cmd.Flags().BoolVar(&await, awaitFlag, false, "Waits until created instance is ready.")

	return cmd
}
