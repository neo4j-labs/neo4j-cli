// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package customermanagedkey

import (
	"encoding/json"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		region        string
		name          string
		instanceType  flags.InstanceType
		tenantId      string
		cloudProvider flags.CloudProvider
		keyId         string
		await         bool
	)

	const (
		regionFlag        = "region"
		nameFlag          = "name"
		instanceTypeFlag  = "type"
		tenantIdFlag      = "tenant-id"
		cloudProviderFlag = "cloud-provider"
		keyIdFlag         = "key-id"
		awaitFlag         = "await"
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a new customer managed key",
		Long: `This subcommand creates a new Customer Managed Key in Aura. Creating a new key is an asynchronous operation.

Before you can use the key you will need to setup permissions for it. Log in to the Console, navigate to 'Customer Managed Keys' and click on the Edit icon next to the Key in order to see the instructions.

You can poll the current status of this operation by periodically getting the key details using the get subcommand.

Once the key has a status of ready you can use it for creating new instances by setting the --customer-managed-key-id flag.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if cfg.Aura.DefaultTenant() == "" {
				cmd.MarkFlagRequired(tenantIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"region":         region,
				"name":           name,
				"instance_type":  instanceType,
				"cloud_provider": cloudProvider,
				"key_id":         keyId,
			}

			if tenantId == "" {
				body["tenant_id"] = cfg.Aura.DefaultTenant()
			} else {
				body["tenant_id"] = tenantId
			}

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, "/customer-managed-keys", &api.RequestConfig{
				Method:   http.MethodPost,
				PostBody: body,
			})
			if err != nil {
				return err
			}
			// NOTE: Instance delete should not return OK (200), it always returns 202
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "tenant_id", "status", "created", "cloud_provider", "key_id", "region", "type"})

				if await {
					cmd.Println("Waiting for customer managed key to be ready...")
					var response api.CreateCMKResponse
					if err := json.Unmarshal(resBody, &response); err != nil {
						return err
					}

					pollResponse, err := api.PollCMK(cfg, response.Data.Id)
					if err != nil {
						return err
					}

					cmd.Println("CMK Status:", pollResponse.Data.Status)
				}

			}

			return nil

		},
	}

	cmd.Flags().StringVar(&region, regionFlag, "", "(required) The region where the instance is hosted.")
	cmd.MarkFlagRequired(regionFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) The name of the customer managed key (any UTF-8 characters with no trailing or leading whitespace).")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().Var(&instanceType, instanceTypeFlag, "(required) The type of the instance.")
	cmd.MarkFlagRequired(instanceTypeFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&tenantId, tenantIdFlag, "", "The Aura tenant/project ID")

	cmd.Flags().Var(&cloudProvider, cloudProviderFlag, "(required) The cloud provider hosting the instance.")
	cmd.MarkFlagRequired(cloudProviderFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&keyId, keyIdFlag, "", "(required) Encryption Key ARN")
	cmd.MarkFlagRequired(keyIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().BoolVar(&await, awaitFlag, false, "Waits until created customer managed key is ready.")

	return cmd
}
