// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package history

import (
	"os"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pinInvoker overrides the local invokerFn seam to a fixed classification and
// restores it via t.Cleanup.
func pinInvoker(t *testing.T, want string) {
	t.Helper()
	orig := invokerFn
	invokerFn = func() string { return want }
	t.Cleanup(func() { invokerFn = orig })
}

func newTestConfig(t *testing.T, config, credentials string) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetTestFs(config, credentials)
	require.NoError(t, err)
	return clicfg.NewConfig(fs, "test-version", clicfg.GlobalScope)
}

func withArgs(t *testing.T, args []string) {
	t.Helper()
	orig := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = orig })
}

func withInvoker(t *testing.T, tty bool) {
	t.Helper()
	if tty {
		pinInvoker(t, "human")
	} else {
		pinInvoker(t, "agent")
	}
}

func TestRecord_AppendsRedactedEntry(t *testing.T) {
	cfg := newTestConfig(t, `{"format":"json"}`, "{}")
	withArgs(t, []string{"neo4j-cli", "credential", "add", "--password", "supersecret"})
	withInvoker(t, true)

	Record(cfg)

	entries, err := Load(cfg)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "neo4j-cli credential add --password ***", entries[0].Command)
	assert.NotContains(t, entries[0].Command, "supersecret")
	assert.Equal(t, "human", entries[0].Invoker)
	assert.Equal(t, "test-version", entries[0].Version)
	assert.False(t, entries[0].Time.IsZero())
}

func TestRecord_InvokerResolution(t *testing.T) {
	for _, tc := range []struct {
		name string
		tty  bool
		want string
	}{
		{"tty is human", true, "human"},
		{"non-tty is agent", false, "agent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestConfig(t, `{"format":"json"}`, "{}")
			withArgs(t, []string{"neo4j-cli", "instance", "list"})
			withInvoker(t, tc.tty)

			Record(cfg)

			entries, err := Load(cfg)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Equal(t, tc.want, entries[0].Invoker)
		})
	}
}

func TestRecord_StampsInvokerFromSeam(t *testing.T) {
	cfg := newTestConfig(t, `{"format":"json"}`, "{}")
	withArgs(t, []string{"neo4j-cli", "instance", "list"})
	pinInvoker(t, "agent")

	Record(cfg)

	entries, err := Load(cfg)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "agent", entries[0].Invoker)
}

func TestRecord_TrimsToLimitKeepingLast(t *testing.T) {
	cfg := newTestConfig(t, `{"format":"json","history-limit":3}`, "{}")
	withInvoker(t, false)

	for _, n := range []string{"a", "b", "c", "d", "e"} {
		withArgs(t, []string{"neo4j-cli", n})
		Record(cfg)
	}

	entries, err := Load(cfg)
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, "neo4j-cli c", entries[0].Command)
	assert.Equal(t, "neo4j-cli d", entries[1].Command)
	assert.Equal(t, "neo4j-cli e", entries[2].Command)
}

func TestRecord_DisabledOrZeroLimitWritesNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
	}{
		{"disabled", `{"history-enabled":false}`},
		{"zero limit", `{"history-limit":0}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestConfig(t, tc.config, "{}")
			withArgs(t, []string{"neo4j-cli", "instance", "list"})
			withInvoker(t, false)

			Record(cfg)

			entries, err := Load(cfg)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

func TestRecord_WritesFileMode0600(t *testing.T) {
	cfg := newTestConfig(t, `{"format":"json"}`, "{}")
	withArgs(t, []string{"neo4j-cli", "instance", "list"})
	withInvoker(t, false)

	Record(cfg)

	info, err := cfg.Aura.Fs().Stat(path())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestRecord_CapturesCredentialAndWorkspace(t *testing.T) {
	creds := `{"aura":{"default-credential":"prod","credentials":[{"name":"prod","client-id":"id","client-secret":"sec"}]}}`
	cfg := newTestConfig(t, `{"format":"json","aura":{"default-workspace":"{org}"}}`, creds)
	withArgs(t, []string{"neo4j-cli", "instance", "list"})
	withInvoker(t, false)

	Record(cfg)

	entries, err := Load(cfg)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "prod", entries[0].Credential)
	assert.Equal(t, "{org}", entries[0].Workspace)
}

func TestLoad_SkipsCorruptLines(t *testing.T) {
	cfg := newTestConfig(t, `{"format":"json"}`, "{}")
	content := `{"time":"2026-06-01T00:00:00Z","command":"neo4j-cli a","invoker":"agent","version":"v1"}
not json at all
{"time":"2026-06-01T00:01:00Z","command":"neo4j-cli b","invoker":"agent","version":"v1"}
`
	require.NoError(t, afero.WriteFile(cfg.Aura.Fs(), path(), []byte(content), 0600))

	entries, err := Load(cfg)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "neo4j-cli a", entries[0].Command)
	assert.Equal(t, "neo4j-cli b", entries[1].Command)
}

func TestLoad_AbsentFileReturnsEmpty(t *testing.T) {
	cfg := newTestConfig(t, `{"format":"json"}`, "{}")

	entries, err := Load(cfg)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestClear_EmptiesFile(t *testing.T) {
	cfg := newTestConfig(t, `{"format":"json"}`, "{}")
	withArgs(t, []string{"neo4j-cli", "instance", "list"})
	withInvoker(t, false)
	Record(cfg)

	require.NoError(t, Clear(cfg))

	entries, err := Load(cfg)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
