// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	auraflags "github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func newCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name               string
		dataSourceID       string
		importModelID      string
		cloudProvider      auraflags.CloudProvider
		region             string
		memory             string
		maximumBytesBilled int64
		wait               bool
	)

	const (
		nameFlag               = "name"
		dataSourceIDFlag       = "data-source-id"
		importModelIDFlag      = "import-model-id"
		cloudProviderFlag      = "cloud-provider"
		regionFlag             = "region"
		memoryFlag             = "memory"
		maximumBytesBilledFlag = "maximum-bytes-billed"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Creates a new virtual graph",
		Long: `This subcommand creates a virtual graph from an existing data source and graph data model, both created in Data Importer.

Creating a virtual graph is an asynchronous operation that can be waited for with --wait: the command returns immediately with status 'creating'.

The initial Neo4j password is returned once, in the plain_password field of the response, and cannot be retrieved again.`,
		Example: `# Create a virtual graph on GCP from a Data Importer data source and model
neo4j-cli aura virtual-graph create --rw --name sales-analytics --data-source-id ds-abc123 --import-model-id im-xyz789 --cloud-provider gcp --region europe-west1 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Create with an explicit memory allocation and wait until it is running
neo4j-cli aura virtual-graph create --rw --name sales-analytics --data-source-id ds-abc123 --import-model-id im-xyz789 --cloud-provider aws --region us-east-1 --memory 8Gi --wait

# Create over a BigQuery data source with a per-query bytes-billed cap, emitting JSON for scripting
neo4j-cli aura virtual-graph create --rw --name bq-analytics --data-source-id ds-bq001 --import-model-id im-xyz789 --cloud-provider gcp --region europe-west1 --maximum-bytes-billed 1099511627776 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			// project_id is NOT sent: the API derives the owning project from the
			// org/project-scoped path and the caller's token.
			body := map[string]any{
				"name":            name,
				"data_source_id":  dataSourceID,
				"import_model_id": importModelID,
				"cloud_provider":  cloudProvider.String(),
				"region":          region,
			}

			if memory != "" {
				body["memory"] = memory
			}

			// Sent only when explicitly given so a zero value is never mistaken for
			// "cap every query at 0 bytes"; omitting it lets the API apply its default.
			if cmd.Flags().Changed(maximumBytesBilledFlag) {
				body["maximum_bytes_billed"] = maximumBytesBilled
			}

			path := api.ScopedVirtualGraphsPath(orgID, projectID)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:   http.MethodPost,
				PostBody: body,
				Version:  api.AuraApiVersion2,
			})
			if err != nil {
				return err
			}

			if statusCode != http.StatusAccepted && statusCode != http.StatusOK {
				return nil
			}

			responseData := api.ParseBody(resBody)
			virtualGraph, err := responseData.GetSingleOrError()
			if err != nil {
				return err
			}

			// The password is printed (once) for the user, but on a later --wait
			// failure the captured output is teed to disk. Register the literal value
			// so tee redaction scrubs it from formats the shape-based regexes can't
			// reach (notably the table-cell layout).
			if password, ok := virtualGraph["plain_password"].(string); ok {
				clievents.RegisterSecretValue(password)
			}

			output.PrintBodyMap(cmd, cfg, responseData, detailFieldsFor(virtualGraph, "plain_password"))

			if !wait {
				return nil
			}

			virtualGraphID, ok := virtualGraph["id"].(string)
			if !ok {
				return clierr.NewUpstreamError("create response did not carry a virtual graph id, cannot wait")
			}

			fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for virtual graph to be running...") //nolint:errcheck // narration to stderr; write errors are not actionable

			pollResponse, err := api.PollVirtualGraph(cfg, orgID, projectID, virtualGraphID, api.VirtualGraphStatusCreating)
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.ErrOrStderr(), "Virtual Graph Status:", pollResponse.Data.Status) //nolint:errcheck // narration to stderr; write errors are not actionable

			return nil
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) The name of the virtual graph (maximum 30 characters).")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&dataSourceID, dataSourceIDFlag, "", "(required) The ID of a data source created in Data Importer, for example a Databricks or Snowflake connector.")
	cmd.MarkFlagRequired(dataSourceIDFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&importModelID, importModelIDFlag, "", "(required) The ID of a graph data model stored in Data Importer. The model is fetched on your behalf to build the virtual graph.")
	cmd.MarkFlagRequired(importModelIDFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().Var(&cloudProvider, cloudProviderFlag, "(required) The cloud provider hosting the virtual graph.")
	cmd.MarkFlagRequired(cloudProviderFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&region, regionFlag, "", "(required) The cloud region hosting the virtual graph, for example europe-west1.")
	cmd.MarkFlagRequired(regionFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&memory, memoryFlag, "", "The memory allocation, for example 4Gi. Must be one of the values from 'aura virtual-graph allowed-configs'. Omit to use the project default.")

	cmd.Flags().Int64Var(&maximumBytesBilled, maximumBytesBilledFlag, 0, "Per-query bytes-billed cap for BigQuery data sources. Ignored for other data-source types.")

	auraflags.RegisterWait(cmd, &wait, "Waits until the created virtual graph is running.")

	return cmd
}
