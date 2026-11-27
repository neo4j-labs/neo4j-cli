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
	KubectlVersionCalls     int
	HelmVersionCalls        int
	ApplyCRDsCalls          int
	DeleteCRDsCalls         int
	HelmUpgradeCalls        int
	HelmUninstallCalls      int
	HelmReleasesCalls       int
	DeploymentCalls         int
	CRDCalls                int
	NamespaceCalls          int
	CurrentContextCalls     int
	GetNeo4jCalls           int
	ListNeo4jCalls          int
	CreateNeo4jCalls        int
	DeleteNeo4jCalls        int
	OperatorDeploymentCalls int

	// Recorded arguments.
	AppliedCRDSources            []string
	UpgradeOptions               []HelmUpgradeOptions
	DeploymentNames              []string
	CRDNames                     []string
	NamespaceNames               []string
	GetNeo4jNames                []string
	DeleteNeo4jNames             []string
	CreatedManifests             [][]byte
	OperatorDeploymentNamespaces []string

	// Canned return values.
	KubectlVersionResult VersionInfo
	KubectlVersionErr    error
	HelmVersionResult    string
	HelmVersionErr       error
	ApplyCRDsResult      []string
	ApplyCRDsErr         error
	DeleteCRDsResult     []string
	DeleteCRDsErr        error
	HelmUpgradeErr       error
	HelmUninstallErr     error
	HelmReleasesResult   []HelmRelease
	HelmReleasesErr      error
	DeploymentResult     DeploymentStatus
	DeploymentErr        error
	CRDResult            CRDInfo
	CRDErr               error
	// MissingNamespaces names the namespaces NamespaceExists reports absent.
	MissingNamespaces map[string]bool
	NamespaceErr      error
	// CurrentContextResult is the context CurrentContext reports.
	CurrentContextResult     string
	CurrentContextErr        error
	GetNeo4jResult           Neo4j
	GetNeo4jErr              error
	ListNeo4jResult          []Neo4j
	ListNeo4jErr             error
	CreateNeo4jResult        Neo4j
	CreateNeo4jErr           error
	DeleteNeo4jErr           error
	OperatorDeploymentResult DeploymentStatus
	OperatorDeploymentErr    error

	// Optional behaviour overrides. When set they take precedence over the
	// canned fields.
	KubectlVersionFn     func(ctx context.Context) (VersionInfo, error)
	HelmVersionFn        func(ctx context.Context) (string, error)
	ApplyCRDsFn          func(ctx context.Context, source string) ([]string, error)
	DeleteCRDsFn         func(ctx context.Context) ([]string, error)
	HelmUpgradeFn        func(ctx context.Context, opts HelmUpgradeOptions) error
	HelmUninstallFn      func(ctx context.Context) error
	HelmReleasesFn       func(ctx context.Context) ([]HelmRelease, error)
	DeploymentFn         func(ctx context.Context, name string) (DeploymentStatus, error)
	CRDFn                func(ctx context.Context, name string) (CRDInfo, error)
	GetNeo4jFn           func(ctx context.Context, name string) (Neo4j, error)
	ListNeo4jFn          func(ctx context.Context) ([]Neo4j, error)
	CreateNeo4jFn        func(ctx context.Context, manifest []byte) (Neo4j, error)
	DeleteNeo4jFn        func(ctx context.Context, name string) error
	OperatorDeploymentFn func(ctx context.Context, namespace string) (DeploymentStatus, error)
}

// NewFakeClient returns a FakeClient with a healthy default: a matching
// kubectl and server pair, a current helm, an installed and ready operator
// release and four installed CRDs. Tests override the fields they care
// about.
func NewFakeClient() *FakeClient {
	return &FakeClient{
		KubectlVersionResult: VersionInfo{
			ClientVersion: "v1.36.0",
			ServerVersion: "v1.36.0",
		},
		HelmVersionResult:    "v3.16.4",
		CurrentContextResult: "kind-kind",
		ApplyCRDsResult:      OperatorCRDNames,
		HelmReleasesResult: []HelmRelease{{
			Name:       OperatorReleaseName,
			Namespace:  DefaultOperatorNamespace,
			Revision:   "1",
			Status:     "deployed",
			Chart:      OperatorReleaseName + "-" + DefaultOperatorVersion,
			AppVersion: DefaultOperatorVersion,
		}},
		DeploymentResult: DeploymentStatus{
			Name:            OperatorDeploymentName,
			Namespace:       DefaultOperatorNamespace,
			Replicas:        1,
			ReadyReplicas:   1,
			Available:       true,
			WatchNamespaces: []string{"default"},
		},
		GetNeo4jResult:    defaultFakeNeo4j(),
		CreateNeo4jResult: defaultFakeNeo4j(),
		ListNeo4jResult:   []Neo4j{defaultFakeNeo4j()},
	}
}

