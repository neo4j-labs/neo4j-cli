// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/neo4j/cli/internal/clicfg/credentials"
	"github.com/neo4j/cli/internal/clicfg/envfile"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/dbconn"
	engine "github.com/neo4j/cli/internal/k8s"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boltProbe records the connection that reaches readiness and returns the
// result of respond. A nil respond reports a ready server.
type boltProbe struct {
	uri      string
	username string
	password string
	timeout  time.Duration
	respond  func() error
}

// connectEnv seeds a ready standalone server and replaces the Bolt probe, so
// no test opens a driver. The fake port-forward returns as soon as the command
// reports readiness. A test that needs the blocking loop clears PortForwardFn.
func connectEnv(t *testing.T) (*instanceEnv, *boltProbe) {
	t.Helper()
	env := credentialEnv(t, `{"dbms":{"credentials":[]}}`)
	env.ctx = t.Context()
	probe := &boltProbe{}
	original := waitForBoltFn
	waitForBoltFn = func(_ context.Context, uri, username, password string, timeout time.Duration) error {
		probe.uri, probe.username, probe.password, probe.timeout = uri, username, password, timeout
		if probe.respond != nil {
			return probe.respond()
		}
		return nil
	}
	t.Cleanup(func() { waitForBoltFn = original })
	env.fake.PortForwardFn = func(ctx context.Context, opts engine.PortForwardOptions) error {
		return opts.OnReady(ctx, opts.LocalPort)
	}
	return env, probe
}

// parseEnvOutput reads the dotenv text back with the same reader that
// `query --env` uses, so escaping is tested end to end.
func parseEnvOutput(t *testing.T, env *instanceEnv, contents string) map[string]string {
	t.Helper()
	require.NoError(t, afero.WriteFile(env.cfg.Fs(), "/tmp/connect.env", []byte(contents), 0o600))
	vals, _, err := envfile.Parse(env.cfg.Fs(), "/tmp/connect.env")
	require.NoError(t, err)
	return vals
}

func TestConnect_ForwardsReadyPodAndPrintsDotenv(t *testing.T) {
	env, probe := connectEnv(t)
	stdout, stderr, err := runInstance(t, env, "connect dev --port 7688 --rw")
	require.NoError(t, err)
	require.Len(t, env.fake.ForwardOptions, 1)
	opts := env.fake.ForwardOptions[0]
	assert.Equal(t, "dev-server-0", opts.Pod)
	assert.Equal(t, int32(7687), opts.RemotePort)
	assert.Equal(t, 7688, opts.LocalPort)
	assert.Equal(t, "bolt://127.0.0.1:7688", probe.uri)
	assert.Equal(t, "neo4j", probe.username)
	assert.Equal(t, "rotated/secret", probe.password)
	assert.Equal(t, 60*time.Second, probe.timeout)
	assert.Contains(t, stdout, "# neo4j-cli k8s\n")
	assert.Contains(t, stderr, "Connection ready. Press Ctrl-C to stop.")
	vals := parseEnvOutput(t, env, stdout)
	assert.Equal(t, "bolt://127.0.0.1:7688", vals[credentials.EnvURI])
	assert.Equal(t, "neo4j", vals[credentials.EnvUsername])
	assert.Equal(t, "rotated/secret", vals[credentials.EnvPassword])
	assert.Equal(t, "neo4j", vals[credentials.EnvDatabase])
	assert.Empty(t, env.cfg.DbmsCredentials().List(), "connect leaves the credential store alone")
}

func TestConnect_WritesEnvFileWithOwnerOnlyPermissions(t *testing.T) {
	env, _ := connectEnv(t)
	require.NoError(t, afero.WriteFile(env.cfg.Fs(), "/tmp/dev.env", []byte("NEO4J_PASSWORD=stale\n"), 0o644))
	stdout, stderr, err := runInstance(t, env, "connect dev --env-out-file /tmp/dev.env --rw")
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, `Connection details written to "/tmp/dev.env".`)
	info, err := env.cfg.Fs().Stat("/tmp/dev.env")
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
	contents, err := afero.ReadFile(env.cfg.Fs(), "/tmp/dev.env")
	require.NoError(t, err)
	assert.NotContains(t, string(contents), "stale")
	assert.Equal(t, "rotated/secret", parseEnvOutput(t, env, string(contents))[credentials.EnvPassword])
	entries, err := afero.ReadDir(env.cfg.Fs(), "/tmp")
	require.NoError(t, err)
	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), ".neo4j-cli-env-", "a successful write leaves no temporary file")
	}
}

