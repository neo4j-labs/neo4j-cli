// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project

import (
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewUseCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:         "use <name>",
		Short:       "Sets the default project to be used",
		Annotations: map[string]string{"write": "true"},
		Long:        "Sets the default project to be used by other commands that require the organization and project ID flags. This allows running said commands without setting the flags explicitly as the values will be taken from the configuration",
		Example: `# Switch the default project used by subsequent aura commands
neo4j-cli aura config project use prod --rw

# Switch to a staging project before running write operations
neo4j-cli aura config project use staging --rw

# Switch and verify the default has been set
neo4j-cli aura config project use prod --rw && neo4j-cli aura config project list --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defaultProject, err := cfg.Aura.Projects.SetDefault(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Set %s as default project with organization ID %s and project ID %s\n", args[0], defaultProject.OrganizationId, defaultProject.ProjectId) //nolint:errcheck // narration to stderr; write errors are not actionable
			return nil
		},
	}
}
