// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package flags owns the `k8s` group's persistent flags. It also owns the
// function that copies them into the engine settings for each leaf, and the
// line every write prints to name its target. Every kubectl and helm call
// receives those settings. The separate package lets the group root and its
// subtrees share one definition without importing each other.
package flags

import (
	"fmt"
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

// RegisterPersistentFlags registers the shared flags for the k8s group on cmd.
func RegisterPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String(KubeconfigFlag, "", "Path to the kubeconfig file to use (default: kubectl default resolution)")
	cmd.PersistentFlags().String(ContextFlag, "", "Kubernetes context to use (default: the current context)")
	cmd.PersistentFlags().StringP(NamespaceFlag, "n", "", "Kubernetes namespace to scope calls to")
	cmd.PersistentFlags().Bool(DebugFlag, false, "Write kubectl and helm commands, exit codes and stderr to stderr. Stdout is unchanged [env: NEO4J_DEBUG=1]")
}

// SettingsFrom copies the persistent flag values from cmd into engine
// Settings. If --kubeconfig does not exist on disk or --namespace is not a DNS
// label, SettingsFrom returns a usage error before any child process runs.
func SettingsFrom(cmd *cobra.Command, cfg *clicfg.Config) (engine.Settings, error) {
	kubeconfig, _ := cmd.Flags().GetString(KubeconfigFlag)
	if kubeconfig != "" {
		if _, err := cfg.Fs().Stat(kubeconfig); err != nil {
			return engine.Settings{}, clierr.NewUsageError("--kubeconfig file %q does not exist", kubeconfig)
		}
	}
	kubeContext, _ := cmd.Flags().GetString(ContextFlag)
	namespace, _ := cmd.Flags().GetString(NamespaceFlag)
	if namespace != "" {
		if err := engine.ValidateResourceName("--namespace", namespace); err != nil {
			return engine.Settings{}, err
		}
	}
	return engine.Settings{
		Kubeconfig: kubeconfig,
		Context:    kubeContext,
		Namespace:  namespace,
		Debug:      debug.Resolve(cmd),
	}, nil
}

// PrintWriteTarget names the kube context and namespace on stderr before a
// write. Kubeconfigs often hold production contexts, so a write against the
// wrong cluster is then visible at once.
func PrintWriteTarget(cmd *cobra.Command, client engine.Client, namespace string) error {
	kubeContext, err := client.CurrentContext(cmd.Context())
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.ErrOrStderr(), "Targeting Kubernetes context %q in namespace %q.\n", kubeContext, namespace)
	return err
}
