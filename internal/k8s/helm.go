// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"strings"
)

// helmArgs builds the argv for one helm call. The verb and its local flags
// come first, then the shared flags. Helm spells the context flag
// --kube-context, so the group's --context value is translated here and
// nowhere else.
func helmArgs(s Settings, verb ...string) []string {
	args := append([]string{}, verb...)
	if s.Kubeconfig != "" {
		args = append(args, "--kubeconfig", s.Kubeconfig)
	}
	if s.Context != "" {
		args = append(args, "--kube-context", s.Context)
	}
	if s.Namespace != "" {
		args = append(args, "-n", s.Namespace)
	}
	return args
}

// helmUpgradeArgs builds `helm upgrade <release> <chart> …` for the operator.
// The release and chart are the operator's fixed identity, so only the
// per-invocation choices come from opts.
func helmUpgradeArgs(s Settings, opts HelmUpgradeOptions) []string {
	args := []string{"upgrade", OperatorReleaseName, OperatorChart}
	if opts.Version != "" {
		args = append(args, "--version", opts.Version)
	}
	if opts.Install {
		args = append(args, "--install")
	}
	if opts.CreateNamespace {
		args = append(args, "--create-namespace")
	}
	if opts.ReuseValues {
		args = append(args, "--reset-then-reuse-values")
	}
	if len(opts.WatchNamespaces) > 0 {
		args = append(args, "--set", "watchNamespaces="+helmNamespaceList(opts.WatchNamespaces))
	}
	return helmArgs(s, args...)
}

// helmNamespaceList renders a namespace list as the helm --set literal
// `{a,b}`. Namespace names are DNS labels, so no escaping is needed.
func helmNamespaceList(namespaces []string) string {
	return "{" + strings.Join(namespaces, ",") + "}"
}

// helmUninstallArgs builds `helm uninstall <release>` for the operator.
func helmUninstallArgs(s Settings) []string {
	return helmArgs(s, "uninstall", OperatorReleaseName)
}

// helmListArgs builds `helm list -o json` for the operator release. Helm 3
// lists only deployed and failed releases by default, so a release mid-way
// through an install, upgrade or uninstall would look absent; helm 4 lists
// every status but rejects helm 3's --all. Naming each live status works on
// both, and leaves out a release kept by `helm uninstall --keep-history`.
func helmListArgs(s Settings) []string {
	return helmArgs(s, "list",
		"--deployed", "--failed", "--pending", "--superseded", "--uninstalling",
		"--filter", "^"+OperatorReleaseName+"$",
		"-o", "json")
}

// parseHelmVersion reads the stdout of `helm version --short`, which is one
// line such as "v4.1.1+g5caf004" on helm 4 or "v3.16.4+g8e8f4e2" on helm 3.
// Empty output yields an empty string. Anything after the first line is
// dropped, so a stray warning cannot pollute the version.
func parseHelmVersion(stdout string) string {
	line, _, _ := strings.Cut(stdout, "\n")
	return strings.TrimSpace(line)
}
