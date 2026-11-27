// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package operator

import (
	"bytes"
	"context"
	"testing"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/google/shlex"
	"github.com/neo4j/cli/internal/clicfg"
	k8sflags "github.com/neo4j/cli/internal/commands/k8s/flags"
	"github.com/neo4j/cli/internal/flags"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// operatorEnv is the per-test engine fake plus what the leaves asked it to
// do: the order of the calls (so a test can prove the CRDs are applied
// before helm runs) and the settings every call carried.
type operatorEnv struct {
	fake     *engine.FakeClient
	order    []string
	settings engine.Settings
}

// newOperatorEnv returns a fake with a healthy cluster and call recording.
func newOperatorEnv() *operatorEnv {
	env := &operatorEnv{fake: engine.NewFakeClient()}
	env.fake.ApplyCRDsFn = func(context.Context, string) ([]string, error) {
		env.order = append(env.order, "apply_crds")
		return env.fake.ApplyCRDsResult, env.fake.ApplyCRDsErr
	}
	env.fake.HelmUpgradeFn = func(context.Context, engine.HelmUpgradeOptions) error {
		env.order = append(env.order, "helm_upgrade")
		return env.fake.HelmUpgradeErr
	}
	env.fake.DeleteCRDsFn = func(context.Context) ([]string, error) {
		env.order = append(env.order, "delete_crds")
		return env.fake.DeleteCRDsResult, env.fake.DeleteCRDsErr
	}
	env.fake.HelmUninstallFn = func(context.Context) error {
		env.order = append(env.order, "helm_uninstall")
		return env.fake.HelmUninstallErr
	}
	return env
}

// runOperator drives one operator leaf through a stub root carrying the
// group's persistent flags and the real --rw gate. clientFactory returns
// env.fake, so no child process ever runs.
func runOperator(t *testing.T, env *operatorEnv, args string) (string, string, error) {
	t.Helper()
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	// The --crd-file and --kubeconfig existence checks stat the test fs.
	require.NoError(t, afero.WriteFile(fs, "/tmp/crd.yaml", []byte("kind: List\n"), 0o600))
	require.NoError(t, afero.WriteFile(fs, "/tmp/kube", []byte("apiVersion: v1\n"), 0o600))
	cfg := clicfg.NewConfig(fs, "test")

	orig := clientFactory
	clientFactory = func(settings engine.Settings) engine.Client {
		env.settings = settings
		return env.fake
	}
	t.Cleanup(func() { clientFactory = orig })

	root := &cobra.Command{Use: "neo4j-cli"}
	k8sflags.RegisterPersistentFlags(root)
	flags.RegisterRwFlag(root)
	flags.RegisterOutputFlag(root, cfg)
	root.PersistentPreRunE = flags.ComposeRootPersistentPreRunE(cfg)
	root.AddCommand(NewCmd(cfg))

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	root.SetOut(out)
	root.SetErr(errBuf)

	argv, err := shlex.Split(args)
	require.NoError(t, err)
	root.SetArgs(argv)

	// Execute before reading the buffers: Go evaluates the return operands
	// left to right, so `return out.String(), root.Execute()` would read an
	// empty buffer.
	execErr := root.Execute()
	return out.String(), errBuf.String(), execErr
}

func TestOperatorWriteLeaves_RequireRw(t *testing.T) {
	for _, args := range []string{
		"operator install --watch-namespace default",
		"operator upgrade",
		"operator uninstall",
	} {
		t.Run(args, func(t *testing.T) {
			env := newOperatorEnv()
			_, _, err := runOperator(t, env, args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--rw")
			assert.Empty(t, env.order, "the gate must fire before any engine call")
		})
	}
}
