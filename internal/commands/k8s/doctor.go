// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/debug"
	"github.com/spf13/cobra"
)

// Lowercase wire status values. The table renderer upper-cases them for
// display.
const (
	StatusPass = "pass"
	StatusFail = "fail"
	StatusSkip = "skip"
	StatusInfo = "info"
	StatusWarn = "warn"
)

// Stable check identifiers. They are the `name` field in JSON and toon
// output and the dependency keys for skip semantics.
const (
	CheckKubectlPresent    = "kubectl_present"
	CheckKubectlVersion    = "kubectl_version"
	CheckHelmPresent       = "helm_present"
	CheckHelmVersion       = "helm_version"
	CheckClusterReachable  = "cluster_reachable"
	CheckKubernetesVersion = "kubernetes_version"
	CheckVersionSkew       = "version_skew"
)

// User-visible labels for the table view.
const (
	LabelKubectlPresent    = "kubectl present"
	LabelKubectlVersion    = "kubectl version"
	LabelHelmPresent       = "helm present"
	LabelHelmVersion       = "helm version"
	LabelClusterReachable  = "Cluster reachable"
	LabelKubernetesVersion = "Kubernetes version"
	LabelVersionSkew       = "Client/server skew"
)

// Version floors doctor enforces. kubectl 1.22 introduced
// `--server-side` apply. Helm 3.8 introduced OCI registry support. The
// operator's CRDs carry CEL rules that need Kubernetes 1.35.
const (
	minKubectlMajor, minKubectlMinor = 1, 22
	minHelmMajor, minHelmMinor       = 3, 8
	minServerMajor, minServerMinor   = 1, 35
)

// CheckResult is one row in the doctor report. Version carries the version
// string the check learned, when there is one.
type CheckResult struct {
	Name    string `json:"name"`
	Label   string `json:"-"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Hint    string `json:"hint,omitempty"`
	Version string `json:"version,omitempty"`
}

// DoctorSummary is the verdict appended to the report. Ready is true when
// no check failed. A warn does not block, because the skew policy is
// support guidance rather than a hard limit.
type DoctorSummary struct {
	Ready          bool   `json:"ready"`
	KubectlVersion string `json:"kubectl_version,omitempty"`
	HelmVersion    string `json:"helm_version,omitempty"`
	ServerVersion  string `json:"server_version,omitempty"`
	NextStep       string `json:"next_step,omitempty"`
}

// DoctorReport is the full `--format json` / `--format toon` envelope.
type DoctorReport struct {
	Checks  []CheckResult `json:"checks"`
	Summary DoctorSummary `json:"summary"`
}

// newDoctorCmd returns the read-only `k8s doctor` leaf. It exits 0 whatever
// the checks find. Consumers that need a hard gate parse `summary.ready`.
func newDoctorCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the local Kubernetes setup for the Neo4j operator",
		Long: "Run an ordered sequence of seven checks against the local Kubernetes setup: " +
			"(1) kubectl present, (2) kubectl version, (3) helm present, (4) helm version, " +
			"(5) cluster reachable, (6) Kubernetes version, (7) client/server version skew. " +
			"Each check produces a `{name, status, detail, hint?}` record. " +
			"When a check fails, the checks that depend on it render as `skip` with a `(depends on …)` detail. " +
			"The version checks enforce kubectl 1.22 or newer (for `--server-side`), helm 3.8 or newer (for OCI) " +
			"and Kubernetes 1.35 or newer (the operator's CRDs carry CEL rules that need it). " +
			"The skew check warns when kubectl and the API server differ by more than one minor version. " +
			"Kubernetes supports a skew of one minor version in either direction. A warning does not fail the report. " +
			"`--format json` (or `toon`) emits a single `{checks: [...], summary: {ready, kubectl_version?, helm_version?, server_version?, next_step?}}` document. " +
			"The default table format renders aligned check / status / detail columns and a trailing summary line. " +
			"The leaf is read-only and exits 0 whatever the checks find (an interrupt still exits 130). Parse `summary.ready` to gate downstream actions.",
		Example: `# Run all seven checks against the current Kubernetes context
neo4j-cli k8s doctor

# Emit a structured JSON report suitable for agent consumption
neo4j-cli k8s doctor --format json

# Check a specific kubeconfig file and context
neo4j-cli k8s doctor --kubeconfig ~/.kube/kind-config --context kind-kind --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			settings, err := settingsFrom(cmd, cfg, debug.Resolve(cmd))
			if err != nil {
				return err
			}
			client := clientFactory(settings)
			report := runChecks(cmd.Context(), client)
			// A signal killed the child mid-check, so the report describes the
			// interrupt, not the cluster. Return the context error so cli.Run
			// exits 128+signal instead of printing a misleading FAIL row.
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			return renderReport(cmd, cfg, report)
		},
	}
}
