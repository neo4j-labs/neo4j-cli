// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"encoding/json"
)

// kubectlArgs builds the argv for one kubectl call. The verb and its local
// flags come first, then the group's persistent flags, matching the form
// users write by hand (`kubectl get pods -n ns`). Only non-empty settings
// reach argv, so a bare `kubectl version -o json` stays bare.
func kubectlArgs(s Settings, verb ...string) []string {
	args := append([]string{}, verb...)
	args = appendGlobalArgs(args, s)
	return args
}

// appendGlobalArgs appends the shared flag arguments for kubectl: the
// kubeconfig file, the context and the namespace.
func appendGlobalArgs(args []string, s Settings) []string {
	if s.Kubeconfig != "" {
		args = append(args, "--kubeconfig", s.Kubeconfig)
	}
	if s.Context != "" {
		args = append(args, "--context", s.Context)
	}
	if s.Namespace != "" {
		args = append(args, "-n", s.Namespace)
	}
	return args
}

// kubectlApplyCRDArgs builds the server-side apply of the operator CRD
// manifest. source is the release asset URL or a local path; kubectl fetches
// a URL itself. --force-conflicts matches the operator's own install
// instructions, and the field manager keeps the CLI's ownership attributable.
func kubectlApplyCRDArgs(s Settings, source string) []string {
	return kubectlArgs(s,
		"apply",
		"--server-side",
		"--force-conflicts",
		"--field-manager="+OperatorFieldManager,
		"-f", source)
}

// kubectlDeleteCRDArgs builds the delete of the four operator CRDs by name.
// --ignore-not-found makes a second uninstall --remove-crds a no-op instead
// of an error.
func kubectlDeleteCRDArgs(s Settings) []string {
	args := append([]string{"delete", "crd"}, OperatorCRDNames...)
	args = append(args, "--ignore-not-found")
	return kubectlArgs(s, args...)
}

// kubectlCurrentContextArgs builds `kubectl config current-context`.
func kubectlCurrentContextArgs(s Settings) []string {
	return kubectlArgs(s, "config", "current-context")
}

// kubectlGetArgs builds a read of one object as JSON.
func kubectlGetArgs(s Settings, kind, name string) []string {
	return kubectlArgs(s, "get", kind, name, "-o", "json")
}

// kubectlVersionJSON mirrors the shape of `kubectl version -o json`. The
// camelCase tags are the wire format, so they stay as kubectl emits them.
type kubectlVersionJSON struct {
	ClientVersion struct {
		GitVersion string `json:"gitVersion"`
	} `json:"clientVersion"`
	ServerVersion struct {
		GitVersion string `json:"gitVersion"`
	} `json:"serverVersion"`
}

// parseKubectlVersion decodes the stdout of `kubectl version -o json`.
// Malformed or partial output yields zero values rather than an error.
// When the API server is down, some kubectl versions still print the
// client half before exiting non-zero. Doctor wants that half.
func parseKubectlVersion(stdout string) VersionInfo {
	var raw kubectlVersionJSON
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		return VersionInfo{}
	}
	return VersionInfo{
		ClientVersion: raw.ClientVersion.GitVersion,
		ServerVersion: raw.ServerVersion.GitVersion,
	}
}
