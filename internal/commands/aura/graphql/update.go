// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

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

func NewUpdateCmd(cfg *clicfg.Config) *cobra.Command {
	const (
		instanceIdFlag     = "instance-id"
		nameFlag           = "name"
		serviceAccountFlag = "service-account"
		typeDefsFlag       = "type-definitions"
		typeDefsFileFlag   = "type-definitions-file"
	)

	var (
		instanceId     string
		name           string
		serviceAccount string
		typeDefs       string
		typeDefsFile   string
		wait           bool
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "update <id>",
		Short:       "Edit a GraphQL Data API",
		Long: `This endpoint edits a specific GraphQL Data API.

Updating a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready again. Once the status transitions from "updating" to "ready" you may continue to use your GraphQL Data API.`,
		Example: `# Rename a GraphQL Data API (using flags)
neo4j-cli aura graphql update 11111111 --instance-id 00000000 --name renamed-api --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Rename a GraphQL Data API using a configured default workspace
neo4j-cli aura graphql update 11111111 --instance-id 00000000 --name renamed-api --rw

# Update the service account permission and wait for the API to be ready
neo4j-cli aura graphql update 11111111 --instance-id 00000000 --service-account read_only --wait --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			graphqlId := strings.TrimSpace(args[0])

			if serviceAccount != "" && serviceAccount != "read_only" && serviceAccount != "read_write" {
				return fmt.Errorf("invalid --service-account value %q: must be read_only or read_write", serviceAccount)
			}

			if _, err := utils.ResolveAndVerifyInstance(cmd, cfg, instanceId); err != nil {
				return err
			}

			patch := aura.GraphQLPatch{Name: name, ServiceAccount: serviceAccount}
			if typeDefs != "" || typeDefsFile != "" {
				base64EncodedTypeDefs, err := GetTypeDefsFromFlag(cfg, typeDefs, typeDefsFile)
				if err != nil {
					return err
				}
				patch.TypeDefinitions = base64EncodedTypeDefs
			}

			g, err := aura.New(cfg).GraphQL().Update(cmd.Context(), instanceId, graphqlId, patch)
			if err != nil {
				return err
			}
			output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(g.Record), []string{"id", "name", "status", "url"})

			if wait {
				fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for GraphQL Data API to be updated...") //nolint:errcheck // narration to stderr; write errors are not actionable
				status, err := aura.New(cfg).GraphQL().WaitWhile(cmd.Context(), instanceId, graphqlId, aura.GraphQLStatusUpdating)
				if err != nil {
					return err
				}

				fmt.Fprintln(cmd.ErrOrStderr(), "GraphQL Data API Status:", status) //nolint:errcheck // narration to stderr; write errors are not actionable
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, instanceIdFlag, "", "(required) The ID of the instance to update the Data API for")
	cmd.MarkFlagRequired(instanceIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&name, nameFlag, "", "The name of the GraphQL Data API")

	cmd.Flags().StringVar(&serviceAccount, serviceAccountFlag, "", "The service account permission for the instance this GraphQL Data API will be connected to (read_only or read_write)")

	cmd.Flags().StringVar(&typeDefs, typeDefsFlag, "", "The GraphQL type definitions, NOTE: must be base64 encoded")

	cmd.Flags().StringVar(&typeDefsFile, typeDefsFileFlag, "", "Path to a local GraphQL type definitions file, e.g. path/to/typeDefs.graphql")
	cmd.MarkFlagsMutuallyExclusive(typeDefsFlag, typeDefsFileFlag)

	commonflags.RegisterWait(cmd, &wait, "Waits until updated GraphQL Data API is ready again.")

	return cmd
}
