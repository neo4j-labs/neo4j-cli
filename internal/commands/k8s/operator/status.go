// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package operator

import (
	"context"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

// newStatusCmd builds the read-only `neo4j-cli k8s operator status` leaf.
func newStatusCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the Neo4j operator's release, CRDs and controller health",
		Long: "Show the state of the Neo4j Kubernetes operator: the `" + engine.OperatorReleaseName + "` Helm release in the namespace " +
			"given by -n/--namespace (or " + engine.DefaultOperatorNamespace + "), the chart and app version, the watch scope read from the " +
			"controller Deployment, whether the controller is ready, and the versions of the four CRDs. " +
			"A missing release, controller Deployment or CRD is reported as absent rather than an error, so the command is safe to run " +
			"before the operator is installed. It exits 0 whatever it finds; parse `installed` and `deployment.ready` to gate downstream " +
			"actions. An interrupt still exits 130.",
		Example: `# Show the operator's release, watch scope, controller health and CRD versions
neo4j-cli k8s operator status

# Emit the report as JSON for scripting
neo4j-cli k8s operator status --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			settings, err := operatorSettings(cmd, cfg)
			if err != nil {
				return err
			}
			report, err := buildStatus(cmd.Context(), clientFactory(settings), settings.Namespace)
			if err != nil {
				return err
			}
			// A signal killed a child mid-read, so the report describes the
			// interrupt, not the cluster.
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			return renderReport(cmd, cfg, report, statusTable(report))
		},
	}
}

// buildStatus reads the release, the controller Deployment and the four
// CRDs. A missing release, Deployment or CRD is reported as absent; any
// other failure propagates.
func buildStatus(ctx context.Context, client engine.Client, namespace string) (OperatorStatus, error) {
	report := OperatorStatus{
		Release:   engine.OperatorReleaseName,
		Namespace: namespace,
	}

	releases, err := client.HelmReleases(ctx)
	if err != nil {
		return OperatorStatus{}, err
	}
	release, installed := engine.FindOperatorRelease(releases)
	report.Installed = installed
	if installed {
		report.Status = release.Status
		report.ChartVersion = release.ChartVersion()
		report.AppVersion = release.AppVersion
		report.Revision = release.Revision

		deployment, err := client.Deployment(ctx, engine.OperatorDeploymentName)
		switch {
		case err == nil:
			report.Deployment = &DeploymentReport{
				Name:          deployment.Name,
				Ready:         deployment.Available,
				Replicas:      deployment.Replicas,
				ReadyReplicas: deployment.ReadyReplicas,
				Message:       deployment.AvailableMessage,
			}
			report.WatchNamespaces = deployment.WatchNamespaces
			report.WatchAllNamespaces = deployment.WatchAllNamespaces
		case engine.IsNotFound(err):
			// The release exists but its Deployment is gone; report it as
			// absent and let the operator decide what to do.
		default:
			return OperatorStatus{}, err
		}
	}

	for _, name := range engine.OperatorCRDNames {
		info, err := client.CRD(ctx, name)
		switch {
		case err == nil:
			crdName := info.Name
			if crdName == "" {
				crdName = name
			}
			report.CRDs = append(report.CRDs, CRDReport{
				Name:           crdName,
				Installed:      true,
				Versions:       info.Versions,
				StoredVersions: info.StoredVersions,
			})
		case engine.IsNotFound(err):
			report.CRDs = append(report.CRDs, CRDReport{Name: name})
		default:
			return OperatorStatus{}, err
		}
	}

	if !installed {
		report.Hint = "install it with `neo4j-cli k8s operator install --watch-namespace <namespace> --rw`"
	}
	return report, nil
}
