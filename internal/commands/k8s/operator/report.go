// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package operator

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
	toon "github.com/toon-format/toon-go"
)

// OperatorChange is the report `operator install` and `operator upgrade`
// emit.
type OperatorChange struct {
	Release         string   `json:"release"`
	Namespace       string   `json:"namespace"`
	Action          string   `json:"action"`
	ChartVersion    string   `json:"chart_version"`
	WatchNamespaces []string `json:"watch_namespaces,omitempty"`
	CRDs            []string `json:"crds"`
}

// OperatorRemoval is the report `operator uninstall` emits.
type OperatorRemoval struct {
	Release     string   `json:"release"`
	Namespace   string   `json:"namespace"`
	CRDsRemoved []string `json:"crds_removed,omitempty"`
}

// OperatorStatus is the report `operator status` emits. Installed is false
// when no operator Helm release exists; the CRD entries are still reported,
// because the CRDs outlive the release.
type OperatorStatus struct {
	Installed          bool              `json:"installed"`
	Release            string            `json:"release"`
	Namespace          string            `json:"namespace"`
	Status             string            `json:"status,omitempty"`
	ChartVersion       string            `json:"chart_version,omitempty"`
	AppVersion         string            `json:"app_version,omitempty"`
	Revision           string            `json:"revision,omitempty"`
	WatchNamespaces    []string          `json:"watch_namespaces,omitempty"`
	WatchAllNamespaces bool              `json:"watch_all_namespaces,omitempty"`
	Deployment         *DeploymentReport `json:"deployment,omitempty"`
	CRDs               []CRDReport       `json:"crds"`
	Hint               string            `json:"hint,omitempty"`
}

// DeploymentReport is the controller Deployment part of the status report.
type DeploymentReport struct {
	Name          string `json:"name"`
	Ready         bool   `json:"ready"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"ready_replicas"`
	Message       string `json:"message,omitempty"`
}

// CRDReport is one CRD's presence and versions in the status report.
type CRDReport struct {
	Name           string   `json:"name"`
	Installed      bool     `json:"installed"`
	Versions       []string `json:"versions,omitempty"`
	StoredVersions []string `json:"stored_versions,omitempty"`
}

// renderReport dispatches one operator report to the format-resolved
// renderer. JSON and toon are the report itself; the table is the sectioned
// key/value view the leaf builds from the same report.
func renderReport(cmd *cobra.Command, cfg *clicfg.Config, report any, table []string) error {
	switch commonoutput.ResolveOutput(cmd, cfg) {
	case "json":
		return writeReportJSON(cmd.OutOrStdout(), report)
	case "toon":
		return writeReportToon(cmd.OutOrStdout(), report)
	default:
		for _, line := range table {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), commonoutput.StripControl(line)); err != nil {
				return err
			}
		}
		return nil
	}
}

func writeReportJSON(w io.Writer, report any) error {
	b, err := json.MarshalIndent(report, "", "\t")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

// writeReportToon marshals through JSON first so `json:"-"` fields and
// omitempty behave exactly as they do in the JSON view, then strips control
// bytes, which toon rejects.
func writeReportToon(w io.Writer, report any) error {
	b, err := json.Marshal(report)
	if err != nil {
		return err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	v = commonoutput.WalkStrings(v, commonoutput.StripControl)
	out, err := toon.Marshal(v, toon.WithLengthMarkers(true))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(out))
	return err
}

// changeTable renders an install or upgrade report as key/value lines.
func changeTable(change OperatorChange) []string {
	pairs := [][2]string{
		{"release", change.Release},
		{"namespace", change.Namespace},
		{"action", change.Action},
		{"chart_version", change.ChartVersion},
	}
	if len(change.WatchNamespaces) > 0 {
		pairs = append(pairs, [2]string{"watch_namespaces", strings.Join(change.WatchNamespaces, ",")})
	}
	pairs = append(pairs, [2]string{"crds", strings.Join(change.CRDs, ",")})
	return keyValueLines(pairs)
}

// removalTable renders an uninstall report as key/value lines.
func removalTable(removal OperatorRemoval) []string {
	pairs := [][2]string{
		{"release", removal.Release},
		{"namespace", removal.Namespace},
	}
	if len(removal.CRDsRemoved) > 0 {
		pairs = append(pairs, [2]string{"crds_removed", strings.Join(removal.CRDsRemoved, ",")})
	}
	return keyValueLines(pairs)
}

// statusTable renders a status report as key/value lines plus a CRD section.
func statusTable(status OperatorStatus) []string {
	pairs := [][2]string{
		{"installed", fmt.Sprintf("%t", status.Installed)},
		{"release", status.Release},
		{"namespace", status.Namespace},
	}
	if status.Installed {
		pairs = append(pairs,
			[2]string{"status", status.Status},
			[2]string{"chart_version", status.ChartVersion},
			[2]string{"app_version", status.AppVersion},
			[2]string{"revision", status.Revision},
		)
	}
	scope := strings.Join(status.WatchNamespaces, ",")
	if status.WatchAllNamespaces {
		scope = "all namespaces"
	}
	if scope != "" {
		pairs = append(pairs, [2]string{"watch", scope})
	}
	if status.Deployment != nil {
		readiness := "not ready"
		if status.Deployment.Ready {
			readiness = "ready"
		}
		pairs = append(pairs, [2]string{
			"deployment",
			fmt.Sprintf("%s (%s, %d/%d replicas)", status.Deployment.Name, readiness,
				status.Deployment.ReadyReplicas, status.Deployment.Replicas),
		})
	}

	lines := keyValueLines(pairs)
	lines = append(lines, "", "CRDs:")
	for _, crd := range status.CRDs {
		if !crd.Installed {
			lines = append(lines, fmt.Sprintf("  %-30s not installed", crd.Name))
			continue
		}
		lines = append(lines, fmt.Sprintf("  %-30s %s", crd.Name, strings.Join(crd.Versions, ",")))
	}
	if status.Hint != "" {
		lines = append(lines, "", "Hint: "+status.Hint)
	}
	return lines
}

// keyValueLines renders aligned `key: value` lines.
func keyValueLines(pairs [][2]string) []string {
	width := 0
	for _, pair := range pairs {
		if len(pair[0]) > width {
			width = len(pair[0])
		}
	}
	lines := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		lines = append(lines, fmt.Sprintf("%-*s  %s", width+1, pair[0]+":", pair[1]))
	}
	return lines
}
