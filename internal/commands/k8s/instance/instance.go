// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package instance implements the `neo4j-cli k8s instance` subtree.
package instance

import (
	"context"
	"fmt"
	"slices"
	"time"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	k8sflags "github.com/neo4j/cli/internal/commands/k8s/flags"
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
			"The commands drive the host `kubectl`, so it must be on PATH. " +
			"The group's -n/--namespace selects the workload namespace, or `default` when it is empty. " +
			"Create checks that the operator watches that namespace before it creates the resource. " +
			"--operator-namespace selects the controller installation when it is not in " + engine.DefaultOperatorNamespace + ". " +
			"JSON and TOON output contain the full Neo4j server object returned by Kubernetes.",
	}
	cmd.AddCommand(newCreateCmd(cfg))
	cmd.AddCommand(newGetCmd(cfg))
	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newDeleteCmd(cfg))
	return cmd
}

func instanceSettings(cmd *cobra.Command, cfg *clicfg.Config) (engine.Settings, error) {
	settings, err := k8sflags.SettingsFrom(cmd, cfg)
	if err != nil {
		return engine.Settings{}, err
	}
	if settings.Namespace == "" {
		settings.Namespace = "default"
	}
	return settings, nil
}

func printTarget(cmd *cobra.Command, settings engine.Settings) error {
	contextName, err := clientFactory(settings).CurrentContext(cmd.Context())
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.ErrOrStderr(), "Targeting Kubernetes context %q in namespace %q.\n", contextName, settings.Namespace)
	return err
}

func ensureNamespaceWatched(ctx context.Context, client engine.Client, namespace, operatorNamespace string) error {
	deployment, err := client.OperatorDeployment(ctx, operatorNamespace)
	if engine.IsNotFound(err) {
		return clierr.NewConflictError(
			"the Neo4j operator is not installed in namespace %q: install it with `neo4j-cli k8s operator install -n %s --watch-namespace %s --rw`",
			operatorNamespace, operatorNamespace, namespace)
	}
	if err != nil {
		return err
	}
	if !deployment.Available {
		return clierr.NewConflictError(
			"the Neo4j operator controller is not ready: check `neo4j-cli k8s operator status -n %s`, then retry",
			operatorNamespace)
	}
	if deployment.WatchAllNamespaces || slices.Contains(deployment.WatchNamespaces, namespace) {
		return nil
	}
	return clierr.NewConflictError(
		"namespace %q is not in the operator's watch scope %v: add it with `neo4j-cli k8s operator upgrade -n %s --watch-namespace %s --rw`",
		namespace, deployment.WatchNamespaces, operatorNamespace, namespace)
}
