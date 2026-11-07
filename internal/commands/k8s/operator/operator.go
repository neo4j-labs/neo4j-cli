// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package operator implements the `neo4j-cli k8s operator` subtree: install,
// inspect, upgrade and uninstall the Neo4j Kubernetes operator through the
// host helm and kubectl.
package operator

import (
	"context"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	k8sflags "github.com/neo4j/cli/internal/commands/k8s/flags"
	"github.com/spf13/cobra"
)

// clientFactory is the injectable seam for the engine.Client used by the
// leaves. Production wires the exec-backed client; tests swap in
// engine.FakeClient so no child process ever runs.
var clientFactory = engine.NewClient

// NewCmd returns the `k8s operator` parent command.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Install and manage the Neo4j Kubernetes operator",
		Long: "Install, inspect, upgrade and uninstall the open-source Neo4j Kubernetes operator. " +
			"The commands drive the host `helm` and `kubectl`, so both must be on PATH. " +
			"The operator lives in the namespace given by the group's -n/--namespace flag, or in " +
			engine.DefaultOperatorNamespace + " when that flag is empty. " +
			"The operator watches the workload namespaces you pass as --watch-namespace; it never watches its own namespace. " +
			"The four CRDs ship outside the chart (the Neo4j schema is close to the etcd object limit), so every install and " +
			"upgrade applies them first with `kubectl apply --server-side --field-manager=" + engine.OperatorFieldManager + "`. " +
			"Run `k8s doctor` first to check kubectl, helm and the cluster.",
	}

	cmd.AddCommand(newInstallCmd(cfg))
	cmd.AddCommand(newStatusCmd(cfg))
	cmd.AddCommand(newUpgradeCmd(cfg))
	cmd.AddCommand(newUninstallCmd(cfg))

	return cmd
}

// operatorSettings resolves the group's settings and settles the operator
// namespace: the group's -n/--namespace value, or the default when it is
// empty. Every operator command scopes helm and kubectl to that namespace.
func operatorSettings(cmd *cobra.Command, cfg *clicfg.Config) (engine.Settings, error) {
	settings, err := k8sflags.SettingsFrom(cmd, cfg)
	if err != nil {
		return engine.Settings{}, err
	}
	if settings.Namespace == "" {
		settings.Namespace = engine.DefaultOperatorNamespace
	}
	return settings, nil
}

// crdSource returns the manifest kubectl applies: the --crd-file path when
// set, otherwise the pinned release asset URL.
func crdSource(crdFile, version string) string {
	if crdFile != "" {
		return crdFile
	}
	return engine.OperatorCRDManifest(version)
}

// validateCRDFile rejects a --crd-file that does not exist before any child
// process runs.
func validateCRDFile(cfg *clicfg.Config, path string) error {
	if path == "" {
		return nil
	}
	if _, err := cfg.Fs().Stat(path); err != nil {
		return clierr.NewUsageError("--crd-file %q does not exist", path)
	}
	return nil
}

// The first helm release with --reset-then-reuse-values, which
// `operator upgrade` relies on.
const minResetThenReuseHelmMajor, minResetThenReuseHelmMinor = 3, 14

// normaliseVersion checks a --version value and strips a leading "v", so the
// asset URL and the chart version are built from the same bare version.
func normaliseVersion(version string) (string, error) {
	normalised, ok := engine.NormaliseReleaseVersion(version)
	if !ok {
		return "", clierr.NewUsageError("--version %q is not an operator release version such as %s", version, engine.DefaultOperatorVersion)
	}
	return normalised, nil
}

// requireRelease returns the operator's Helm release, or a not-found error
// with an install hint when there is none.
func requireRelease(ctx context.Context, client engine.Client, namespace string) (engine.HelmRelease, error) {
	releases, err := client.HelmReleases(ctx)
	if err != nil {
		return engine.HelmRelease{}, err
	}
	release, ok := engine.FindOperatorRelease(releases)
	if !ok {
		return engine.HelmRelease{}, clierr.NewNotFoundError(
			"the operator release %q is not installed in namespace %q: install it with `neo4j-cli k8s operator install --watch-namespace <namespace> --rw`",
			engine.OperatorReleaseName, namespace)
	}
	return release, nil
}

