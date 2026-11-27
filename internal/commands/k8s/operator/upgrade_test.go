// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package operator

import (
	"errors"
	"testing"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpgrade_AppliesCRDsThenReusesTheDeployedValues(t *testing.T) {
	env := newOperatorEnv()
	_, _, err := runOperator(t, env, "operator upgrade --rw")
	require.NoError(t, err)

	assert.Equal(t, []string{"apply_crds", "helm_upgrade"}, env.order)
	require.Len(t, env.fake.UpgradeOptions, 1)
	// No --watch-namespace: the deployed scope and every other hand-set
	// value survive, because helm reuses the release's values.
	assert.Equal(t, engine.HelmUpgradeOptions{
		Version:     engine.DefaultOperatorVersion,
		ReuseValues: true,
	}, env.fake.UpgradeOptions[0])
}

func TestUpgrade_ChangingTheScopeOverridesTheReusedValues(t *testing.T) {
	env := newOperatorEnv()
	_, _, err := runOperator(t, env, "operator upgrade --watch-namespace team-b --rw")
	require.NoError(t, err)

	require.Len(t, env.fake.UpgradeOptions, 1)
	assert.Equal(t, engine.HelmUpgradeOptions{
		Version:         engine.DefaultOperatorVersion,
		WatchNamespaces: []string{"team-b"},
		ReuseValues:     true,
	}, env.fake.UpgradeOptions[0])
}

// Without --version the upgrade moves to the pinned release but never rolls
// a newer deployed release back; an explicit --version always wins.
func TestUpgrade_ChoosesTheChartVersion(t *testing.T) {
	tests := []struct {
		name     string
		deployed string
		args     string
		want     string
	}{
		{name: "older deployed moves to the pinned release", deployed: "0.9.0", args: "", want: engine.DefaultOperatorVersion},
		{name: "newer deployed is kept", deployed: "9.0.0", args: "", want: "9.0.0"},
		{name: "explicit version may downgrade", deployed: "9.0.0", args: "--version 1.0.0", want: "1.0.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newOperatorEnv()
			env.fake.HelmReleasesResult[0].Chart = engine.OperatorReleaseName + "-" + tc.deployed

			_, _, err := runOperator(t, env, "operator upgrade --rw "+tc.args)
			require.NoError(t, err)
			require.Len(t, env.fake.UpgradeOptions, 1)
			assert.Equal(t, tc.want, env.fake.UpgradeOptions[0].Version)
			assert.Equal(t, engine.OperatorCRDManifest(tc.want), env.fake.AppliedCRDSources[0])
		})
	}
}

// Every refusal fires before the CRDs are applied, so the cluster is left as
// it was.
func TestUpgrade_RefusesBeforeTouchingTheCluster(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		setup    func(*engine.FakeClient)
		wantCode int
		wantErr  string
	}{
		{
			name:     "missing release",
			setup:    func(f *engine.FakeClient) { f.HelmReleasesResult = nil },
			wantCode: 3,
			wantErr:  "operator install",
		},
		{
			name:     "helm older than --reset-then-reuse-values",
			setup:    func(f *engine.FakeClient) { f.HelmVersionResult = "v3.13.3" },
			wantCode: 2,
			wantErr:  "helm v3.14",
		},
		{
			name:     "version that is not a release version",
			args:     "--version 1.0",
			setup:    func(*engine.FakeClient) {},
			wantCode: 2,
			wantErr:  "not an operator release version",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newOperatorEnv()
			tc.setup(env.fake)

			_, _, err := runOperator(t, env, "operator upgrade --rw "+tc.args)
			require.Error(t, err)
			var cliErr *clierr.CLIError
			require.True(t, errors.As(err, &cliErr))
			assert.Equal(t, tc.wantCode, cliErr.Code)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, env.order)
		})
	}
}
