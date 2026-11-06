// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/neo4j/cli/internal/clierr"
)

// kubectlLookPathFn and helmLookPathFn are the injectable seams for finding
// the host binaries. Production wires exec.LookPath. Tests swap them so the
// missing-binary paths run without touching the host PATH.
var (
	kubectlLookPathFn = exec.LookPath
	helmLookPathFn    = exec.LookPath
)

// KubectlPresent reports whether kubectl resolves on PATH.
func KubectlPresent() bool {
	_, err := kubectlLookPathFn("kubectl")
	return err == nil
}

// HelmPresent reports whether helm resolves on PATH.
func HelmPresent() bool {
	_, err := helmLookPathFn("helm")
	return err == nil
}

// kubectlMissingErr is the shared install hint for a missing kubectl.
func kubectlMissingErr() error {
	return clierr.NewUsageError(
		"kubectl not found in PATH — install kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl/) " +
			"or a bundled distribution such as kind, minikube or Docker Desktop with its CLI on PATH",
	)
}

// helmMissingErr is the shared install hint for a missing helm.
func helmMissingErr() error {
	return clierr.NewUsageError(
		"helm not found in PATH — install Helm 3.8 or newer (https://helm.sh/docs/intro/install/)",
	)
}

// resolveKubectl returns the kubectl path, or the usage error with the
// install hint when kubectl is missing.
func resolveKubectl() (string, error) {
	path, err := kubectlLookPathFn("kubectl")
	if err != nil {
		return "", kubectlMissingErr()
	}
	return path, nil
}

// resolveHelm returns the helm path, or the usage error with the install
// hint when helm is missing.
func resolveHelm() (string, error) {
	path, err := helmLookPathFn("helm")
	if err != nil {
		return "", helmMissingErr()
	}
	return path, nil
}

// realExec runs one child process and captures its output. It is the
// production wiring behind the runFn seam. The child inherits the parent
// environment. Secrets never travel through argv, so none are added here.
func realExec(ctx context.Context, path string, args []string) runResult {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return runResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
	}
}
