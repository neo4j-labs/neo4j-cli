// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"

	engine "github.com/neo4j/cli/internal/k8s"
)

// runChecks executes the seven health checks in order:
//
//	kubectl_present → kubectl_version → helm_present → helm_version
//	  → cluster_reachable → kubernetes_version → version_skew
//
// The cluster checks read one `kubectl version -o json` call, made only
// when kubectl is present. The helm checks read one `helm version --short`
// call, made only when helm is present. The kubectl version floor does not
// gate the cluster checks: an older kubectl can still reach a healthy
// cluster, and the reachability detail is the more useful diagnostic.
func runChecks(ctx context.Context, client engine.Client) DoctorReport {
	checks := make([]CheckResult, 0, 7)

	kubectlPresent := engine.KubectlPresent()
	checks = append(checks, checkBinaryPresent(CheckKubectlPresent, "kubectl", LabelKubectlPresent, kubectlPresent))

	var (
		vi         engine.VersionInfo
		kubectlErr error
	)
	if kubectlPresent {
		vi, kubectlErr = client.KubectlVersion(ctx)
	}
	checks = append(checks, checkKubectlVersion(vi, kubectlErr, kubectlPresent))

	helmPresent := engine.HelmPresent()
	checks = append(checks, checkBinaryPresent(CheckHelmPresent, "helm", LabelHelmPresent, helmPresent))

	var (
		helmVersion string
		helmErr     error
	)
	if helmPresent {
		helmVersion, helmErr = client.HelmVersion(ctx)
	}
	checks = append(checks, checkHelmVersion(helmVersion, helmErr, helmPresent))

	reachable := kubectlPresent && kubectlErr == nil && vi.ServerVersion != ""
	checks = append(checks, checkClusterReachable(vi, kubectlErr, kubectlPresent))
	checks = append(checks, checkKubernetesVersion(vi, reachable))
	checks = append(checks, checkVersionSkew(vi, reachable))

	return DoctorReport{
		Checks:  checks,
		Summary: buildSummary(checks),
	}
}

// buildSummary derives the verdict from the per-check results. Ready is
// true when no check failed. NextStep carries the hint of the first
// failing check.
func buildSummary(checks []CheckResult) DoctorSummary {
	summary := DoctorSummary{Ready: true}
	for _, c := range checks {
		if c.Status == StatusFail {
			summary.Ready = false
		}
		switch c.Name {
		case CheckKubectlVersion:
			summary.KubectlVersion = c.Version
		case CheckHelmVersion:
			summary.HelmVersion = c.Version
		case CheckClusterReachable:
			summary.ServerVersion = c.Version
		}
	}
	for _, c := range checks {
		if c.Status == StatusFail && c.Hint != "" {
			summary.NextStep = c.Hint
			break
		}
	}
	return summary
}
