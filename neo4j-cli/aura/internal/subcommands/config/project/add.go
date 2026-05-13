// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewAddCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name           string
		organizationId string
		projectId      string
	)

	const (
		nameFlag           = "name"
		organizationIdFlag = "organization-id"
		projectIdFlag      = "project-id"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "add",
		Short:       "Adds a project",
		Example: `# Add a project configuration (becomes the default if it is the first one)
neo4j-cli aura config project add --name prod --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw

# Add a second project alongside an existing default
neo4j-cli aura config project add --name staging --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw

# Add a project and emit the response as JSON
neo4j-cli aura config project add --name prod --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Aura.Projects.Add(name, organizationId, projectId)
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Name")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "(required) Oragnization ID")
	cmd.MarkFlagRequired(organizationIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "(required) Project ID")
	cmd.MarkFlagRequired(projectIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
