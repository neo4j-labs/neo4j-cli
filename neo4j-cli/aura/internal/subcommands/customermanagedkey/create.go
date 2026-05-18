// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package customermanagedkey

import (
	"encoding/json"
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
		region        string
		name          string
		instanceType  flags.InstanceType
		cloudProvider flags.CloudProvider
		keyId         string
		wait          bool
	)

	const (
		regionFlag        = "region"
		nameFlag          = "name"
		instanceTypeFlag  = "type"
		cloudProviderFlag = "cloud-provider"
		keyIdFlag         = "key-id"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Creates a new customer managed key",
		Long: `This subcommand creates a new Customer Managed Key in Aura. Creating a new key is an asynchronous operation.

Before you can use the key you will need to setup permissions for it. Log in to the Console, navigate to 'Customer Managed Keys' and click on the Edit icon next to the Key in order to see the instructions.

You can poll the current status of this operation by periodically getting the key details using the get subcommand.

Once the key has a status of ready you can use it for creating new instances by setting the --customer-managed-key-id flag.`,
		Example: `# Create a customer managed key (AWS-hosted instance)
neo4j-cli aura customer-managed-key create --name my-key --region us-east-1 --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Create a key and wait until it is ready before returning
neo4j-cli aura customer-managed-key create --name my-key --region us-east-1 --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --wait --rw

# Create a key and emit JSON for scripting
neo4j-cli aura customer-managed-key create --name my-key --region us-east-1 --type enterprise-db --cloud-provider aws --key-id arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			body := map[string]any{
				"region":         region,
				"name":           name,
				"instance_type":  instanceType,
				"cloud_provider": cloudProvider,
				"key_id":         keyId,
				"tenant_id":      projectID,
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
				responseData := api.ParseBody(resBody)
				renamed := utils.RenameResponseField(responseData, "tenant_id", "project_id")
				output.PrintBodyMap(cmd, cfg, renamed, []string{"id", "name", "project_id", "status", "created", "cloud_provider", "key_id", "region", "type"})

				if wait {
					fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for customer managed key to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable
					var response api.CreateCMKResponse
					if err := json.Unmarshal(resBody, &response); err != nil {
						return err
					}

					pollResponse, err := api.PollCMK(cfg, response.Data.Id)
					if err != nil {
						return err
					}

					fmt.Fprintln(cmd.ErrOrStderr(), "CMK Status:", pollResponse.Data.Status) //nolint:errcheck // narration to stderr; write errors are not actionable
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

	cmd.Flags().Var(&cloudProvider, cloudProviderFlag, "(required) The cloud provider hosting the instance.")
	cmd.MarkFlagRequired(cloudProviderFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&keyId, keyIdFlag, "", "(required) Encryption Key ARN")
	cmd.MarkFlagRequired(keyIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	flags.RegisterWait(cmd, &wait, "Waits until created customer managed key is ready.")

	return cmd
}
