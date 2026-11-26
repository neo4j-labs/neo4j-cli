// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"fmt"

	engine "github.com/neo4j/cli/internal/k8s"
)

// checkBinaryPresent reports whether one host binary resolves on PATH.
func checkBinaryPresent(checkName, binaryName, label string, present bool) CheckResult {
	if !present {
		hint := "Install kubectl 1.22 or newer (https://kubernetes.io/docs/tasks/tools/install-kubectl/)."
		if binaryName == "helm" {
			hint = "Install Helm 3.8 or newer (https://helm.sh/docs/intro/install/)."
		}
		return CheckResult{
			Name:   checkName,
			Label:  label,
			Status: StatusFail,
			Detail: fmt.Sprintf("%s not found in PATH.", binaryName),
			Hint:   hint,
		}
	}
	return CheckResult{
		Name:   checkName,
		Label:  label,
		Status: StatusPass,
		Detail: fmt.Sprintf("%s found in PATH.", binaryName),
	}
}

// checkKubectlVersion compares the kubectl client version against the
// 1.22 floor. present gates the skip. A missing client version is a fail,
// because doctor cannot advise on an unknown kubectl.
func checkKubectlVersion(vi engine.VersionInfo, callErr error, present bool) CheckResult {
	if !present {
		return skipResult(CheckKubectlVersion, LabelKubectlVersion, CheckKubectlPresent)
	}
	if vi.ClientVersion == "" {
		detail := "kubectl version could not be determined."
		if callErr != nil {
			detail = fmt.Sprintf("kubectl version could not be determined: %s.", callErr.Error())
		}
		return CheckResult{
			Name:   CheckKubectlVersion,
			Label:  LabelKubectlVersion,
			Status: StatusFail,
			Detail: detail,
			Hint:   "Check that kubectl runs outside neo4j-cli, then re-run `k8s doctor`.",
		}
	}
	if !engine.MajorMinorAtLeast(vi.ClientVersion, minKubectlMajor, minKubectlMinor) {
		return CheckResult{
			Name:    CheckKubectlVersion,
			Label:   LabelKubectlVersion,
			Status:  StatusFail,
			Detail:  fmt.Sprintf("kubectl %s is older than v%d.%d.", vi.ClientVersion, minKubectlMajor, minKubectlMinor),
			Version: vi.ClientVersion,
			Hint:    "Install kubectl 1.22 or newer. Server-side apply, which `k8s operator install` uses, needs it.",
		}
	}
	return CheckResult{
		Name:    CheckKubectlVersion,
		Label:   LabelKubectlVersion,
		Status:  StatusPass,
		Detail:  fmt.Sprintf("kubectl %s.", vi.ClientVersion),
		Version: vi.ClientVersion,
	}
}

// checkHelmVersion compares the helm version against the 3.8 floor.
func checkHelmVersion(version string, callErr error, present bool) CheckResult {
	if !present {
		return skipResult(CheckHelmVersion, LabelHelmVersion, CheckHelmPresent)
	}
	if version == "" {
		detail := "helm version could not be determined."
		if callErr != nil {
			detail = fmt.Sprintf("helm version could not be determined: %s.", callErr.Error())
		}
		return CheckResult{
			Name:   CheckHelmVersion,
			Label:  LabelHelmVersion,
			Status: StatusFail,
			Detail: detail,
			Hint:   "Check that `helm version` runs outside neo4j-cli, then re-run `k8s doctor`.",
		}
	}
	if !engine.MajorMinorAtLeast(version, minHelmMajor, minHelmMinor) {
		return CheckResult{
			Name:    CheckHelmVersion,
			Label:   LabelHelmVersion,
			Status:  StatusFail,
			Detail:  fmt.Sprintf("helm %s is older than v%d.%d.", version, minHelmMajor, minHelmMinor),
			Version: version,
			Hint:    "Install Helm 3.8 or newer. The operator chart is served from an OCI registry, which helm only supports from 3.8.",
		}
	}
	return CheckResult{
		Name:    CheckHelmVersion,
		Label:   LabelHelmVersion,
		Status:  StatusPass,
		Detail:  fmt.Sprintf("helm %s.", version),
		Version: version,
	}
}

