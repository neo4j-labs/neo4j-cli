// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package k8s is the Kubernetes engine for the `neo4j-cli k8s` command tree.
//
// It drives the host `kubectl` and `helm` binaries, exactly as the docker
// engine drives the host `docker`. The package never imports client-go or a
// Helm SDK. Reads decode `-o json` output into plain Go values.
//
// Only this package runs kubectl or helm. Command packages call it through
// the Client interface. Tests inject FakeClient, so no test spawns a child
// process or touches a real kubeconfig.
package k8s

import (
	"context"
	"time"
)

// Settings carries the k8s group's persistent flag values to every child
// process. The group root owns the flags. Each leaf copies them into a
// Settings value and passes it to NewClient. This way --kubeconfig,
// --context and --namespace reach every kubectl and helm call.
type Settings struct {
	// Kubeconfig is the value of --kubeconfig. Empty means kubectl finds
	// the default file itself.
	Kubeconfig string
	// Context is the value of --context. Empty means the current context.
	Context string
	// Namespace is the value of -n/--namespace. Empty means the default.
	Namespace string
	// Debug turns on the [k8s-debug] trace for every child process.
	Debug bool
}

// Client is the surface the `k8s` command tree drives. The execClient
// implementation shells out to the host binaries. Tests inject FakeClient.
// Methods grow per PR as the operator and instance leaves land.
type Client interface {
	// KubectlVersion runs `kubectl version -o json` with a 10s request
	// timeout and returns the parsed
	// client and server git versions. When the API server is unreachable the
	// call returns an error. Any stdout the child still produced is parsed
	// first, so ClientVersion can carry a value alongside the error.
	KubectlVersion(ctx context.Context) (VersionInfo, error)
	// HelmVersion runs `helm version --short` and returns the helm version,
	// for example "v4.1.1+g5caf004". Helm 4 removed the -o json flag, and
	// --short works on every helm 3 and 4 release.
	HelmVersion(ctx context.Context) (string, error)
}

// VersionInfo is the parsed output of `kubectl version -o json`. The
// server field is empty when the API server did not answer.
type VersionInfo struct {
	ClientVersion string
	ServerVersion string
}

// execClient is the default Client that shells out via the runFn seam.
type execClient struct {
	settings Settings
}

// NewClient returns the exec-backed client for the given settings. When
// settings.Debug is set, every child invocation echoes a redacted
// [k8s-debug] trace to the package's debug writer.
func NewClient(settings Settings) Client {
	return &execClient{settings: settings}
}

// versionProbeTimeout bounds the `kubectl version` probe.
const versionProbeTimeout = "10s"

// runResult carries the captured outcome of one child process.
type runResult struct {
	stdout string
	stderr string
	err    error
}

// runFn is the process-execution seam. Production wires realExec. Engine
// tests swap it to capture the argv the engine builds and return canned
// output, so argv-pinning tests never spawn a process.
var runFn = realExec

// exec runs one child process through the seam and emits the debug trace.
// It returns the raw result. Callers classify failures through
// classifyRunError.
func (c *execClient) exec(ctx context.Context, binary, path string, args []string) runResult {
	if c.settings.Debug {
		debugInvocation(binary, args)
	}
	start := time.Now()
	res := runFn(ctx, path, args)
	if c.settings.Debug {
		debugResult(binary, res, time.Since(start))
	}
	return res
}

// KubectlVersion implements Client.
func (c *execClient) KubectlVersion(ctx context.Context) (VersionInfo, error) {
	path, err := resolveKubectl()
	if err != nil {
		return VersionInfo{}, err
	}
	// A black-holed API server would otherwise hold doctor until client-go's
	// own dial timeout, so the probe gets a short, explicit request timeout.
	args := kubectlArgs(c.settings, "version", "-o", "json", "--request-timeout="+versionProbeTimeout)
	res := c.exec(ctx, "kubectl", path, args)
	info := parseKubectlVersion(res.stdout)
	if res.err != nil {
		return info, classifyRunError(ctx, "kubectl", args, res)
	}
	return info, nil
}

// HelmVersion implements Client.
func (c *execClient) HelmVersion(ctx context.Context) (string, error) {
	path, err := resolveHelm()
	if err != nil {
		return "", err
	}
	args := helmArgs(c.settings, "version", "--short")
	res := c.exec(ctx, "helm", path, args)
	version := parseHelmVersion(res.stdout)
	if res.err != nil {
		return version, classifyRunError(ctx, "helm", args, res)
	}
	return version, nil
}
