// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package operator

import (
	"encoding/json"
	"testing"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstall_AppliesCRDsThenHelmAndReportsTheChange(t *testing.T) {
	env := newOperatorEnv()
	out, _, err := runOperator(t, env,
		"operator install --watch-namespace default --watch-namespace team-a --watch-namespace default --rw --format json")
	require.NoError(t, err)

	// The chart does not bundle the CRDs, so the apply must come first.
	assert.Equal(t, []string{"apply_crds", "helm_upgrade"}, env.order)
	require.Len(t, env.fake.AppliedCRDSources, 1)
	assert.Equal(t, engine.OperatorCRDManifest(engine.DefaultOperatorVersion), env.fake.AppliedCRDSources[0])
	require.Len(t, env.fake.UpgradeOptions, 1)
	assert.Equal(t, engine.HelmUpgradeOptions{
		Version: engine.DefaultOperatorVersion,
		// A repeated namespace is collapsed, and the order is kept.
		WatchNamespaces: []string{"default", "team-a"},
		Install:         true,
		CreateNamespace: true,
	}, env.fake.UpgradeOptions[0])
	assert.Equal(t, engine.DefaultOperatorNamespace, env.settings.Namespace)

	var report OperatorChange
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.Equal(t, OperatorChange{
		Release:         engine.OperatorReleaseName,
		Namespace:       engine.DefaultOperatorNamespace,
		Action:          "installed",
		ChartVersion:    engine.DefaultOperatorVersion,
		WatchNamespaces: []string{"default", "team-a"},
		CRDs:            engine.OperatorCRDNames,
	}, report)
}

func TestInstall_HonoursGroupFlagsAndTheCRDFileOverride(t *testing.T) {
	env := newOperatorEnv()
	_, _, err := runOperator(t, env,
		"operator install --kubeconfig /tmp/kube --context kind-kind -n operators --version v1.0.0 "+
			"--crd-file /tmp/crd.yaml --watch-namespace default --rw")
	require.NoError(t, err)

	assert.Equal(t, engine.Settings{
		Kubeconfig: "/tmp/kube",
		Context:    "kind-kind",
		Namespace:  "operators",
	}, env.settings)
	require.Len(t, env.fake.AppliedCRDSources, 1)
	assert.Equal(t, "/tmp/crd.yaml", env.fake.AppliedCRDSources[0])
	require.Len(t, env.fake.UpgradeOptions, 1)
	// A leading "v" is stripped, so the chart and asset see the bare version.
	assert.Equal(t, "1.0.0", env.fake.UpgradeOptions[0].Version)
}

func TestInstall_RejectsBadInputBeforeAnyEngineCall(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		setup   func(*operatorEnv)
		wantErr string
	}{
		{
			name:    "no watch scope",
			args:    "operator install --rw",
			wantErr: "--watch-namespace is required",
		},
		{
			name:    "operator namespace in the watch scope",
			args:    "operator install -n operators --watch-namespace operators --rw",
			wantErr: "must not include the operator namespace",
		},
		{
			name:    "version that is not a release version",
			args:    "operator install --watch-namespace default --version latest --rw",
			wantErr: "--version \"latest\" is not an operator release version",
		},
		{
			name:    "crd file that does not exist",
			args:    "operator install --watch-namespace default --crd-file /tmp/nope.yaml --rw",
			wantErr: "--crd-file \"/tmp/nope.yaml\" does not exist",
		},
		{
			// The chart creates a Role in each watched namespace, so a
			// namespace that does not exist must fail before the CRDs are
			// applied, not half way through the release.
			name:    "watch namespace that does not exist",
			args:    "operator install --watch-namespace ghost --rw",
			setup:   func(env *operatorEnv) { env.fake.MissingNamespaces = map[string]bool{"ghost": true} },
			wantErr: "namespace \"ghost\" does not exist",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newOperatorEnv()
			if tc.setup != nil {
				tc.setup(env)
			}
			_, _, err := runOperator(t, env, tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, env.order, "no engine call may run on a rejected invocation")
		})
	}
}
