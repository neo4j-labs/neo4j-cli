// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package operator

import (
	"context"
	"encoding/json"
	"testing"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatus_InstalledAndReady(t *testing.T) {
	env := newOperatorEnv()
	out, _, err := runOperator(t, env, "operator status --format json")
	require.NoError(t, err)

	var report OperatorStatus
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.True(t, report.Installed)
	assert.Equal(t, engine.OperatorReleaseName, report.Release)
	assert.Equal(t, engine.DefaultOperatorNamespace, report.Namespace)
	assert.Equal(t, "deployed", report.Status)
	assert.Equal(t, engine.DefaultOperatorVersion, report.ChartVersion)
	assert.Equal(t, engine.DefaultOperatorVersion, report.AppVersion)
	assert.Equal(t, "1", report.Revision)
	assert.Equal(t, []string{"default"}, report.WatchNamespaces)
	require.NotNil(t, report.Deployment)
	assert.True(t, report.Deployment.Ready)
	assert.Equal(t, engine.OperatorDeploymentName, report.Deployment.Name)
	assert.Empty(t, report.Hint)

	require.Len(t, report.CRDs, len(engine.OperatorCRDNames))
	for _, crd := range report.CRDs {
		assert.True(t, crd.Installed, crd.Name)
		assert.Equal(t, []string{"v1"}, crd.Versions)
	}
	assert.Equal(t, len(engine.OperatorCRDNames), env.fake.CRDCalls)
}

func TestStatus_NotInstalledReportsAHintAndStillListsCRDs(t *testing.T) {
	env := newOperatorEnv()
	env.fake.HelmReleasesResult = nil

	out, _, err := runOperator(t, env, "operator status --format json")
	require.NoError(t, err)

	var report OperatorStatus
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.False(t, report.Installed)
	assert.Empty(t, report.Status)
	assert.Nil(t, report.Deployment)
	assert.Contains(t, report.Hint, "operator install")
	assert.Len(t, report.CRDs, len(engine.OperatorCRDNames))
	assert.Equal(t, 0, env.fake.DeploymentCalls, "no controller read without a release")
}

func TestStatus_ToleratesAMissingDeploymentAndCRD(t *testing.T) {
	env := newOperatorEnv()
	env.fake.DeploymentErr = clierr.NewNotFoundError("deployment gone")
	env.fake.CRDFn = func(_ context.Context, name string) (engine.CRDInfo, error) {
		if name == "neo4js.neo4j.com" {
			return engine.CRDInfo{}, clierr.NewNotFoundError("crd gone")
		}
		return engine.CRDInfo{Name: name, Versions: []string{"v1"}, StoredVersions: []string{"v1"}}, nil
	}

	out, _, err := runOperator(t, env, "operator status --format json")
	require.NoError(t, err)

	var report OperatorStatus
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.True(t, report.Installed)
	assert.Nil(t, report.Deployment)
	require.Len(t, report.CRDs, len(engine.OperatorCRDNames))
	assert.Equal(t, "neo4js.neo4j.com", report.CRDs[0].Name)
	assert.False(t, report.CRDs[0].Installed)
	assert.True(t, report.CRDs[1].Installed)
}

func TestStatus_TableOutputNamesTheSections(t *testing.T) {
	env := newOperatorEnv()
	out, _, err := runOperator(t, env, "operator status --format table")
	require.NoError(t, err)

	assert.Contains(t, out, "installed:")
	assert.Contains(t, out, "chart_version:")
	assert.Contains(t, out, "deployment:")
	assert.Contains(t, out, "ready")
	assert.Contains(t, out, "CRDs:")
	assert.Contains(t, out, "neo4js.neo4j.com")
}
