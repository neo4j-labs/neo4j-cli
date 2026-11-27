// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

func newGetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Get one Neo4j resource",
		Long: "Get one Neo4j custom resource by name from the group's workload namespace, or from `default` when no namespace is set. " +
			"JSON and TOON output contain the full server object returned by Kubernetes. " +
			"Table output summarises identity, topology, version, phase and readiness.",
		Example: `# Inspect a Neo4j resource
neo4j-cli k8s instance get dev

# Emit the full server object as JSON
neo4j-cli k8s instance get dev --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			settings, err := instanceSettings(cmd, cfg)
			if err != nil {
				return err
			}
			instance, err := clientFactory(settings).GetNeo4j(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			printInstance(cmd, cfg, instance)
			return nil
		},
	}
}