func defaultFakeNeo4j() Neo4j {
	record := map[string]any{
		"apiVersion": "neo4j.com/v1",
		"kind":       "Neo4j",
		"metadata": map[string]any{
			"name": "dev", "namespace": "default", "generation": float64(1),
		},
		"spec": map[string]any{
			"edition": "enterprise", "version": DefaultNeo4jVersion,
			"topology": map[string]any{"mode": "Standalone"},
		},
		"status": map[string]any{
			"phase": "Running", "observedGeneration": float64(1),
			"serverSummary": map[string]any{"servers": float64(1), "ready": float64(1)},
			"conditions":    []any{map[string]any{"type": "Ready", "status": "True"}},
		},
	}
	instance, _ := neo4jFromRecord(record)
	return instance
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

// ApplyCRDs implements Client.
func (f *FakeClient) ApplyCRDs(ctx context.Context, source string) ([]string, error) {
	f.ApplyCRDsCalls++
	f.AppliedCRDSources = append(f.AppliedCRDSources, source)
	if f.ApplyCRDsFn != nil {
		return f.ApplyCRDsFn(ctx, source)
	}
	return f.ApplyCRDsResult, f.ApplyCRDsErr
}

// DeleteCRDs implements Client.
func (f *FakeClient) DeleteCRDs(ctx context.Context) ([]string, error) {
	f.DeleteCRDsCalls++
	if f.DeleteCRDsFn != nil {
		return f.DeleteCRDsFn(ctx)
	}
	return f.DeleteCRDsResult, f.DeleteCRDsErr
}

// HelmUpgrade implements Client.
func (f *FakeClient) HelmUpgrade(ctx context.Context, opts HelmUpgradeOptions) error {
	f.HelmUpgradeCalls++
	f.UpgradeOptions = append(f.UpgradeOptions, opts)
	if f.HelmUpgradeFn != nil {
		return f.HelmUpgradeFn(ctx, opts)
	}
	return f.HelmUpgradeErr
}

// HelmUninstall implements Client.
func (f *FakeClient) HelmUninstall(ctx context.Context) error {
	f.HelmUninstallCalls++
	if f.HelmUninstallFn != nil {
		return f.HelmUninstallFn(ctx)
	}
	return f.HelmUninstallErr
}

// HelmReleases implements Client.
func (f *FakeClient) HelmReleases(ctx context.Context) ([]HelmRelease, error) {
	f.HelmReleasesCalls++
	if f.HelmReleasesFn != nil {
		return f.HelmReleasesFn(ctx)
	}
	return f.HelmReleasesResult, f.HelmReleasesErr
}

// Deployment implements Client.
func (f *FakeClient) Deployment(ctx context.Context, name string) (DeploymentStatus, error) {
	f.DeploymentCalls++
	f.DeploymentNames = append(f.DeploymentNames, name)
	if f.DeploymentFn != nil {
		return f.DeploymentFn(ctx, name)
	}
	return f.DeploymentResult, f.DeploymentErr
}

// CRD implements Client. With no canned result it reports the requested CRD
// as installed at v1, which is what a healthy cluster returns.
func (f *FakeClient) CRD(ctx context.Context, name string) (CRDInfo, error) {
	f.CRDCalls++
	f.CRDNames = append(f.CRDNames, name)
	if f.CRDFn != nil {
		return f.CRDFn(ctx, name)
	}
	if f.CRDErr != nil || f.CRDResult.Name != "" {
		return f.CRDResult, f.CRDErr
	}
	return CRDInfo{Name: name, Versions: []string{"v1"}, StoredVersions: []string{"v1"}}, nil
}

// NamespaceExists implements Client. Every namespace exists unless it is
// named in MissingNamespaces.
func (f *FakeClient) NamespaceExists(ctx context.Context, name string) (bool, error) {
	f.NamespaceCalls++
	f.NamespaceNames = append(f.NamespaceNames, name)
	if f.NamespaceErr != nil {
		return false, f.NamespaceErr
	}
	return !f.MissingNamespaces[name], nil
}

// CurrentContext implements Client.
func (f *FakeClient) CurrentContext(context.Context) (string, error) {
	f.CurrentContextCalls++
	return f.CurrentContextResult, f.CurrentContextErr
}

// GetNeo4j implements Client.
func (f *FakeClient) GetNeo4j(ctx context.Context, name string) (Neo4j, error) {
	f.GetNeo4jCalls++
	f.GetNeo4jNames = append(f.GetNeo4jNames, name)
	if f.GetNeo4jFn != nil {
		return f.GetNeo4jFn(ctx, name)
	}
	return f.GetNeo4jResult, f.GetNeo4jErr
}

// ListNeo4j implements Client.
func (f *FakeClient) ListNeo4j(ctx context.Context) ([]Neo4j, error) {
	f.ListNeo4jCalls++
	if f.ListNeo4jFn != nil {
		return f.ListNeo4jFn(ctx)
	}
	return f.ListNeo4jResult, f.ListNeo4jErr
}

// CreateNeo4j implements Client.
func (f *FakeClient) CreateNeo4j(ctx context.Context, manifest []byte) (Neo4j, error) {
	f.CreateNeo4jCalls++
	f.CreatedManifests = append(f.CreatedManifests, append([]byte(nil), manifest...))
	if f.CreateNeo4jFn != nil {
		return f.CreateNeo4jFn(ctx, manifest)
	}
	return f.CreateNeo4jResult, f.CreateNeo4jErr
}

// DeleteNeo4j implements Client.
func (f *FakeClient) DeleteNeo4j(ctx context.Context, name string) error {
	f.DeleteNeo4jCalls++
	f.DeleteNeo4jNames = append(f.DeleteNeo4jNames, name)
	if f.DeleteNeo4jFn != nil {
		return f.DeleteNeo4jFn(ctx, name)
	}
	return f.DeleteNeo4jErr
}

// OperatorDeployment implements Client. The controller defaults to the same
// healthy Deployment that operator status uses.
func (f *FakeClient) OperatorDeployment(ctx context.Context, namespace string) (DeploymentStatus, error) {
	f.OperatorDeploymentCalls++
	f.OperatorDeploymentNamespaces = append(f.OperatorDeploymentNamespaces, namespace)
	if f.OperatorDeploymentFn != nil {
		return f.OperatorDeploymentFn(ctx, namespace)
	}
	if f.OperatorDeploymentErr != nil || f.OperatorDeploymentResult.Name != "" {
		return f.OperatorDeploymentResult, f.OperatorDeploymentErr
	}
	return f.DeploymentResult, nil
}
