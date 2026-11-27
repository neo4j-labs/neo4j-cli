// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"context"
	"slices"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	k8sflags "github.com/neo4j/cli/internal/commands/k8s/flags"
	engine "github.com/neo4j/cli/internal/k8s"
	"github.com/spf13/cobra"
)

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

// ensureNamespaceWatched fails unless the operator is installed, ready and
// watching the workload namespace. Without this step, nothing reconciles the
// new resource and --wait only times out.
func ensureNamespaceWatched(ctx context.Context, operator engine.Client, namespace, operatorNamespace string) error {
	deployment, err := operator.Deployment(ctx, engine.OperatorDeploymentName)
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
			"the Neo4j operator controller is not ready: run `neo4j-cli k8s operator status -n %s`, then retry",
			operatorNamespace)
	}
	if deployment.WatchAllNamespaces || slices.Contains(deployment.WatchNamespaces, namespace) {
		return nil
	}
	scope := "none"
	if len(deployment.WatchNamespaces) > 0 {
		scope = strings.Join(deployment.WatchNamespaces, ", ")
	}
	return clierr.NewConflictError(
		"namespace %q is not in the operator's watch scope (%s): add it with `neo4j-cli k8s operator upgrade -n %s %s --rw`",
		namespace, scope, operatorNamespace, watchNamespaceFlags(deployment.WatchNamespaces, namespace))
}

func watchNamespaceFlags(existing []string, extra string) string {
	flags := make([]string, 0, len(existing)+1)
	for _, ns := range existing {
		flags = append(flags, "--watch-namespace "+ns)
	}
	if !slices.Contains(existing, extra) {
		flags = append(flags, "--watch-namespace "+extra)
	}
	return strings.Join(flags, " ")
}
