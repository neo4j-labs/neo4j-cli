// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The operator's fixed identity. The CLI always installs it as this Helm
// release, into this namespace, applying the four CRDs from the matching
// release asset, so `status` and `uninstall` find what `install` created
// without asking the user.
const (
	// DefaultOperatorVersion is the operator release the CLI installs when
	// --version is omitted. Only v1.0.0 is published.
	DefaultOperatorVersion = "1.0.0"
	// OperatorReleaseName is the fixed Helm release name.
	OperatorReleaseName = "neo4j-operator"
	// DefaultOperatorNamespace is where the operator lives when the group's
	// -n/--namespace flag is empty. The chart's watch scope must not include
	// it.
	DefaultOperatorNamespace = "neo4j-operator-system"
	// OperatorChart is the published OCI chart reference.
	OperatorChart = "oci://ghcr.io/neo4j-partners/charts/neo4j-operator"
	// OperatorDeploymentName is the chart's fixed controller Deployment name.
	OperatorDeploymentName = "neo4j-operator-controller-manager"
	// OperatorFieldManager is the server-side-apply field manager the CLI
	// uses for the CRDs, so the fields it owns stay attributable.
	OperatorFieldManager = "neo4j-cli"
)

// OperatorCRDNames are the four CRDs the operator release asset installs.
var OperatorCRDNames = []string{
	"neo4js.neo4j.com",
	"neo4jbackups.neo4j.com",
	"neo4jbackupschedules.neo4j.com",
	"neo4jrestores.neo4j.com",
}

// OperatorCRDManifest returns the release asset URL that bundles all four
// CRDs for one operator version. kubectl fetches the URL itself, so the
// engine needs no HTTP client and the CLI keeps no copy of the schema.
func OperatorCRDManifest(version string) string {
	return fmt.Sprintf(
		"https://github.com/neo4j-partners/neo4j-kubernetes-operator/releases/download/v%s/neo4j-crd-%s.yaml",
		version, version)
}

// HelmUpgradeOptions describes one `helm upgrade` invocation. The release
// name, chart and namespace are the operator's fixed identity plus the
// group's settings; only the per-invocation choices live here.
type HelmUpgradeOptions struct {
	// Version is the chart version to install, for example "1.0.0".
	Version string
	// WatchNamespaces is the workload scope. The chart refuses to render
	// without one. An empty slice omits --set, which an upgrade needs when
	// it reuses the scope already deployed.
	WatchNamespaces []string
	// Install adds --install, so the call also creates a missing release.
	Install bool
	// ReuseValues adds --reset-then-reuse-values (helm 3.14 and later), so
	// an upgrade keeps the values already deployed and overrides only the
	// ones passed here. Plain --reuse-values would also replace the new
	// chart's defaults with the old chart's, dropping any key a later chart
	// adds.
	ReuseValues bool
	// CreateNamespace adds --create-namespace.
	CreateNamespace bool
}

// HelmRelease is one entry of `helm list -o json`. The tags are helm's wire
// format, not CLI output names.
type HelmRelease struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Revision   string `json:"revision"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"app_version"`
}

// ChartVersion returns the chart version from the reference `helm list`
// reports, which concatenates the chart name and version
// ("neo4j-operator-1.0.0"). An unexpected shape is returned unchanged.
func (r HelmRelease) ChartVersion() string {
	return strings.TrimPrefix(r.Chart, OperatorReleaseName+"-")
}

// CRDInfo is the part of one CustomResourceDefinition the status report
// shows.
type CRDInfo struct {
	Name           string   `json:"name"`
	Versions       []string `json:"versions"`
	StoredVersions []string `json:"stored_versions"`
}

// DeploymentStatus is the part of the controller Deployment the status
// report shows.
type DeploymentStatus struct {
	Name               string
	Namespace          string
	Replicas           int32
	ReadyReplicas      int32
	Available          bool
	AvailableReason    string
	AvailableMessage   string
	WatchNamespaces    []string
	WatchAllNamespaces bool
	Image              string
}

// parseHelmReleases decodes `helm list -o json`. An empty list is valid and
// means no release is installed.
func parseHelmReleases(stdout string) ([]HelmRelease, error) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil, nil
	}
	var releases []HelmRelease
	if err := json.Unmarshal([]byte(trimmed), &releases); err != nil {
		return nil, fmt.Errorf("parse helm list output: %w", err)
	}
	return releases, nil
}

// FindOperatorRelease returns the operator's release from a helm list
// result, and whether it is present at all.
func FindOperatorRelease(releases []HelmRelease) (HelmRelease, bool) {
	for _, release := range releases {
		if release.Name == OperatorReleaseName {
			return release, true
		}
	}
	return HelmRelease{}, false
}

// parseCRDNames reads the CRD names from a kubectl apply or delete report.
// Apply prints `customresourcedefinition.apiextensions.k8s.io/<name>
// serverside-applied`; delete prints
// `customresourcedefinition.apiextensions.k8s.io "<name>" deleted`.
func parseCRDNames(stdout string) []string {
	var names []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "customresourcedefinition") {
			continue
		}
		if start := strings.IndexByte(line, '"'); start >= 0 {
			if end := strings.IndexByte(line[start+1:], '"'); end >= 0 {
				names = append(names, line[start+1:start+1+end])
				continue
			}
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			if i := strings.LastIndexByte(fields[0], '/'); i >= 0 {
				names = append(names, fields[0][i+1:])
			}
		}
	}
	return names
}

