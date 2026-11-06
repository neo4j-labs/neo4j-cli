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

// parseHelmVersion reads the stdout of `helm version --short`, which is one
// line such as "v4.1.1+g5caf004" on helm 4 or "v3.16.4+g8e8f4e2" on helm 3.
// Empty output yields an empty string. Anything after the first line is
// dropped, so a stray warning cannot pollute the version.
func parseHelmVersion(stdout string) string {
	line, _, _ := strings.Cut(stdout, "\n")
	return strings.TrimSpace(line)
}
