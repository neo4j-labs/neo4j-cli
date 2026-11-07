// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package operator

import (
	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

// newInstallCmd builds the `neo4j-cli k8s operator install` leaf.
func newInstallCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		version         string
		watchNamespaces []string
		crdFile         string
	)

	const (
		versionFlag        = "version"
		watchNamespaceFlag = "watch-namespace"
		crdFileFlag        = "crd-file"
	)

	cmd := &cobra.Command{
		Use:         "install",
		Short:       "Install the Neo4j Kubernetes operator",
		Annotations: map[string]string{"write": "true"},
		Long: "Install the Neo4j Kubernetes operator: apply the four CRDs with server-side apply, then install the Helm chart " +
			"from the published OCI registry. The CRDs ship outside the chart (the Neo4j schema is close to the etcd object limit), " +
			"so they are always applied first, with `--field-manager=" + engine.OperatorFieldManager + "`. " +
			"The operator is installed as the Helm release `" + engine.OperatorReleaseName + "` in the namespace given by " +
			"-n/--namespace, or " + engine.DefaultOperatorNamespace + " when that flag is empty; the namespace is created if missing. " +
			"--watch-namespace is required: the chart refuses to render without a watch scope, and the operator never watches its own " +
			"namespace. Pass --crd-file to apply a CRD manifest from disk instead of the GitHub release asset. " +
			"The command does not wait for the controller to become ready; run `k8s operator status` afterwards. Requires --rw.",
		Example: `# Install the pinned operator release, watching the default namespace
neo4j-cli k8s operator install --watch-namespace default --rw

# Install a specific version and watch two namespaces
neo4j-cli k8s operator install --version 1.0.0 --watch-namespace team-a --watch-namespace team-b --rw

# Apply the CRDs from a local manifest instead of the GitHub release asset
neo4j-cli k8s operator install --watch-namespace default --crd-file ./neo4j-crd-1.0.0.yaml --rw`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			settings, err := operatorSettings(cmd, cfg)
			if err != nil {
				return err
			}
			version, err = normaliseVersion(version)
			if err != nil {
				return err
			}
			if err := validateCRDFile(cfg, crdFile); err != nil {
				return err
			}
			watchNamespaces = dedupeNamespaces(watchNamespaces)
			if err := validateWatchNamespaces(watchNamespaces, settings.Namespace, true); err != nil {
				return err
			}

			client := clientFactory(settings)
			if err := ensureWatchNamespacesExist(cmd.Context(), client, watchNamespaces); err != nil {
				return err
			}
			crds, err := client.ApplyCRDs(cmd.Context(), crdSource(crdFile, version))
			if err != nil {
				return err
			}
			if err := client.HelmUpgrade(cmd.Context(), engine.HelmUpgradeOptions{
				Version:         version,
				WatchNamespaces: watchNamespaces,
				Install:         true,
				CreateNamespace: true,
			}); err != nil {
				return err
			}

			report := OperatorChange{
				Release:         engine.OperatorReleaseName,
				Namespace:       settings.Namespace,
				Action:          "installed",
				ChartVersion:    version,
				WatchNamespaces: watchNamespaces,
				CRDs:            crds,
			}
			return renderReport(cmd, cfg, report, changeTable(report))
		},
	}

	cmd.Flags().StringVar(&version, versionFlag, engine.DefaultOperatorVersion, "Operator release to install (default: the version this CLI pins)")
	cmd.Flags().StringArrayVar(&watchNamespaces, watchNamespaceFlag, nil, "Namespace the operator watches for Neo4j resources (repeatable; at least one is required)")
	cmd.Flags().StringVar(&crdFile, crdFileFlag, "", "Apply the operator CRDs from this manifest instead of the GitHub release asset")

	return cmd
}