// deploymentJSON mirrors the fields of `kubectl get deployment -o json` the
// status report reads. The camelCase tags are kubectl's wire format.
type deploymentJSON struct {
	Metadata struct {
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		Generation int64  `json:"generation"`
	} `json:"metadata"`
	Spec struct {
		Replicas *int32 `json:"replicas"`
		Template struct {
			Spec struct {
				Containers []containerJSON `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		Replicas           int32 `json:"replicas"`
		ReadyReplicas      int32 `json:"readyReplicas"`
		ObservedGeneration int64 `json:"observedGeneration"`
		Conditions         []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"conditions"`
	} `json:"status"`
}

// containerJSON is one pod template container: its name, image and the
// plain env entries the watch scope is read from.
type containerJSON struct {
	Name  string    `json:"name"`
	Image string    `json:"image"`
	Env   []envJSON `json:"env"`
}

// envJSON is one container env entry. Entries set via valueFrom carry an
// empty Value, which the watch scope ignores.
type envJSON struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// parseDeployment decodes `kubectl get deployment -o json` into the status
// report's view. A Deployment is ready when its Available condition is true,
// the controller has observed the current generation, and no pod is left
// over from an earlier rollout — Available alone stays true during a rolling
// update, while an old pod is still terminating.
func parseDeployment(stdout string) (DeploymentStatus, error) {
	var raw deploymentJSON
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		return DeploymentStatus{}, fmt.Errorf("parse deployment output: %w", err)
	}
	status := DeploymentStatus{
		Name:          raw.Metadata.Name,
		Namespace:     raw.Metadata.Namespace,
		Replicas:      raw.Status.Replicas,
		ReadyReplicas: raw.Status.ReadyReplicas,
	}
	desired := int32(1)
	if raw.Spec.Replicas != nil {
		desired = *raw.Spec.Replicas
	}
	if manager, ok := managerContainer(raw.Spec.Template.Spec.Containers); ok {
		status.Image = manager.Image
		status.WatchNamespaces, status.WatchAllNamespaces = watchScope(manager.Env)
	}
	for _, condition := range raw.Status.Conditions {
		if condition.Type != "Available" {
			continue
		}
		status.Available = condition.Status == "True" &&
			raw.Status.ObservedGeneration >= raw.Metadata.Generation &&
			raw.Status.Replicas == desired &&
			raw.Status.ReadyReplicas >= desired
		status.AvailableReason = condition.Reason
		status.AvailableMessage = condition.Message
	}
	return status, nil
}

// managerContainer picks the operator's manager out of the pod template.
// A sidecar such as kube-rbac-proxy may come first, so it prefers the
// container that carries the watch-scope env, then the one kubebuilder names
// "manager", and only then the first container.
func managerContainer(containers []containerJSON) (containerJSON, bool) {
	for _, container := range containers {
		for _, entry := range container.Env {
			if entry.Name == "WATCH_NAMESPACE" || entry.Name == "WATCH_ALL_NAMESPACES" {
				return container, true
			}
		}
	}
	for _, container := range containers {
		if container.Name == "manager" {
			return container, true
		}
	}
	if len(containers) == 0 {
		return containerJSON{}, false
	}
	return containers[0], true
}

// watchScope reads the operator's watch scope from the manager container
// env: WATCH_NAMESPACE is a comma-separated list, WATCH_ALL_NAMESPACES=true
// means cluster-wide.
func watchScope(env []envJSON) ([]string, bool) {
	var (
		namespaces []string
		all        bool
	)
	for _, entry := range env {
		switch entry.Name {
		case "WATCH_ALL_NAMESPACES":
			all = strings.EqualFold(strings.TrimSpace(entry.Value), "true")
		case "WATCH_NAMESPACE":
			for _, namespace := range strings.Split(entry.Value, ",") {
				if namespace = strings.TrimSpace(namespace); namespace != "" {
					namespaces = append(namespaces, namespace)
				}
			}
		}
	}
	return namespaces, all
}

// crdJSON mirrors the fields of `kubectl get crd -o json` the status report
// reads.
type crdJSON struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Versions []struct {
			Name string `json:"name"`
		} `json:"versions"`
	} `json:"spec"`
	Status struct {
		StoredVersions []string `json:"storedVersions"`
	} `json:"status"`
}

// parseCRD decodes `kubectl get crd -o json` into the status report's view.
func parseCRD(stdout string) (CRDInfo, error) {
	var raw crdJSON
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		return CRDInfo{}, fmt.Errorf("parse crd output: %w", err)
	}
	info := CRDInfo{
		Name:           raw.Metadata.Name,
		StoredVersions: raw.Status.StoredVersions,
	}
	for _, version := range raw.Spec.Versions {
		info.Versions = append(info.Versions, version.Name)
	}
	return info, nil
}
