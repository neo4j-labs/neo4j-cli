// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package flags owns the `k8s` group's persistent flags and the one function
// that copies them into the engine settings every leaf passes to kubectl and
// helm. It is a separate package so the group root, `doctor` and the
// `operator` subtree share one definition without importing each other.
package flags

import (
	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/debug"
	"github.com/spf13/cobra"
)

// Flag names for the group's persistent flags.
const (
	KubeconfigFlag = "kubeconfig"
	ContextFlag    = "context"
	NamespaceFlag  = "namespace"
	DebugFlag      = "debug"
)

// RegisterPersistentFlags registers the group's shared flags on cmd.
func RegisterPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String(KubeconfigFlag, "", "Path to the kubeconfig file to use (default: kubectl's own resolution)")
	cmd.PersistentFlags().String(ContextFlag, "", "Kubernetes context to use (default: the current context)")
	cmd.PersistentFlags().StringP(NamespaceFlag, "n", "", "Kubernetes namespace to scope calls to")
	cmd.PersistentFlags().Bool(DebugFlag, false, "Route kubectl and helm activity (the commands neo4j-cli shells out to, their exit status and captured stderr) to stderr; stdout is unaffected [env: NEO4J_DEBUG (set to 1 to enable)]")
}

// SettingsFrom copies the group's persistent flag values into the engine
// Settings that reach every kubectl and helm call. A --kubeconfig value
// that does not exist on disk is rejected here, before any child process
// runs, because kubectl's own error for it is a raw stat message.
func SettingsFrom(cmd *cobra.Command, cfg *clicfg.Config) (engine.Settings, error) {
	kubeconfig, _ := cmd.Flags().GetString(KubeconfigFlag)
	if kubeconfig != "" {
		if _, err := cfg.Fs().Stat(kubeconfig); err != nil {
			return engine.Settings{}, clierr.NewUsageError("--kubeconfig file %q does not exist", kubeconfig)
		}
	}
	kubeContext, _ := cmd.Flags().GetString(ContextFlag)
	namespace, _ := cmd.Flags().GetString(NamespaceFlag)
	return engine.Settings{
		Kubeconfig: kubeconfig,
		Context:    kubeContext,
		Namespace:  namespace,
		Debug:      debug.Resolve(cmd),
	}, nil
}
