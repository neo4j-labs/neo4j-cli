// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// call records one child invocation the captureRun seam observed.
type call struct {
	path string
	args []string
}

// captureRun swaps the runFn seam for a recorder. It logs each call and
// answers from a queue of canned results. No process ever runs.
func captureRun(t *testing.T, results []runResult) *[]call {
	t.Helper()
	calls := &[]call{}
	i := 0
	SetRunFnForTest(t, func(_ context.Context, path string, args []string) runResult {
		*calls = append(*calls, call{path: path, args: args})
		res := runResult{}
		if i < len(results) {
			res = results[i]
		}
		i++
		return res
	})
	return calls
}

// stubLookPath points both lookPath seams at a fake binary path. The fake
// path does not exist, so the validation seam accepts it unconditionally.
func stubLookPath(t *testing.T) {
	t.Helper()
	SetKubectlLookPathForTest(t, func(string) (string, error) { return "/usr/local/bin/fake-kubectl", nil })
	SetHelmLookPathForTest(t, func(string) (string, error) { return "/usr/local/bin/fake-helm", nil })
	SetValidateBinaryForTest(t, func(string, string) error { return nil })
}

// missingLookPath makes both lookPath seams report a missing binary.
func missingLookPath(t *testing.T) {
	t.Helper()
	notFound := func(string) (string, error) { return "", exec.ErrNotFound }
	SetKubectlLookPathForTest(t, notFound)
	SetHelmLookPathForTest(t, notFound)
}

// happyKubectlVersion is the `kubectl version -o json` shape for a healthy
// 1.36 cluster probed by a 1.36 client.
const happyKubectlVersion = `{"clientVersion":{"gitVersion":"v1.36.0"},"serverVersion":{"gitVersion":"v1.36.1"}}`

// happyHelmVersion is the `helm version --short` shape. Helm 4 removed the
// -o json flag, so --short is the only cross-version form.
const happyHelmVersion = "v4.1.1+g5caf004"

func TestKubectlVersion_ArgvCarriesGroupFlags(t *testing.T) {
	stubLookPath(t)
	calls := captureRun(t, []runResult{{stdout: happyKubectlVersion}})

	client := NewClient(Settings{
		Kubeconfig: "/tmp/kube",
		Context:    "kind-kind",
		Namespace:  "neo4j",
	})
	info, err := client.KubectlVersion(context.Background())
	require.NoError(t, err)

	require.Len(t, *calls, 1)
	assert.Equal(t, "/usr/local/bin/fake-kubectl", (*calls)[0].path)
	// The verb comes first, then the group's persistent flags. This pins
	// that --kubeconfig, --context and -n reach the child process.
	assert.Equal(t, []string{"version", "-o", "json", "--request-timeout=10s", "--kubeconfig", "/tmp/kube", "--context", "kind-kind", "-n", "neo4j"}, (*calls)[0].args)

	assert.Equal(t, "v1.36.0", info.ClientVersion)
	assert.Equal(t, "v1.36.1", info.ServerVersion)
}

func TestKubectlVersion_BareSettingsKeepArgvMinimal(t *testing.T) {
	stubLookPath(t)
	calls := captureRun(t, []runResult{{stdout: happyKubectlVersion}})

	_, err := NewClient(Settings{}).KubectlVersion(context.Background())
	require.NoError(t, err)
	require.Len(t, *calls, 1)
	assert.Equal(t, []string{"version", "-o", "json", "--request-timeout=10s"}, (*calls)[0].args)
}

func TestKubectlVersion_UnreachableServerStillParsesClientHalf(t *testing.T) {
	stubLookPath(t)
	partial := `{"clientVersion":{"gitVersion":"v1.36.0"}}`
	captureRun(t, []runResult{{
		stdout: partial,
		stderr: "The connection to the server localhost:8080 was refused",
		err:    &exec.ExitError{},
	}})

	info, err := NewClient(Settings{}).KubectlVersion(context.Background())
	require.Error(t, err)
	// The client half survives alongside the classified error, so doctor
	// can still check the kubectl version while the cluster check fails.
	assert.Equal(t, "v1.36.0", info.ClientVersion)
	assert.Empty(t, info.ServerVersion)
}

func TestKubectlVersion_MissingBinaryIsUsageError(t *testing.T) {
	missingLookPath(t)
	calls := captureRun(t, nil)

	_, err := NewClient(Settings{}).KubectlVersion(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kubectl not found in PATH")
	assert.Empty(t, *calls, "no child process may run when the binary is missing")
}

func TestHelmVersion_ArgvTranslatesContextFlag(t *testing.T) {
	stubLookPath(t)
	calls := captureRun(t, []runResult{{stdout: happyHelmVersion}})

	client := NewClient(Settings{
		Kubeconfig: "/tmp/kube",
		Context:    "kind-kind",
		Namespace:  "neo4j",
	})
	version, err := client.HelmVersion(context.Background())
	require.NoError(t, err)
	assert.Equal(t, happyHelmVersion, version)

	require.Len(t, *calls, 1)
	assert.Equal(t, "/usr/local/bin/fake-helm", (*calls)[0].path)
	// Helm spells the context flag --kube-context. The translation lives in
	// helmArgs and nowhere else.
	assert.Equal(t, []string{"version", "--short", "--kubeconfig", "/tmp/kube", "--kube-context", "kind-kind", "-n", "neo4j"}, (*calls)[0].args)
}

func TestHelmVersion_MissingBinaryIsUsageError(t *testing.T) {
	missingLookPath(t)
	_, err := NewClient(Settings{}).HelmVersion(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "helm not found in PATH")
}

func TestParseKubectlVersion_MalformedOutputYieldsZeroValues(t *testing.T) {
	assert.Equal(t, VersionInfo{}, parseKubectlVersion("not json at all"))
	assert.Equal(t, VersionInfo{}, parseKubectlVersion(""))
}

func TestParseHelmVersion(t *testing.T) {
	assert.Equal(t, happyHelmVersion, parseHelmVersion(happyHelmVersion+"\n"))
	// A stray second line is dropped, and empty output stays empty.
	assert.Equal(t, happyHelmVersion, parseHelmVersion(happyHelmVersion+"\nwarning: something\n"))
	assert.Equal(t, "", parseHelmVersion(""))
	assert.Equal(t, "", parseHelmVersion("\n"))
}

func TestDebugTrace_EmitsArgvAndStderrThroughSeam(t *testing.T) {
	stubLookPath(t)
	var buf strings.Builder
	SetDebugWriterForTest(t, &buf)
	captureRun(t, []runResult{{
		stdout: happyKubectlVersion,
		stderr: "connection refused",
		err:    &exec.ExitError{},
	}})

	_, _ = NewClient(Settings{Debug: true}).KubectlVersion(context.Background())
	out := buf.String()
	assert.Contains(t, out, DebugReqPrefix+"kubectl version -o json --request-timeout=10s")
	assert.Contains(t, out, DebugRespPrefix+"exit ")
	assert.Contains(t, out, "stderr connection refused")
}
