// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package operator

import (
	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clicfg"
	k8sflags "github.com/neo4j/cli/internal/commands/k8s/flags"
	"github.com/spf13/cobra"
)

// newUpgradeCmd builds the `neo4j-cli k8s operator upgrade` leaf.
func newUpgradeCmd(cfg *clicfg.Config) *cobra.Command {
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
		Use:         "upgrade",
		Short:       "Upgrade the Neo4j Kubernetes operator",
		Annotations: map[string]string{"write": "true"},
		Long: "Upgrade an installed Neo4j Kubernetes operator. Re-apply the four CRDs with server-side apply, then run " +
			"`helm upgrade` for the `" + engine.OperatorReleaseName + "` release in the namespace given by -n/--namespace, or " +
			engine.DefaultOperatorNamespace + " when that flag is empty. " +
			"Without --version, the command moves to the release this CLI pins. It never rolls back a newer deployed release. " +
			"An explicit --version is honoured even when it is older. " +
			"The upgrade uses Helm's --reset-then-reuse-values flag, which requires Helm 3.14 or later. " +
			"Custom values such as a pinned image digest survive. Defaults added by a newer chart still apply. " +
			"A --watch-namespace value overrides the deployed watch scope. The release must already exist. " +
			"Use `k8s operator install` for a fresh installation. Pass --crd-file to apply a CRD manifest from disk instead of the GitHub release asset. " +
			"Before applying a remote CRD asset, the command matches its SHA256 digest against the GitHub release metadata. " +
			"The command does not wait for the controller to become ready. Run `k8s operator status` afterwards. Requires --rw.",
		Example: `# Upgrade to the operator release this CLI pins, keeping the deployed values
neo4j-cli k8s operator upgrade --rw

# Upgrade to a specific version and change the watched namespaces
neo4j-cli k8s operator upgrade --version 1.0.0 --watch-namespace team-a --watch-namespace team-b --rw

# Re-apply the CRDs from a local manifest before upgrading
neo4j-cli k8s operator upgrade --crd-file ./neo4j-crd-1.0.0.yaml --rw`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			settings, err := operatorSettings(cmd, cfg)
			if err != nil {
				return err
			}
			requested, err := normaliseVersion(version)
			if err != nil {
				return err
			}
			if err := validateCRDFile(cfg, crdFile); err != nil {
				return err
			}
			watchNamespaces = dedupeNamespaces(watchNamespaces)
			if err := validateWatchNamespaces(watchNamespaces, settings.Namespace, false); err != nil {
				return err
			}

			// Every check that can fail runs before the CRDs are applied, so
			// a refused upgrade leaves the cluster untouched.
			client := clientFactory(settings)
			if err := requireResetThenReuseHelm(cmd.Context(), client); err != nil {
				return err
			}
			release, err := requireRelease(cmd.Context(), client, settings.Namespace)
			if err != nil {
				return err
			}
			if err := requireWatchNamespacesExist(cmd.Context(), client, watchNamespaces); err != nil {
				return err
			}
			if err := k8sflags.PrintWriteTarget(cmd, client, settings.Namespace); err != nil {
				return err
			}
			target := upgradeVersion(requested, cmd.Flags().Changed(versionFlag), release)
			crds, err := client.ApplyCRDs(cmd.Context(), crdSource(crdFile, target))
			if err != nil {
				return err
			}
			if err := client.HelmUpgrade(cmd.Context(), engine.HelmUpgradeOptions{
				Version:         target,
				WatchNamespaces: watchNamespaces,
				ReuseValues:     true,
			}); err != nil {
				return err
			}

			report := OperatorChange{
				Release:         engine.OperatorReleaseName,
				Namespace:       settings.Namespace,
				Action:          "upgraded",
				ChartVersion:    target,
				WatchNamespaces: watchNamespaces,
				CRDs:            crds,
			}
			return renderReport(cmd, cfg, report, changeTable(report))
		},
	}

	cmd.Flags().StringVar(&version, versionFlag, engine.DefaultOperatorVersion, "Operator release to upgrade to (default: the version this CLI pins, or the deployed one if newer)")
	cmd.Flags().StringArrayVar(&watchNamespaces, watchNamespaceFlag, nil, "Change the namespaces the operator watches (repeatable, omit to keep the deployed scope)")
	cmd.Flags().StringVar(&crdFile, crdFileFlag, "", "Apply the operator CRDs from this manifest instead of the GitHub release asset")

	return cmd
}
