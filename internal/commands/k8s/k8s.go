// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package k8s implements the `neo4j-cli k8s` command group, behind the
// flag.k8s-commands feature flag: manage Neo4j on Kubernetes through the
// open-source Neo4j operator, by driving the host `kubectl` and `helm`.
package k8s

import (
	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/spf13/cobra"
)

// Flag names for the group's persistent flags.
const (
	kubeconfigFlag = "kubeconfig"
	contextFlag    = "context"
	namespaceFlag  = "namespace"
)

// clientFactory is the injectable seam for the engine.Client used by
// leaves. Production wires the exec-backed client. Tests swap in an
// engine.FakeClient without touching the leaf code.
var clientFactory = engine.NewClient

// settingsFrom copies the group's persistent flag values into the engine
// Settings that reach every kubectl and helm call. A --kubeconfig value
// that does not exist on disk is rejected here, before any child process
// runs, because kubectl's own error for it is a raw stat message.
func settingsFrom(cmd *cobra.Command, cfg *clicfg.Config, debug bool) (engine.Settings, error) {
	kubeconfig, _ := cmd.Flags().GetString(kubeconfigFlag)
	if kubeconfig != "" {
		if _, err := cfg.Fs().Stat(kubeconfig); err != nil {
			return engine.Settings{}, clierr.NewUsageError("--kubeconfig file %q does not exist", kubeconfig)
		}
	}
	kubeContext, _ := cmd.Flags().GetString(contextFlag)
	namespace, _ := cmd.Flags().GetString(namespaceFlag)
	return engine.Settings{
		Kubeconfig: kubeconfig,
		Context:    kubeContext,
		Namespace:  namespace,
		Debug:      debug,
	}, nil
}

// NewCmd returns the `k8s` parent command. The group is a preview: it is
// mounted on the root only while flag.k8s-commands is on.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "k8s",
		Short: "Manage Neo4j on Kubernetes through the Neo4j operator",
		Long: "Manage Neo4j on Kubernetes through the open-source Neo4j Kubernetes operator: install the operator, " +
			"create and inspect Neo4j servers and clusters, and connect to them. " +
			"The group drives the host `kubectl` and `helm` binaries, so both must be on PATH. " +
			"Pass --kubeconfig, --context and -n/--namespace on this group to scope every call. " +
			"This group is a preview behind the flag.k8s-commands feature flag. " +
			"Write commands will require --rw. Start with `k8s doctor` to check the local setup.",
	}

	cmd.PersistentFlags().String(kubeconfigFlag, "", "Path to the kubeconfig file to use (default: kubectl's own resolution)")
	cmd.PersistentFlags().String(contextFlag, "", "Kubernetes context to use (default: the current context)")
	cmd.PersistentFlags().StringP(namespaceFlag, "n", "", "Kubernetes namespace to scope calls to")
	cmd.PersistentFlags().Bool("debug", false, "Route kubectl and helm activity (the commands neo4j-cli shells out to, their exit status and captured stderr) to stderr; stdout is unaffected [env: NEO4J_DEBUG (set to 1 to enable)]")

	cmd.AddCommand(newDoctorCmd(cfg))

	return cmd
}