// checkClusterReachable reports whether the API server answered the version
// probe. The detail carries the child's own error text, which the engine
// has already redacted, so the operator sees the real cause.
func checkClusterReachable(vi engine.VersionInfo, callErr error, present bool) CheckResult {
	if !present {
		return skipResult(CheckClusterReachable, LabelClusterReachable, CheckKubectlPresent)
	}
	if callErr != nil {
		return CheckResult{
			Name:   CheckClusterReachable,
			Label:  LabelClusterReachable,
			Status: StatusFail,
			Detail: fmt.Sprintf("kubectl could not reach the API server: %s.", callErr.Error()),
			Hint:   "Start the cluster, or point --kubeconfig / --context at the right cluster, then re-run `k8s doctor`.",
		}
	}
	if vi.ServerVersion == "" {
		return CheckResult{
			Name:   CheckClusterReachable,
			Label:  LabelClusterReachable,
			Status: StatusFail,
			Detail: "kubectl reported no server version, so the cluster state is unknown.",
			Hint:   "Run `kubectl version` outside neo4j-cli to see the raw error, then re-run `k8s doctor`.",
		}
	}
	return CheckResult{
		Name:    CheckClusterReachable,
		Label:   LabelClusterReachable,
		Status:  StatusPass,
		Detail:  fmt.Sprintf("API server answered, Kubernetes %s.", vi.ServerVersion),
		Version: vi.ServerVersion,
	}
}

// checkKubernetesVersion compares the server version against the 1.35
// floor the operator needs.
func checkKubernetesVersion(vi engine.VersionInfo, reachable bool) CheckResult {
	if !reachable {
		return skipResult(CheckKubernetesVersion, LabelKubernetesVersion, CheckClusterReachable)
	}
	if !engine.MajorMinorAtLeast(vi.ServerVersion, minServerMajor, minServerMinor) {
		return CheckResult{
			Name:    CheckKubernetesVersion,
			Label:   LabelKubernetesVersion,
			Status:  StatusFail,
			Detail:  fmt.Sprintf("Kubernetes %s is older than v%d.%d.", vi.ServerVersion, minServerMajor, minServerMinor),
			Version: vi.ServerVersion,
			Hint:    fmt.Sprintf("The Neo4j operator needs Kubernetes %d.%d or newer. Upgrade the cluster before `k8s operator install`.", minServerMajor, minServerMinor),
		}
	}
	return CheckResult{
		Name:    CheckKubernetesVersion,
		Label:   LabelKubernetesVersion,
		Status:  StatusPass,
		Detail:  fmt.Sprintf("Kubernetes %s.", vi.ServerVersion),
		Version: vi.ServerVersion,
	}
}

// checkVersionSkew compares the kubectl client minor against the server
// minor. Kubernetes supports a skew of one minor version in either
// direction. A wider skew renders as a warning, because the policy is
// support guidance rather than a hard limit.
func checkVersionSkew(vi engine.VersionInfo, reachable bool) CheckResult {
	if !reachable {
		return skipResult(CheckVersionSkew, LabelVersionSkew, CheckClusterReachable)
	}
	skew, ok := engine.MinorSkew(vi.ClientVersion, vi.ServerVersion)
	if !ok {
		return CheckResult{
			Name:   CheckVersionSkew,
			Label:  LabelVersionSkew,
			Status: StatusInfo,
			Detail: fmt.Sprintf("Could not compare kubectl %s with Kubernetes %s.", vi.ClientVersion, vi.ServerVersion),
		}
	}
	distance := skew
	if distance < 0 {
		distance = -distance
	}
	if distance <= 1 {
		return CheckResult{
			Name:   CheckVersionSkew,
			Label:  LabelVersionSkew,
			Status: StatusPass,
			Detail: fmt.Sprintf("kubectl %s is within one minor of Kubernetes %s.", vi.ClientVersion, vi.ServerVersion),
		}
	}
	direction := "newer"
	if skew < 0 {
		direction = "older"
	}
	return CheckResult{
		Name:   CheckVersionSkew,
		Label:  LabelVersionSkew,
		Status: StatusWarn,
		Detail: fmt.Sprintf("kubectl %s is %d minors %s than Kubernetes %s. Kubernetes supports a skew of one minor.", vi.ClientVersion, distance, direction, vi.ServerVersion),
		Hint:   "Align the kubectl minor with the server minor to stay inside the supported skew.",
	}
}

// skipResult builds the `skip` row emitted in place of a dependent check
// when an upstream check failed.
func skipResult(name, label, dependency string) CheckResult {
	return CheckResult{
		Name:   name,
		Label:  label,
		Status: StatusSkip,
		Detail: fmt.Sprintf("(depends on %s)", dependency),
	}
}
