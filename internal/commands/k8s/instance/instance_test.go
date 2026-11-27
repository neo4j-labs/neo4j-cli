// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"bytes"
	"testing"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/google/shlex"
	"github.com/neo4j/cli/internal/clicfg"
	k8sflags "github.com/neo4j/cli/internal/commands/k8s/flags"
	"github.com/neo4j/cli/internal/flags"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

const testManifest = `apiVersion: neo4j.com/v1
kind: Neo4j
spec:
  edition: enterprise
  version: "2026.05.0"
  license: {accept: "eval"}
  topology: {mode: Standalone}
  trust:
    enabled: true
    certificates:
      bolt: {secretName: bolt-tls}
`

type instanceEnv struct {
	fake *engine.FakeClient
	// namespaces records the namespace of each client the leaf builds, in
	// order. create builds a workload client, then an operator client.
	namespaces []string
}

func newInstanceEnv() *instanceEnv {
	return &instanceEnv{fake: engine.NewFakeClient()}
}

func runInstance(t *testing.T, env *instanceEnv, args string) (string, string, error) {
	t.Helper()
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	require.NoError(t, afero.WriteFile(fs, "/tmp/kube", []byte("apiVersion: v1\n"), 0o600))
	require.NoError(t, afero.WriteFile(fs, "/tmp/neo4j.yaml", []byte(testManifest), 0o600))
	cfg := clicfg.NewConfig(fs, "test")

	original := clientFactory
	clientFactory = func(settings engine.Settings) engine.Client {
		env.namespaces = append(env.namespaces, settings.Namespace)
		return env.fake
	}
	t.Cleanup(func() { clientFactory = original })

	root := &cobra.Command{Use: "neo4j-cli"}
	k8sflags.RegisterPersistentFlags(root)
	flags.RegisterRwFlag(root)
	flags.RegisterOutputFlag(root, cfg)
	root.PersistentPreRunE = flags.ComposeRootPersistentPreRunE(cfg)
	root.AddCommand(NewCmd(cfg))

	out := bytes.NewBuffer(nil)
	errOut := bytes.NewBuffer(nil)
	root.SetOut(out)
	root.SetErr(errOut)
	argv, err := shlex.Split(args)
	require.NoError(t, err)
	root.SetArgs(append([]string{"instance"}, argv...))
	execErr := root.Execute()
	return out.String(), errOut.String(), execErr
}
