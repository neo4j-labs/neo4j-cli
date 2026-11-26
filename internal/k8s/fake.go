// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
)

// FakeClient is the in-memory Client used by command tests. It is a
// non-test file so tests in other packages can share one fake, mirroring
// the docker engine's FakeClient. Each call records itself. The canned
// fields or the per-method overrides drive the return values.
type FakeClient struct {
	// Recorded calls.
	KubectlVersionCalls int
	HelmVersionCalls    int

	// Canned return values.
	KubectlVersionResult VersionInfo
	KubectlVersionErr    error
	HelmVersionResult    string
	HelmVersionErr       error

	// Optional behaviour overrides. When set they take precedence over the
	// canned fields.
	KubectlVersionFn func(ctx context.Context) (VersionInfo, error)
	HelmVersionFn    func(ctx context.Context) (string, error)
}

// NewFakeClient returns a FakeClient with a healthy default cluster: a
// matching kubectl and server pair and a current helm. Tests override the
// fields they care about.
func NewFakeClient() *FakeClient {
	return &FakeClient{
		KubectlVersionResult: VersionInfo{
			ClientVersion: "v1.36.0",
			ServerVersion: "v1.36.0",
		},
		HelmVersionResult: "v3.16.4",
	}
}

// KubectlVersion implements Client.
func (f *FakeClient) KubectlVersion(ctx context.Context) (VersionInfo, error) {
	f.KubectlVersionCalls++
	if f.KubectlVersionFn != nil {
		return f.KubectlVersionFn(ctx)
	}
	return f.KubectlVersionResult, f.KubectlVersionErr
}

// HelmVersion implements Client.
func (f *FakeClient) HelmVersion(ctx context.Context) (string, error) {
	f.HelmVersionCalls++
	if f.HelmVersionFn != nil {
		return f.HelmVersionFn(ctx)
	}
	return f.HelmVersionResult, f.HelmVersionErr
}
