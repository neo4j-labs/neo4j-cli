// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package instance implements the `neo4j-cli k8s instance` subtree.
package instance

import (
	"time"

	"github.com/neo4j/cli/internal/clicfg"
	engine "github.com/neo4j/cli/internal/k8s"
	"github.com/spf13/cobra"
)

var clientFactory = engine.NewClient

var waitTimeout = 10 * time.Minute

// NewCmd returns the `k8s instance` parent command.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance",
		Short: "Create and manage Neo4j resources on Kubernetes",
		Long: "Create, inspect and delete Neo4j standalone servers and clusters through the Neo4j Kubernetes operator. " +
			"Store credentials and connect to standalone servers. " +
			"The commands drive the host `kubectl` binary, so it must be on PATH. " +
			"The -n flag on the group selects the workload namespace, or `default` when no namespace is set. " +
			"Create makes sure that the operator watches that namespace before creating the resource. " +
			"--operator-namespace selects the controller installation when it is not in " + engine.DefaultOperatorNamespace + ". " +
			"Resource JSON and TOON output contain the full Neo4j server object returned by Kubernetes.",
	}
	cmd.AddCommand(newCreateCmd(cfg))
	cmd.AddCommand(newGetCmd(cfg))
	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newDeleteCmd(cfg))
	cmd.AddCommand(newCredentialCmd(cfg))
	cmd.AddCommand(newConnectCmd(cfg))
	return cmd
}
