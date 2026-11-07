// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package operator

import (
	"errors"
	"testing"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nonInteractive pins the confirmation prompt off, so the --yes/--force
// branch is the only way through.
func nonInteractive(t *testing.T) {
	t.Helper()
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
}

func TestUninstall_KeepsTheCRDsByDefault(t *testing.T) {
	env := newOperatorEnv()
	_, _, err := runOperator(t, env, "operator uninstall --rw")
	require.NoError(t, err)

	assert.Equal(t, []string{"helm_uninstall"}, env.order)
	assert.Equal(t, 0, env.fake.DeleteCRDsCalls)
}

func TestUninstall_RemoveCRDsDeletesThemBeforeTheRelease(t *testing.T) {
	nonInteractive(t)
	env := newOperatorEnv()
	env.fake.DeleteCRDsResult = engine.OperatorCRDNames

	out, _, err := runOperator(t, env, "operator uninstall --remove-crds --yes --force --rw --format json")
	require.NoError(t, err)

	// The CRDs go first, while the controller still runs, so its finalisers
	// clear the custom resources instead of leaving them stuck.
	assert.Equal(t, []string{"delete_crds", "helm_uninstall"}, env.order)
	assert.Contains(t, out, engine.OperatorCRDNames[0])
}

func TestUninstall_RemoveCRDsNeedsConfirmation(t *testing.T) {
	nonInteractive(t)
	env := newOperatorEnv()

	_, stderr, err := runOperator(t, env, "operator uninstall --remove-crds --rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
	// The refusal and the warning name the kube context the delete would reach.
	assert.Contains(t, err.Error(), `"kind-kind"`)
	assert.Contains(t, stderr, "every Neo4j")
	assert.Empty(t, env.order, "no engine call may run without confirmation")
}

// Deleting the CRDs waits for the controller to clear its finalisers, so
// --remove-crds must refuse up front when the controller cannot do that.
func TestUninstall_RemoveCRDsRefusesWithoutAReadyController(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*engine.FakeClient)
		wantCode int
		wantErr  string
	}{
		{
			name:     "release already uninstalled",
			setup:    func(f *engine.FakeClient) { f.HelmReleasesResult = nil },
			wantCode: 3,
			wantErr:  "is not installed",
		},
		{
			name:     "controller deployment missing",
			setup:    func(f *engine.FakeClient) { f.DeploymentErr = clierr.NewNotFoundError("gone") },
			wantCode: 5,
			wantErr:  "Deployment",
		},
		{
			name:     "controller not ready",
			setup:    func(f *engine.FakeClient) { f.DeploymentResult.Available = false },
			wantCode: 5,
			wantErr:  "not ready",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newOperatorEnv()
			tc.setup(env.fake)

			_, _, err := runOperator(t, env, "operator uninstall --remove-crds --yes --force --rw")
			require.Error(t, err)
			var cliErr *clierr.CLIError
			require.True(t, errors.As(err, &cliErr))
			assert.Equal(t, tc.wantCode, cliErr.Code)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, env.order, "nothing may be deleted when the preflight fails")
		})
	}
}

func TestUninstall_MissingReleaseIsNotFound(t *testing.T) {
	env := newOperatorEnv()
	env.fake.HelmUninstallErr = clierr.NewNotFoundError("helm: release: not found")

	_, _, err := runOperator(t, env, "operator uninstall --rw")
	require.Error(t, err)
	var cliErr *clierr.CLIError
	require.True(t, errors.As(err, &cliErr))
	assert.Equal(t, 3, cliErr.Code)
}