func TestConnect_QuotesPasswordForDotenv(t *testing.T) {
	for _, tc := range []struct {
		name     string
		password string
	}{
		{name: "dollar and backslash", password: `pa$$"wo\rd` + "\tkey"},
		{name: "apostrophe", password: "it's a password"},
		{name: "apostrophe and variable", password: `it's $HOME`},
		{name: "line endings", password: "first\nsecond\rthird"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := connectEnv(t)
			env.fake.GetSecretResult = map[string][]byte{"NEO4J_AUTH": []byte("neo4j/" + tc.password)}
			stdout, _, err := runInstance(t, env, "connect dev --rw")
			require.NoError(t, err)
			assert.Equal(t, tc.password, parseEnvOutput(t, env, stdout)[credentials.EnvPassword])
		})
	}
}

func TestConnect_RefusesPasswordDotenvCannotCarry(t *testing.T) {
	for _, password := range []string{`it's \new`, `trailing\`} {
		t.Run(password, func(t *testing.T) {
			env, _ := connectEnv(t)
			env.fake.GetSecretResult = map[string][]byte{"NEO4J_AUTH": []byte("neo4j/" + password)}
			stdout, _, err := runInstance(t, env, "connect dev --rw")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot write NEO4J_PASSWORD")
			assert.NotContains(t, err.Error(), password)
			var failure *clierr.CLIError
			require.ErrorAs(t, err, &failure)
			assert.Contains(t, failure.Suggestion, "instance credential dev")
			assert.Empty(t, stdout)
			assert.Empty(t, env.cfg.DbmsCredentials().List())
		})
	}
}

func TestConnect_BoltTLSUsesSelfSignedURIAndWarns(t *testing.T) {
	env, probe := connectEnv(t)
	instance := env.fake.GetNeo4jResult
	instance.BoltTLS = true
	env.fake.GetNeo4jResult = instance
	stdout, stderr, err := runInstance(t, env, "connect dev --rw")
	require.NoError(t, err)
	assert.Contains(t, stderr, "accepts any server certificate")
	assert.Equal(t, "bolt+ssc://127.0.0.1:7687", probe.uri)
	assert.Equal(t, "bolt+ssc://127.0.0.1:7687", parseEnvOutput(t, env, stdout)[credentials.EnvURI])
}

func TestConnect_ReadinessTimeoutPointsAtTheInstance(t *testing.T) {
	env, probe := connectEnv(t)
	probe.respond = func() error {
		return &dbconn.BoltTimeoutError{Timeout: dbconn.DefaultWaitTimeout}
	}
	stdout, _, err := runInstance(t, env, "connect dev --rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `Neo4j "dev" did not accept an authenticated connection through pod dev-server-0`)
	assert.Empty(t, stdout)
	var failure *clierr.CLIError
	require.ErrorAs(t, err, &failure)
	assert.Contains(t, failure.Suggestion, "k8s instance get dev")
}

func TestConnect_StopsWhenTheContextIsCancelled(t *testing.T) {
	env, probe := connectEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env.ctx = ctx
	// The blocking fake keeps the forward open, as kubectl does, until the
	// command context ends. Ctrl-C cancels that context after readiness.
	env.fake.PortForwardFn = nil
	probe.respond = func() error {
		cancel()
		return nil
	}
	stdout, _, err := runInstance(t, env, "connect dev --env-out-file /tmp/dev.env --rw")
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, stdout)
	assert.Equal(t, 1, env.fake.PortForwardCalls)
	contents, err := afero.ReadFile(env.cfg.Fs(), "/tmp/dev.env")
	require.NoError(t, err)
	assert.Equal(t, "rotated/secret", parseEnvOutput(t, env, string(contents))[credentials.EnvPassword])
}

func TestConnect_NoReadyPodFailsBeforeForwarding(t *testing.T) {
	env, _ := connectEnv(t)
	env.fake.ListPodsResult = []engine.Pod{{Name: "dev-server-0", Phase: "Pending", BoltPort: 7687}}
	_, _, err := runInstance(t, env, "connect dev --rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ready standalone pod")
	assert.Zero(t, env.fake.PortForwardCalls)
	assert.Empty(t, env.cfg.DbmsCredentials().List())
}

func TestConnect_RejectsBadPort(t *testing.T) {
	env, _ := connectEnv(t)
	_, _, err := runInstance(t, env, "connect dev --port 0 --rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--port must be between 1 and 65535")
	assert.Zero(t, env.fake.CurrentContextCalls)
}

func TestConnect_RejectsInvalidNameBeforeTargetPrint(t *testing.T) {
	env, _ := connectEnv(t)
	_, _, err := runInstance(t, env, "connect --rw -- --all")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Neo4j name")
	assert.Zero(t, env.fake.CurrentContextCalls)
	assert.Zero(t, env.fake.PortForwardCalls)
}
