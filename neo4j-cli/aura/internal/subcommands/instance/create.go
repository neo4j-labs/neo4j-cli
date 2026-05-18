// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
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
		Example: `# Create a free-db instance (no cloud provider, region, or memory required)
neo4j-cli aura instance create --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --type free-db --wait --rw

# Create a professional-db instance on AWS (us-east-1, N. Virginia)
neo4j-cli aura instance create --rw --name my-aws-instance --type professional-db --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --cloud-provider aws --region us-east-1 --memory 1GB

# Create a professional-db instance on GCP and emit JSON for scripting
neo4j-cli aura instance create --rw --name my-gcp-instance --type professional-db --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --cloud-provider gcp --region europe-west1 --memory 8GB --format json`,
		Long: `This subcommand starts the creation process of an Aura instance.

Region identifiers follow each cloud provider's own naming convention: AWS uses identifiers such as us-east-1, Azure uses identifiers such as eastus, and GCP uses identifiers such as us-central1.

If you're unsure of possible configurations, run 'tenant get' to discover the full list of supported configurations for your tenant. The output lists every valid combination of --cloud-provider, --region, --type, and --memory.

Creating an instance is an asynchronous operation that can be waited for with --wait. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand. Once the status transitions from "creating" to "running" you may begin to use your instance.

This subcommand returns your instance ID, initial credentials, connection URL along with your project id, cloud provider, region, instance type, and the instance name for you to use once the instance is running. It is important to store these initial credentials until you have the chance to login to your running instance and change them.

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

			credentialNameChanged := cmd.Flags().Changed(credentialNameFlag)

			if credentialNameChanged && noCredentialStorage {
				return fmt.Errorf(`"--%s" and "--%s" cannot be used together`, credentialNameFlag, noCredentialStorageFlag)
			}

			if credentialNameChanged && credentialName == "" {
				return fmt.Errorf(`invalid argument "" for "--%s" flag: name must not be empty`, credentialNameFlag)
			}

			if !noCredentialStorage && (cfg.Credentials == nil || cfg.Credentials.Dbms == nil) {
				return fmt.Errorf("credential storage is not available; use --%s to skip storing credentials locally", noCredentialStorageFlag)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			_, resolvedProjectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			// Auto-generate a default name when --name is omitted.
			if name == "" {
				listBody, _, listErr := api.MakeRequest(cfg, "/instances", &api.RequestConfig{
					Method:      http.MethodGet,
					QueryParams: map[string]string{"tenantId": resolvedProjectID},
				})
				if listErr != nil {
					return listErr
				}
				listData := api.ParseBody(listBody)
				existingNames := make([]string, 0, len(listData.AsArray()))
				for _, inst := range listData.AsArray() {
					if n, ok := inst["name"].(string); ok {
						existingNames = append(existingNames, n)
					}
				}
				name = defaultInstanceName(existingNames)
			}

			body := map[string]any{
				"version":        version,
				"region":         region,
				"name":           name,
				"type":           _type,
				"cloud_provider": cloudProvider,
				"tenant_id":      resolvedProjectID,
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

			resBody, statusCode, err := api.MakeRequest(cfg, "/instances", &api.RequestConfig{
				PostBody: body,
				Method:   http.MethodPost,
			})
			if err != nil {
				return err
			}

			// NOTE: Instance create should not return OK (200), it always returns 202, checking both just in case
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				responseData := api.ParseBody(resBody)
				instance, err := responseData.GetSingleOrError()
				if err != nil {
					return err
				}

				if !noCredentialStorage {
					instanceID, _ := instance["id"].(string)
					username, _ := instance["username"].(string)
					password, _ := instance["password"].(string)
					uri, _ := instance["connection_url"].(string)

					base := baseCredentialName(instanceID, credentialName)
					resolvedName := resolveCredentialName(cfg.Credentials.Dbms, base)
					instance["credential_name"] = resolvedName

					if addErr := cfg.Credentials.Dbms.Add(resolvedName, username, password, databaseName(string(_type), username), uri); addErr != nil {
						if noCredentialPrint {
							fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to store credentials locally (%s). The password has been omitted from output; reset it via the Aura Console.\n", addErr) //nolint:errcheck // warning to stderr; write errors are not actionable
						} else {
							fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to store credentials locally (%s). Save the printed password now — it cannot be retrieved later.\n", addErr) //nolint:errcheck // warning to stderr; write errors are not actionable
						}
					}
				}

				if noCredentialPrint {
					delete(instance, "password")
				}

				// Rename tenant_id -> project_id before output
				renamed := utils.RenameResponseField(api.NewSingleValueResponseData(instance), "tenant_id", "project_id")
				renamedInstance, _ := renamed.GetSingleOrError()

				fields := []string{"id", "name", "project_id", "connection_url", "username"}
				if !noCredentialPrint {
					fields = append(fields, "password")
				}
				if !noCredentialStorage {
					fields = append(fields, "credential_name")
				}
				fields = append(fields, "cloud_provider", "region", "type")

				output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(renamedInstance), fields)

				if wait {
					fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for instance to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable
					instanceId, _ := instance["id"].(string)

					pollResponse, err := api.PollInstance(cfg, instanceId, api.InstanceStatusCreating)
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

	cmd.Flags().StringVar(&region, regionFlag, "", "The region where the instance is hosted. Values follow each cloud provider's naming convention (e.g. us-east-1 for AWS, eastus for Azure, europe-west1 for GCP). Run 'tenant get' to see the full list of supported regions for your tenant.")

	cmd.Flags().Var(&memory, memoryFlag, "The size of the instance memory (e.g. 2GB, 8GB, 64GB). Run with an invalid value to see all accepted sizes.")

	cmd.Flags().StringVar(&name, nameFlag, "", "The name of the instance (any UTF-8 characters with no trailing or leading whitespace). If omitted, a default name is generated automatically (e.g. Instance01).")

	cmd.Flags().Var(&_type, typeFlag, `(required) The type of the instance. Must be one of "free-db", "professional-db", "business-critical", "enterprise-db", "professional-ds", or "enterprise-ds".`)
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
