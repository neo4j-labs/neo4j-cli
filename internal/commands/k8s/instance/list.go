// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Neo4j resources managed by neo4j-cli",
		Long: "List Neo4j resources that carry the `app.kubernetes.io/managed-by=neo4j-cli` label in the workload namespace from -n, " +
			"or in `default` when no namespace is set. Hand-written resources without that label do not appear. " +
			"JSON and TOON output contain the full server objects returned by Kubernetes. An empty result is an empty array.",
		Example: `# List CLI-managed Neo4j resources
neo4j-cli k8s instance list

# Emit the full server objects as JSON
neo4j-cli k8s instance list --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			settings, err := instanceSettings(cmd, cfg)
			if err != nil {
				return err
			}
			instances, err := clientFactory(settings).ListNeo4j(cmd.Context())
			if err != nil {
				return err
			}
			printInstances(cmd, cfg, instances)
			return nil
		},
	}
}
