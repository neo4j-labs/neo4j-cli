// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// happyHelmList is the `helm list -o json` shape recorded against helm 4.1.1.
const happyHelmList = `[{"name":"neo4j-operator","namespace":"neo4j-operator-system","revision":"1",` +
	`"updated":"2026-11-07 16:44:38.485935 +0000 UTC","status":"deployed",` +
	`"chart":"neo4j-operator-1.0.0","app_version":"1.0.0"}]`

func TestOperatorCRDManifest(t *testing.T) {
	assert.Equal(t,
		"https://github.com/neo4j-partners/neo4j-kubernetes-operator/releases/download/v1.0.0/neo4j-crd-1.0.0.yaml",
		OperatorCRDManifest("1.0.0"))
}

func TestParseHelmReleases(t *testing.T) {
	tests := []struct {
		name   string
		stdout string
		want   []HelmRelease
	}{
		{name: "blank output", stdout: " \n", want: nil},
		{name: "empty namespace", stdout: "[]\n", want: []HelmRelease{}},
		{
			name:   "one release",
			stdout: happyHelmList,
			want: []HelmRelease{{
				Name:       "neo4j-operator",
				Namespace:  "neo4j-operator-system",
				Revision:   "1",
				Status:     "deployed",
				Chart:      "neo4j-operator-1.0.0",
				AppVersion: "1.0.0",
			}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHelmReleases(tc.stdout)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseHelmReleases_MalformedOutputIsAnError(t *testing.T) {
	_, err := parseHelmReleases("not json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "helm list")
}

func TestFindOperatorRelease(t *testing.T) {
	release, ok := FindOperatorRelease([]HelmRelease{
		{Name: "other"},
		{Name: OperatorReleaseName, Status: "deployed"},
	})
	require.True(t, ok)
	assert.Equal(t, "deployed", release.Status)

	_, ok = FindOperatorRelease([]HelmRelease{{Name: "other"}})
	assert.False(t, ok)
}

func TestHelmRelease_ChartVersion(t *testing.T) {
	assert.Equal(t, "1.0.0", HelmRelease{Chart: "neo4j-operator-1.0.0"}.ChartVersion())
	// An unexpected chart reference is returned unchanged rather than
	// silently emptied, so the report never hides what helm said.
	assert.Equal(t, "other-2.0.0", HelmRelease{Chart: "other-2.0.0"}.ChartVersion())
}

func TestParseCRDNames(t *testing.T) {
	apply := "customresourcedefinition.apiextensions.k8s.io/neo4js.neo4j.com serverside-applied\n" +
		"customresourcedefinition.apiextensions.k8s.io/neo4jbackups.neo4j.com configured\n"
	assert.Equal(t, []string{"neo4js.neo4j.com", "neo4jbackups.neo4j.com"}, parseCRDNames(apply))

	deleted := "customresourcedefinition.apiextensions.k8s.io \"neo4js.neo4j.com\" deleted\n"
	assert.Equal(t, []string{"neo4js.neo4j.com"}, parseCRDNames(deleted))
	assert.Empty(t, parseCRDNames(""))
}

func TestParseDeployment(t *testing.T) {
	tests := []struct {
		name   string
		stdout string
		want   DeploymentStatus
	}{
		{
			// A sidecar listed first must not hide the manager's image or scope.
			name: "ready with two watched namespaces behind a sidecar",
			stdout: `{
				"metadata":{"name":"neo4j-operator-controller-manager","namespace":"neo4j-operator-system","generation":2},
				"spec":{"template":{"spec":{"containers":[
					{"name":"kube-rbac-proxy","image":"quay.io/brancz/kube-rbac-proxy:v0.18.0","env":[]},
					{"name":"manager",
					"image":"ghcr.io/neo4j-partners/neo4j-kubernetes-operator:1.0.0",
					"env":[{"name":"WATCH_NAMESPACE","value":"default,team-a"},
					       {"name":"POD_NAMESPACE","valueFrom":{"fieldRef":{"fieldPath":"metadata.namespace"}}}]}]}}},
				"status":{"replicas":1,"readyReplicas":1,"observedGeneration":2,"conditions":[
					{"type":"Available","status":"True","reason":"MinimumReplicasAvailable","message":"Deployment has minimum availability."}]}
			}`,
			want: DeploymentStatus{
				Name:             "neo4j-operator-controller-manager",
				Namespace:        "neo4j-operator-system",
				Replicas:         1,
				ReadyReplicas:    1,
				Available:        true,
				AvailableReason:  "MinimumReplicasAvailable",
				AvailableMessage: "Deployment has minimum availability.",
				WatchNamespaces:  []string{"default", "team-a"},
				Image:            "ghcr.io/neo4j-partners/neo4j-kubernetes-operator:1.0.0",
			},
		},
		{
			name: "cluster-wide scope",
			stdout: `{
				"metadata":{"name":"neo4j-operator-controller-manager","namespace":"neo4j-operator-system","generation":1},
				"spec":{"replicas":1,"template":{"spec":{"containers":[{"env":[{"name":"WATCH_ALL_NAMESPACES","value":"true"}]}]}}},
				"status":{"replicas":1,"readyReplicas":1,"observedGeneration":1,"conditions":[
					{"type":"Available","status":"True","reason":"MinimumReplicasAvailable"}]}
			}`,
			want: DeploymentStatus{
				Name:               "neo4j-operator-controller-manager",
				Namespace:          "neo4j-operator-system",
				Replicas:           1,
				ReadyReplicas:      1,
				Available:          true,
				AvailableReason:    "MinimumReplicasAvailable",
				WatchAllNamespaces: true,
			},
		},
		{
			name: "unavailable condition is not ready",
			stdout: `{
				"metadata":{"name":"neo4j-operator-controller-manager","namespace":"neo4j-operator-system","generation":1},
				"spec":{"template":{"spec":{"containers":[{"env":[]}]}}},
				"status":{"replicas":1,"observedGeneration":1,"conditions":[
					{"type":"Available","status":"False","reason":"MinimumReplicasUnavailable","message":"Deployment does not have minimum availability."}]}
			}`,
			want: DeploymentStatus{
				Name:             "neo4j-operator-controller-manager",
				Namespace:        "neo4j-operator-system",
				Replicas:         1,
				Available:        false,
				AvailableReason:  "MinimumReplicasUnavailable",
				AvailableMessage: "Deployment does not have minimum availability.",
			},
		},
		{
			name: "a rolling update is not ready while a replica is outstanding",
			stdout: `{
				"metadata":{"name":"neo4j-operator-controller-manager","namespace":"neo4j-operator-system","generation":2},
				"spec":{"replicas":1,"template":{"spec":{"containers":[{"env":[]}]}}},
				"status":{"replicas":2,"readyReplicas":1,"observedGeneration":2,"conditions":[
					{"type":"Available","status":"True","reason":"MinimumReplicasAvailable"}]}
			}`,
			want: DeploymentStatus{
				Name:            "neo4j-operator-controller-manager",
				Namespace:       "neo4j-operator-system",
				Replicas:        2,
				ReadyReplicas:   1,
				Available:       false,
				AvailableReason: "MinimumReplicasAvailable",
			},
		},
		{
			name: "stale observed generation is not ready",
			stdout: `{
				"metadata":{"name":"neo4j-operator-controller-manager","namespace":"neo4j-operator-system","generation":3},
				"spec":{"template":{"spec":{"containers":[{"env":[]}]}}},
				"status":{"replicas":1,"readyReplicas":1,"observedGeneration":2,"conditions":[
					{"type":"Available","status":"True","reason":"MinimumReplicasAvailable"}]}
			}`,
			want: DeploymentStatus{
				Name:            "neo4j-operator-controller-manager",
				Namespace:       "neo4j-operator-system",
				Replicas:        1,
				ReadyReplicas:   1,
				Available:       false,
				AvailableReason: "MinimumReplicasAvailable",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDeployment(tc.stdout)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseDeployment_MalformedOutputIsAnError(t *testing.T) {
	_, err := parseDeployment("not json")
	require.Error(t, err)
}

func TestParseCRD(t *testing.T) {
	got, err := parseCRD(`{"metadata":{"name":"neo4js.neo4j.com"},"spec":{"versions":[{"name":"v1"}]},"status":{"storedVersions":["v1"]}}`)
	require.NoError(t, err)
	assert.Equal(t, CRDInfo{Name: "neo4js.neo4j.com", Versions: []string{"v1"}, StoredVersions: []string{"v1"}}, got)
}

func TestApplyCRDs_ArgvIsServerSideWithTheCLIFieldManager(t *testing.T) {
	stubLookPath(t)
	calls := captureRun(t, []runResult{{
		stdout: "customresourcedefinition.apiextensions.k8s.io/neo4js.neo4j.com serverside-applied\n",
	}})

	names, err := NewClient(Settings{
		Kubeconfig: "/tmp/kube",
		Context:    "kind-kind",
		Namespace:  "neo4j-operator-system",
	}).ApplyCRDs(context.Background(), "https://example.test/neo4j-crd-1.0.0.yaml")
	require.NoError(t, err)
	assert.Equal(t, []string{"neo4js.neo4j.com"}, names)

	require.Len(t, *calls, 1)
	assert.Equal(t, "/usr/local/bin/fake-kubectl", (*calls)[0].path)
	assert.Equal(t, []string{
		"apply", "--server-side", "--force-conflicts", "--field-manager=neo4j-cli",
		"-f", "https://example.test/neo4j-crd-1.0.0.yaml",
		"--kubeconfig", "/tmp/kube", "--context", "kind-kind", "-n", "neo4j-operator-system",
	}, (*calls)[0].args)
}

func TestDeleteCRDs_ArgvNamesEveryCRDAndIgnoresMissingOnes(t *testing.T) {
	stubLookPath(t)
	calls := captureRun(t, []runResult{{
		stdout: "customresourcedefinition.apiextensions.k8s.io \"neo4js.neo4j.com\" deleted\n",
	}})

	removed, err := NewClient(Settings{Namespace: "neo4j-operator-system"}).DeleteCRDs(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"neo4js.neo4j.com"}, removed)

	require.Len(t, *calls, 1)
	want := append([]string{"delete", "crd"}, OperatorCRDNames...)
	want = append(want, "--ignore-not-found", "-n", "neo4j-operator-system")
	assert.Equal(t, want, (*calls)[0].args)
}

func TestHelmUpgrade_Argv(t *testing.T) {
	tests := []struct {
		name string
		opts HelmUpgradeOptions
		want []string
	}{
		{
			name: "install creates the namespace and sets the scope",
			opts: HelmUpgradeOptions{
				Version:         "1.0.0",
				WatchNamespaces: []string{"default", "team-a"},
				Install:         true,
				CreateNamespace: true,
			},
			want: []string{
				"upgrade", "neo4j-operator", OperatorChart,
				"--version", "1.0.0", "--install", "--create-namespace",
				"--set", "watchNamespaces={default,team-a}",
				"-n", "neo4j-operator-system",
			},
		},
		{
			name: "upgrade reuses the deployed values and scope",
			opts: HelmUpgradeOptions{Version: "1.0.0", ReuseValues: true},
			want: []string{
				"upgrade", "neo4j-operator", OperatorChart,
				"--version", "1.0.0", "--reset-then-reuse-values",
				"-n", "neo4j-operator-system",
			},
		},
		{
			name: "upgrade overrides the scope on top of the reused values",
			opts: HelmUpgradeOptions{Version: "1.0.0", ReuseValues: true, WatchNamespaces: []string{"team-b"}},
			want: []string{
				"upgrade", "neo4j-operator", OperatorChart,
				"--version", "1.0.0", "--reset-then-reuse-values",
				"--set", "watchNamespaces={team-b}",
				"-n", "neo4j-operator-system",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stubLookPath(t)
			calls := captureRun(t, []runResult{{}})

			err := NewClient(Settings{Namespace: "neo4j-operator-system"}).
				HelmUpgrade(context.Background(), tc.opts)
			require.NoError(t, err)

			require.Len(t, *calls, 1)
			assert.Equal(t, "/usr/local/bin/fake-helm", (*calls)[0].path)
			assert.Equal(t, tc.want, (*calls)[0].args)
		})
	}
}

func TestHelmUpgrade_MissingReleaseIsNotFound(t *testing.T) {
	stubLookPath(t)
	captureRun(t, []runResult{{
		stderr: `Error: UPGRADE FAILED: "neo4j-operator" has no deployed releases`,
		err:    &exec.ExitError{},
	}})

	err := NewClient(Settings{}).HelmUpgrade(context.Background(), HelmUpgradeOptions{Version: "1.0.0"})
	require.Error(t, err)
	assert.True(t, IsNotFound(err), "a missing release must classify as not found: %v", err)
}

// TestOperatorCalls_Argv pins the argv of the engine calls whose only logic
// is the argv itself, and the one result each call derives from its output.
func TestOperatorCalls_Argv(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		result   runResult
		run      func(t *testing.T, c Client)
		// want is nil when the call must not run a child process at all.
		want []string
	}{
		{
			name:     "helm uninstall",
			settings: Settings{Namespace: "neo4j-operator-system"},
			run:      func(t *testing.T, c Client) { require.NoError(t, c.HelmUninstall(context.Background())) },
			want:     []string{"uninstall", "neo4j-operator", "-n", "neo4j-operator-system"},
		},
		{
			name:     "helm list names every live status and filters to the operator",
			settings: Settings{Namespace: "neo4j-operator-system"},
			result:   runResult{stdout: happyHelmList},
			run: func(t *testing.T, c Client) {
				releases, err := c.HelmReleases(context.Background())
				require.NoError(t, err)
				require.Len(t, releases, 1)
				assert.Equal(t, "deployed", releases[0].Status)
			},
			want: []string{
				"list", "--deployed", "--failed", "--pending", "--superseded", "--uninstalling",
				"--filter", "^neo4j-operator$", "-o", "json", "-n", "neo4j-operator-system",
			},
		},
		{
			name:     "get deployment",
			settings: Settings{Namespace: "neo4j-operator-system"},
			result:   runResult{stdout: `{"metadata":{"name":"neo4j-operator-controller-manager"}}`},
			run: func(t *testing.T, c Client) {
				_, err := c.Deployment(context.Background(), OperatorDeploymentName)
				require.NoError(t, err)
			},
			want: []string{"get", "deployment", "neo4j-operator-controller-manager", "-o", "json", "-n", "neo4j-operator-system"},
		},
		{
			name:   "get crd",
			result: runResult{stdout: `{"metadata":{"name":"neo4js.neo4j.com"}}`},
			run: func(t *testing.T, c Client) {
				_, err := c.CRD(context.Background(), "neo4js.neo4j.com")
				require.NoError(t, err)
			},
			want: []string{"get", "crd", "neo4js.neo4j.com", "-o", "json"},
		},
		{
			// A missing namespace is a false, not an error, so the command
			// can turn it into a clear local message.
			name:     "missing namespace is false, not an error",
			settings: Settings{Namespace: "neo4j-operator-system"},
			result:   runResult{stderr: `Error from server (NotFound): namespaces "ghost" not found`, err: &exec.ExitError{}},
			run: func(t *testing.T, c Client) {
				exists, err := c.NamespaceExists(context.Background(), "ghost")
				require.NoError(t, err)
				assert.False(t, exists)
			},
			want: []string{"get", "namespace", "ghost", "-o", "json", "-n", "neo4j-operator-system"},
		},
		{
			name:   "current context asks kubectl",
			result: runResult{stdout: "kind-kind\n"},
			run: func(t *testing.T, c Client) {
				got, err := c.CurrentContext(context.Background())
				require.NoError(t, err)
				assert.Equal(t, "kind-kind", got)
			},
			want: []string{"config", "current-context"},
		},
		{
			name:     "current context is the explicit --context without asking kubectl",
			settings: Settings{Context: "prod"},
			run: func(t *testing.T, c Client) {
				got, err := c.CurrentContext(context.Background())
				require.NoError(t, err)
				assert.Equal(t, "prod", got)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stubLookPath(t)
			calls := captureRun(t, []runResult{tc.result})

			tc.run(t, NewClient(tc.settings))

			if tc.want == nil {
				assert.Empty(t, *calls)
				return
			}
			require.Len(t, *calls, 1)
			assert.Equal(t, tc.want, (*calls)[0].args)
		})
	}
}