// requireReadyController fails unless the controller Deployment exists and
// is ready. Deleting the CRDs relies on the controller to clear its
// finalisers; without it the delete waits forever.
func requireReadyController(ctx context.Context, client engine.Client) error {
	deployment, err := client.Deployment(ctx, engine.OperatorDeploymentName)
	if engine.IsNotFound(err) {
		return clierr.NewConflictError(
			"the operator controller Deployment %q is missing, so nothing would clear the finalisers on the Neo4j resources and the CRD delete would never finish: reinstall the operator, then retry",
			engine.OperatorDeploymentName)
	}
	if err != nil {
		return err
	}
	if !deployment.Available {
		return clierr.NewConflictError(
			"the operator controller is not ready, so the finalisers on the Neo4j resources would not clear and the CRD delete would never finish: check `neo4j-cli k8s operator status`, then retry")
	}
	return nil
}

// requireResetThenReuseHelm fails with a clear message when the host helm is
// older than the --reset-then-reuse-values flag the upgrade passes.
func requireResetThenReuseHelm(ctx context.Context, client engine.Client) error {
	version, err := client.HelmVersion(ctx)
	if err != nil {
		return err
	}
	if !engine.MajorMinorAtLeast(version, minResetThenReuseHelmMajor, minResetThenReuseHelmMinor) {
		return clierr.NewUsageError("operator upgrade needs helm v%d.%d or later (found %s) for --reset-then-reuse-values",
			minResetThenReuseHelmMajor, minResetThenReuseHelmMinor, version)
	}
	return nil
}

// upgradeVersion picks the chart version an upgrade targets. An explicit
// --version always wins, even when it is a downgrade. Otherwise the CLI's
// pinned version is used, unless the deployed release is newer: a scope
// change must never silently roll the operator and its CRDs back.
func upgradeVersion(requested string, explicit bool, deployed engine.HelmRelease) string {
	if explicit {
		return requested
	}
	if cmp, ok := engine.CompareReleaseVersions(deployed.ChartVersion(), requested); ok && cmp > 0 {
		return deployed.ChartVersion()
	}
	return requested
}

// validateWatchNamespaces applies the chart's scope rules locally, so a
// mistake is a usage error before kubectl or helm runs. requireScope is set
// on install, where the chart refuses to render without a scope.
func validateWatchNamespaces(namespaces []string, operatorNamespace string, requireScope bool) error {
	if len(namespaces) == 0 {
		if requireScope {
			return clierr.NewUsageError("--watch-namespace is required: the operator chart refuses to render without a watch scope")
		}
		return nil
	}
	for _, namespace := range namespaces {
		if namespace == operatorNamespace {
			return clierr.NewUsageError(
				"--watch-namespace must not include the operator namespace %q: the operator never watches its own namespace",
				operatorNamespace)
		}
	}
	return nil
}

// ensureWatchNamespacesExist fails fast when a watched namespace is absent:
// the chart creates a Role and RoleBinding in each watched namespace, so helm
// otherwise fails with a raw `namespaces "<name>" not found` after the CRDs
// are already applied.
func ensureWatchNamespacesExist(ctx context.Context, client engine.Client, namespaces []string) error {
	for _, namespace := range namespaces {
		exists, err := client.NamespaceExists(ctx, namespace)
		if err != nil {
			return err
		}
		if !exists {
			return clierr.NewUsageError(
				"namespace %q does not exist: create it first with `kubectl create namespace %s`",
				namespace, namespace)
		}
	}
	return nil
}

// dedupeNamespaces removes repeated --watch-namespace values while keeping
// the order the user gave, so the helm --set literal stays predictable. An
// empty input stays nil, so an upgrade that omits the flag omits --set.
func dedupeNamespaces(namespaces []string) []string {
	if len(namespaces) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(namespaces))
	out := make([]string, 0, len(namespaces))
	for _, namespace := range namespaces {
		if seen[namespace] {
			continue
		}
		seen[namespace] = true
		out = append(out, namespace)
	}
	return out
}
