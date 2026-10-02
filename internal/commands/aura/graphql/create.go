// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"fmt"
	"github.com/neo4j/cli/internal/aura"

	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	commonflags "github.com/neo4j/cli/internal/flags"
	"github.com/spf13/cobra"
)

func NewCreateCmd(cfg *clicfg.Config) *cobra.Command {
	const (
		instanceIdFlag     = "instance-id"
		nameFlag           = "name"
		serviceAccountFlag = "service-account"
		memoryFlag         = "memory"
		typeDefsFlag       = "type-definitions"
		typeDefsFileFlag   = "type-definitions-file"
	)

	var (
		instanceId     string
		name           string
		serviceAccount string
		memory         string
		typeDefs       string
		typeDefsFile   string
		wait           bool
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Creates a new GraphQL Data API",
		Long: `This command starts the creation process of a GraphQL Data API.

Creating a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready. Once the status transitions from "creating" to "ready" you may begin to use your GraphQL Data API.

This command returns your GraphQL Data API ID, API key, and connection URL for you to use once the GraphQL Data API is running. It is important to store the API key as it is not currently possible to get this or update it.

If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.`,
		Example: `# Create a GraphQL Data API (using flags)
neo4j-cli aura graphql create --instance-id 00000000 --name my-api --memory 256MB --type-definitions dHlwZSBNb3ZpZSB7IHRpdGxlOiBTdHJpbmcgfQ== --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Create a GraphQL Data API using a configured default workspace (auto-generated name)
neo4j-cli aura graphql create --instance-id 00000000 --memory 256MB --type-definitions-file ./typeDefs.graphql --rw

# Create a GraphQL Data API from a local type definitions file
neo4j-cli aura graphql create --instance-id 00000000 --name my-api --memory 512MB --type-definitions-file ./typeDefs.graphql --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Create a GraphQL Data API and wait until it is ready
neo4j-cli aura graphql create --instance-id 00000000 --name my-api --memory 256MB --type-definitions-file ./typeDefs.graphql --wait --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if serviceAccount != "read_only" && serviceAccount != "read_write" {
				return fmt.Errorf("invalid value for --service-account: %q, must be one of: read_only, read_write", serviceAccount)
			}

			switch memory {
			case "256MB", "512MB", "1024MB", "2048MB", "4096MB":
			default:
				return fmt.Errorf("invalid value for --memory: %q, must be one of: 256MB, 512MB, 1024MB, 2048MB, 4096MB", memory)
			}

			if _, err := utils.ResolveAndVerifyInstance(cmd, cfg, instanceId); err != nil {
				return err
			}

			typeDefsForBody, err := GetTypeDefsFromFlag(cfg, typeDefs, typeDefsFile)
			if err != nil {
				return err
			}

			g, err := aura.New(cfg).GraphQL().Create(cmd.Context(), instanceId, aura.GraphQLCreate{
				Name:            name,
				Memory:          memory,
				ServiceAccount:  serviceAccount,
				TypeDefinitions: typeDefsForBody,
			})
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.ErrOrStderr(), "###############################")                                                                                                                                            //nolint:errcheck // narration to stderr; write errors are not actionable
			fmt.Fprintln(cmd.ErrOrStderr(), "# It is important to store the created API key! If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.") //nolint:errcheck // narration to stderr; write errors are not actionable
			fmt.Fprintln(cmd.ErrOrStderr(), "###############################")                                                                                                                                            //nolint:errcheck // narration to stderr; write errors are not actionable

			output.PrintRecord(cmd, cfg, g.Record, []string{"id", "name", "status", "url", "authentication_providers"})

			if wait {
				fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for GraphQL Data API to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable
				status, err := aura.New(cfg).GraphQL().WaitWhile(cmd.Context(), instanceId, g.ID, aura.GraphQLStatusCreating)
				if err != nil {
					return err
				}

				fmt.Fprintln(cmd.ErrOrStderr(), "GraphQL Data API Status:", status) //nolint:errcheck // narration to stderr; write errors are not actionable
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, instanceIdFlag, "", "(required) The ID of the instance to create the GraphQL Data API for")
	cmd.MarkFlagRequired(instanceIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&serviceAccount, serviceAccountFlag, "read_write", "The service account type for the instance connection, must be one of: read_only, read_write")

	cmd.Flags().StringVar(&memory, memoryFlag, "", "(required) Memory allocated to the GraphQL Data API, must be one of: 256MB, 512MB, 1024MB, 2048MB, 4096MB")
	cmd.MarkFlagRequired(memoryFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&name, nameFlag, "", "The name of the GraphQL Data API (auto-generated if not specified)")

	cmd.Flags().StringVar(&typeDefs, typeDefsFlag, "", "The GraphQL type definitions, NOTE: must be base64 encoded")

	cmd.Flags().StringVar(&typeDefsFile, typeDefsFileFlag, "", "Path to a local GraphQL type definitions file, e.g. path/to/typeDefs.graphql. Must be of file type .graphql")
	cmd.MarkFlagsMutuallyExclusive(typeDefsFlag, typeDefsFileFlag)
	cmd.MarkFlagsOneRequired(typeDefsFlag, typeDefsFileFlag)

	commonflags.RegisterWait(cmd, &wait, "Waits until created GraphQL Data API is ready.")

	return cmd
}

// resolveGraphQLName returns the explicit name when non-empty, otherwise it
// lists the instance's GraphQL data APIs and derives an unused default name
// (e.g. GraphQL01). Shared by create's auto-naming path.
