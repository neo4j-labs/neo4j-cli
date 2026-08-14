// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"errors"
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
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
		cloudProvider        flags.CloudProvider
		customerManagedKeyId string
		vectorOptimized      bool
		graphAnalyticsPlugin bool
		wait                 bool
		credentialName       string
		noCredentialStorage  bool
		noCredentialPrint    bool
	)

	const (
		versionFlag              = "version"
		regionFlag               = "region"
		memoryFlag               = "memory"
		nameFlag                 = "name"
		typeFlag                 = "type"
		cloudProviderFlag        = "cloud-provider"
		customerManagedKeyIdFlag = "customer-managed-key-id"
		vectorOptimizedFlag      = "vector-optimized"
		graphAnalyticsPluginFlag = "graph-analytics-plugin"
		credentialNameFlag       = "credential-name"
		noCredentialStorageFlag  = "no-credential-storage"
		noCredentialPrintFlag    = "no-credential-print"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Creates a new instance",
		Example: `# Create a free instance (no cloud provider, region, or memory required)
neo4j-cli aura instance create --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --type free --wait --rw

# Create a professional instance on AWS (us-east-1, N. Virginia)
neo4j-cli aura instance create --rw --name my-aws-instance --type professional --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --cloud-provider aws --region us-east-1 --memory 1GB

# Create a professional instance on GCP and emit JSON for scripting
neo4j-cli aura instance create --rw --name my-gcp-instance --type professional --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --cloud-provider gcp --region europe-west1 --memory 8GB --format json`,
		Long: `This subcommand starts the creation process of an Aura instance.

Region identifiers follow each cloud provider's own naming convention: AWS uses identifiers such as us-east-1, Azure uses identifiers such as eastus, and GCP uses identifiers such as us-central1.

If you're unsure of possible configurations, run 'project get' to discover the full list of supported configurations for your project. The output lists every valid combination of --cloud-provider, --region, --type, and --memory.

Creating an instance is an asynchronous operation that can be waited for with --wait. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand. Once the status transitions from "creating" to "running" you may begin to use your instance.

This subcommand returns your instance ID, initial credentials, connection URL along with your project id, cloud provider, region, instance type, and the instance name for you to use once the instance is running. It is important to store these initial credentials until you have the chance to login to your running instance and change them.

For Enterprise instances you can specify a --customer-managed-key-id flag to use a Customer Managed Key for encryption.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validateInstanceFlags(cmd, cfg, instanceFlags{
				instanceType:        _type,
				memory:              memory,
				region:              region,
				cloudProvider:       cloudProvider,
				version:             version,
				credentialName:      credentialName,
				credentialNameSet:   cmd.Flags().Changed(credentialNameFlag),
				noCredentialStorage: noCredentialStorage,
			}); err != nil {
				return err
			}

			if graphAnalyticsPlugin && _type != "professional" {
				return errors.New(`"--graph-analytics-plugin" flag can only be set when "--type" flag is set to "professional"`)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			resolvedOrgID, resolvedProjectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			// Auto-generate a default name when --name is omitted.
			name, err = resolveInstanceName(cfg, name, resolvedOrgID, resolvedProjectID)
			if err != nil {
				return err
			}

			body := buildCreateInstanceBody(version, region, name, _type, cloudProvider, customerManagedKeyId, memory, vectorOptimized, graphAnalyticsPlugin, resolvedProjectID)

			instance, err := createAndStoreInstance(cfg, body, resolvedOrgID, resolvedProjectID, credentialOptions{
				instanceType:        string(_type),
				credentialName:      credentialName,
				noCredentialStorage: noCredentialStorage,
				noCredentialPrint:   noCredentialPrint,
				warnOut:             cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}

			if instance != nil {
				renderInstanceResult(cmd, cfg, instance, noCredentialPrint, noCredentialStorage)

				if wait {
					fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for instance to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable
					instanceId, _ := instance["id"].(string)

					pollResponse, err := api.PollInstance(cfg, resolvedOrgID, resolvedProjectID, instanceId, api.InstanceStatusCreating)
					if err != nil {
						return err
					}

					fmt.Fprintln(cmd.ErrOrStderr(), "Instance Status:", pollResponse.Data.Status) //nolint:errcheck // narration to stderr; write errors are not actionable
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&version, versionFlag, "5", "The Neo4j version of the instance.")

	cmd.Flags().StringVar(&region, regionFlag, "", "The region where the instance is hosted. Values follow each cloud provider's naming convention (e.g. us-east-1 for AWS, eastus for Azure, europe-west1 for GCP). Run 'project get' to see the full list of supported regions for your project.")

	cmd.Flags().Var(&memory, memoryFlag, "The size of the instance memory (e.g. 2GB, 8GB, 64GB). Run with an invalid value to see all accepted sizes.")

	cmd.Flags().StringVar(&name, nameFlag, "", "The name of the instance (any UTF-8 characters with no trailing or leading whitespace). If omitted, a default name is generated automatically (e.g. Instance01).")

	cmd.Flags().Var(&_type, typeFlag, `(required) The type of the instance. Must be one of "free", "professional", "business-critical", or "virtual-dedicated-cloud". The former names "free-db", "professional-db", and "enterprise-db" are still accepted.`)
	cmd.MarkFlagRequired(typeFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().Var(&cloudProvider, cloudProviderFlag, `The cloud provider hosting the instance. Must be one of "aws", "azure", or "gcp".`)

	cmd.Flags().StringVar(&customerManagedKeyId, customerManagedKeyIdFlag, "", "An optional customer managed key to be used for instance creation.")

	cmd.Flags().BoolVar(&vectorOptimized, vectorOptimizedFlag, false, "An optional vector optimization configuration to be set during instance creation")

	cmd.Flags().BoolVar(&graphAnalyticsPlugin, graphAnalyticsPluginFlag, false, "An optional graph analytics plugin configuration to be set during instance creation")

	flags.RegisterWait(cmd, &wait, "Waits until created instance is ready.")

	cmd.Flags().StringVar(&credentialName, credentialNameFlag, "", "The name to use when storing the credentials locally. Defaults to <instance-id>-default.")

	cmd.Flags().BoolVar(&noCredentialStorage, noCredentialStorageFlag, false, "Skip storing the instance credentials locally after creation.")

	cmd.Flags().BoolVar(&noCredentialPrint, noCredentialPrintFlag, false, "Omit the password from the command output.")

	return cmd
}
