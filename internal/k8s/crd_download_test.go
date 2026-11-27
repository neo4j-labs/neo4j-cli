// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyCRDs_RemoteManifestIsVerifiedBeforeKubectl(t *testing.T) {
	stubLookPath(t)
	manifest := "apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\n"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(manifest)))
	requests := stubOperatorCRDHTTP(t, manifest, digest)
	var appliedPath string
	calls := &[]call{}
	SetRunFnForTest(t, func(_ context.Context, path string, args []string, _ []byte) runResult {
		*calls = append(*calls, call{path: path, args: args})
		appliedPath = args[len(args)-1]
		contents, err := os.ReadFile(appliedPath)
		require.NoError(t, err)
		assert.Equal(t, manifest, string(contents))
		return runResult{stdout: "customresourcedefinition.apiextensions.k8s.io/neo4js.neo4j.com serverside-applied\n"}
	})

	names, err := NewClient(Settings{}).ApplyCRDs(context.Background(), OperatorCRDManifest("1.0.0"))
	require.NoError(t, err)
	assert.Equal(t, []string{"neo4js.neo4j.com"}, names)
	assert.Equal(t, []string{
		"https://api.github.com/repos/neo4j-partners/neo4j-kubernetes-operator/releases/tags/v1.0.0",
		OperatorCRDManifest("1.0.0"),
	}, *requests)
	require.Len(t, *calls, 1)
	assert.NotEqual(t, OperatorCRDManifest("1.0.0"), appliedPath)
	_, err = os.Stat(appliedPath)
	assert.ErrorIs(t, err, os.ErrNotExist, "the verified temporary manifest is removed after kubectl")
}

func TestApplyCRDs_RemoteManifestRejectsBadDigestBeforeKubectl(t *testing.T) {
	stubLookPath(t)
	manifest := "kind: CustomResourceDefinition\n"
	stubOperatorCRDHTTP(t, manifest, strings.Repeat("0", sha256.Size*2))
	calls := captureRun(t, nil)

	_, err := NewClient(Settings{}).ApplyCRDs(context.Background(), OperatorCRDManifest("1.0.0"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operator CRD integrity verification failed")
	assert.Contains(t, err.Error(), "SHA256 mismatch")
	assert.Empty(t, *calls, "kubectl must not run for a tampered manifest")
}

func TestApplyCRDs_RemoteManifestRequiresReleaseDigest(t *testing.T) {
	stubLookPath(t)
	stubOperatorCRDHTTP(t, "kind: CustomResourceDefinition\n", "")
	calls := captureRun(t, nil)

	_, err := NewClient(Settings{}).ApplyCRDs(context.Background(), OperatorCRDManifest("1.0.0"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid SHA256 digest")
	assert.Empty(t, *calls, "kubectl must not run without a trusted digest")
}

func TestApplyCRDs_RejectsUnrecognisedRemoteSource(t *testing.T) {
	stubLookPath(t)
	calls := captureRun(t, nil)
	requests := 0
	previous := operatorCRDHTTPDoFn
	operatorCRDHTTPDoFn = func(*http.Request) (*http.Response, error) {
		requests++
		return nil, fmt.Errorf("unexpected request")
	}
	t.Cleanup(func() { operatorCRDHTTPDoFn = previous })

	_, err := NewClient(Settings{}).ApplyCRDs(context.Background(), "https://example.test/neo4j-crd-1.0.0.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTPS GitHub release asset")
	assert.Zero(t, requests)
	assert.Empty(t, *calls)
}

// stubOperatorCRDHTTP answers the two GitHub requests without network access.
// It returns the requested URLs so a test can pin the metadata and asset order.
func stubOperatorCRDHTTP(t *testing.T, manifest, digest string) *[]string {
	t.Helper()
	requests := &[]string{}
	previous := operatorCRDHTTPDoFn
	operatorCRDHTTPDoFn = func(req *http.Request) (*http.Response, error) {
		*requests = append(*requests, req.URL.String())
		var body string
		switch req.URL.String() {
		case "https://api.github.com/repos/neo4j-partners/neo4j-kubernetes-operator/releases/tags/v1.0.0":
			body = fmt.Sprintf(`{"assets":[{"name":"neo4j-crd-1.0.0.yaml","digest":"sha256:%s"}]}`, digest)
		case OperatorCRDManifest("1.0.0"):
			body = manifest
		default:
			return nil, fmt.Errorf("unexpected URL %s", req.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	}
	t.Cleanup(func() { operatorCRDHTTPDoFn = previous })
	return requests
}
