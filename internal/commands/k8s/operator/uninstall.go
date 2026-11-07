// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package operator

import (
	"fmt"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/spf13/cobra"
)

// newUninstallCmd builds the `neo4j-cli k8s operator uninstall` leaf.
func newUninstallCmd(cfg *clicfg.Config) *cobra.Command {
	var removeCRDs bool

	const removeCRDsFlag = "remove-crds"

	cmd := &cobra.Command{
		Use:         "uninstall",
		Short:       "Uninstall the Neo4j Kubernetes operator",
		Annotations: map[string]string{"write": "true"},
		Long: "Remove the `" + engine.OperatorReleaseName + "` Helm release from the namespace given by -n/--namespace, or " +
			engine.DefaultOperatorNamespace + " when that flag is empty. " +
			"By default the four CRDs and every Neo4j custom resource are left alone, so a reinstall finds them again. " +
			"Pass --remove-crds to delete the CRDs too: that deletes EVERY Neo4j, backup, restore and backup schedule in the cluster, " +
			"and the workloads and volumes behind them. It is irreversible, so it needs a y/N confirmation on a terminal, or both --yes " +
			"and --force without one. The CRDs are deleted before the release, while the controller still runs, so its finalisers clear " +
			"the custom resources instead of leaving them stuck; for the same reason --remove-crds refuses to run unless the release is " +
			"installed and its controller is ready. Requires --rw.",
		Example: `# Remove the Helm release, keeping the CRDs and every Neo4j custom resource
neo4j-cli k8s operator uninstall --rw

# Also delete the CRDs, and with them every Neo4j in the cluster
neo4j-cli k8s operator uninstall --remove-crds --yes --force --rw`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			settings, err := operatorSettings(cmd, cfg)
			if err != nil {
				return err
			}
			client := clientFactory(settings)
			if removeCRDs {
				if err := preflightRemoveCRDs(cmd, client, settings.Namespace); err != nil {
					return err
				}
			}

			report := OperatorRemoval{
				Release:   engine.OperatorReleaseName,
				Namespace: settings.Namespace,
			}
			if removeCRDs {
				removed, err := client.DeleteCRDs(cmd.Context())
				if err != nil {
					return err
				}
				report.CRDsRemoved = removed
			}
			if err := client.HelmUninstall(cmd.Context()); err != nil {
				return err
			}
			return renderReport(cmd, cfg, report, removalTable(report))
		},
	}

	cmd.Flags().BoolVar(&removeCRDs, removeCRDsFlag, false, "Also delete the operator CRDs, which deletes every Neo4j, backup, restore and backup schedule in the cluster")
	confirm.Register(cmd)

	return cmd
}

// preflightRemoveCRDs runs before any CRD is touched. Deleting the CRDs waits
// for the controller to clear its finalisers on every Neo4j resource, so the
// release and a ready controller must both be present, otherwise the delete
// hangs and leaves the CRDs stuck Terminating. Only then does it ask for
// confirmation, naming the kube context the delete will reach.
func preflightRemoveCRDs(cmd *cobra.Command, client engine.Client, namespace string) error {
	ctx := cmd.Context()
	if _, err := requireRelease(ctx, client, namespace); err != nil {
		return err
	}
	if err := requireReadyController(ctx, client); err != nil {
		return err
	}
	if confirm.Scripted(cmd) {
		return nil
	}
	kubeContext, err := client.CurrentContext(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"Removing the operator CRDs deletes every Neo4j, backup, restore and backup schedule in kube context %q, "+
			"and the workloads and volumes behind them.\n", kubeContext)
	return confirm.RequireTyped(cmd, "operator CRDs in kube context", kubeContext)
}
