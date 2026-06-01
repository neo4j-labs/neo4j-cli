// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stopStartCall records one stopStartFn invocation so tests can assert the
// statement ordering relative to the docker Exec calls.
type stopStartCall struct {
	uri       string
	user      string
	pass      string
	statement string
}

// newDeployTestCfg builds a config with an empty credential store; callers seed
// the source credential via cfg.Credentials.Dbms.Add as needed.
func newDeployTestCfg(t *testing.T) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	return clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
}

// stubStopStartFn swaps the package-level stopStartFn for a recorder that
// optionally fails, restoring the original on cleanup. The returned slice
// accumulates every call in invocation order.
func stubStopStartFn(t *testing.T, failOn map[string]error) *[]stopStartCall {
	t.Helper()
	calls := &[]stopStartCall{}
	orig := stopStartFn
	t.Cleanup(func() { stopStartFn = orig })
	stopStartFn = func(_ context.Context, uri, user, pass, statement string) error {
		*calls = append(*calls, stopStartCall{uri: uri, user: user, pass: pass, statement: statement})
		if failOn != nil {
			if err, ok := failOn[statement]; ok {
				return err
			}
		}
		return nil
	}
	return calls
}

func TestPushToAura_HappyPath_Ordering(t *testing.T) {
	cfg := newDeployTestCfg(t)
	require.NoError(t, cfg.Credentials.Dbms.Add("dev", "neo4j", "srcpw", "neo4j", "neo4j://localhost:7687"))

	starts := stubStopStartFn(t, nil)
	fake := newFakeDockerClient()

	err := PushToAura(context.Background(), cfg, fake, "dev", "neo4j", AuraTarget{
		URI:      "neo4j+s://abc.databases.neo4j.io",
		Username: "neo4j",
		Password: "aurasecret",
	})
	require.NoError(t, err)

	// STOP then START on the source connection, in that order.
	require.Len(t, *starts, 2)
	assert.Equal(t, "STOP DATABASE neo4j", (*starts)[0].statement)
	assert.Equal(t, "START DATABASE neo4j", (*starts)[1].statement)
	assert.Equal(t, "neo4j://localhost:7687", (*starts)[0].uri)
	assert.Equal(t, "srcpw", (*starts)[0].pass)

	// dump -> upload -> cleanup, all against the source container.
	require.Len(t, fake.ExecCalls, 3)
	for _, c := range fake.ExecCalls {
		assert.Equal(t, "dev", c.Name)
	}
	assert.Equal(t,
		[]string{"neo4j-admin", "database", "dump", "neo4j", "--to-path=/tmp/neo4j-cli-deploy"},
		fake.ExecCalls[0].Args,
	)
	assert.Equal(t,
		[]string{
			"neo4j-admin", "database", "upload", "neo4j",
			"--from-path=/tmp/neo4j-cli-deploy",
			"--to-uri=neo4j+s://abc.databases.neo4j.io",
			"--overwrite-destination",
		},
		fake.ExecCalls[1].Args,
	)
	// The Aura credentials travel via the docker process environment, never argv.
	assert.Equal(t,
		[]string{"NEO4J_USERNAME=neo4j", "NEO4J_PASSWORD=aurasecret"},
		fake.ExecCalls[1].Env,
	)
	for _, a := range fake.ExecCalls[1].Args {
		assert.NotContains(t, a, "aurasecret", "Aura password must not appear in any docker argv element")
		assert.NotContains(t, a, "--to-password", "argv must not carry --to-password")
		assert.NotContains(t, a, "--to-user", "argv must not carry --to-user")
	}
	assert.Equal(t, []string{"rm", "-rf", "/tmp/neo4j-cli-deploy"}, fake.ExecCalls[2].Args)
}

func TestPushToAura_StartRunsWhenUploadFails(t *testing.T) {
	cfg := newDeployTestCfg(t)
	require.NoError(t, cfg.Credentials.Dbms.Add("dev", "neo4j", "srcpw", "neo4j", "neo4j://localhost:7687"))

	starts := stubStopStartFn(t, nil)

	uploadErr := errors.New("boom: upload failed")
	fake := newFakeDockerClient()
	fake.ExecFn = func(_ context.Context, _ string, args []string) (string, error) {
		if len(args) >= 3 && args[2] == "upload" {
			return "", uploadErr
		}
		return "", nil
	}

	err := PushToAura(context.Background(), cfg, fake, "dev", "neo4j", AuraTarget{
		URI:      "neo4j+s://abc.databases.neo4j.io",
		Username: "neo4j",
		Password: "aurasecret",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, uploadErr)

	// START must still run (deferred restore) even though upload failed.
	require.Len(t, *starts, 2)
	assert.Equal(t, "STOP DATABASE neo4j", (*starts)[0].statement)
	assert.Equal(t, "START DATABASE neo4j", (*starts)[1].statement)

	// Cleanup still runs after the failed upload.
	require.NotEmpty(t, fake.ExecCalls)
	last := fake.ExecCalls[len(fake.ExecCalls)-1]
	assert.Equal(t, []string{"rm", "-rf", "/tmp/neo4j-cli-deploy"}, last.Args)
}

func TestPushToAura_MissingCredential_UsageError(t *testing.T) {
	cfg := newDeployTestCfg(t)
	// No credential seeded for "dev".

	starts := stubStopStartFn(t, nil)
	fake := newFakeDockerClient()

	err := PushToAura(context.Background(), cfg, fake, "dev", "neo4j", AuraTarget{
		URI:      "neo4j+s://abc.databases.neo4j.io",
		Username: "neo4j",
		Password: "aurasecret",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no stored dbms credential named "dev"`)

	// Nothing else should have run.
	assert.Empty(t, *starts)
	assert.Empty(t, fake.ExecCalls)
}

func TestPushToAura_InjectionDatabaseName_Rejected(t *testing.T) {
	for _, name := range []string{
		"neo4j; DROP DATABASE neo4j",
		"neo4j` STOP DATABASE system; //",
		"neo4j foo",
		"",
		"1neo4j",
		"neo4j\nSTART DATABASE system",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := newDeployTestCfg(t)
			require.NoError(t, cfg.Credentials.Dbms.Add("dev", "neo4j", "srcpw", "neo4j", "neo4j://localhost:7687"))

			starts := stubStopStartFn(t, nil)
			fake := newFakeDockerClient()

			err := PushToAura(context.Background(), cfg, fake, "dev", name, AuraTarget{
				URI:      "neo4j+s://abc.databases.neo4j.io",
				Username: "neo4j",
				Password: "aurasecret",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid database name")

			// The injectable name must never reach the Cypher builder or docker.
			assert.Empty(t, *starts, "no STOP/START DATABASE statement may run for an invalid name")
			assert.Empty(t, fake.ExecCalls, "no neo4j-admin call may run for an invalid name")
		})
	}
}

func TestPushToAura_TargetPasswordAbsentFromArgvOnError(t *testing.T) {
	cfg := newDeployTestCfg(t)
	require.NoError(t, cfg.Credentials.Dbms.Add("dev", "neo4j", "srcpw", "neo4j", "neo4j://localhost:7687"))

	stubStopStartFn(t, nil)

	// Simulate the real execClient surfacing docker's argv-echoing stderr on a
	// non-zero exit. Because the secret now travels via env (not argv), the
	// echoed argv cannot contain it at all — no redaction is even required.
	const secret = "supersecret-aura-pw"
	fake := newFakeDockerClient()
	fake.ExecFn = func(_ context.Context, name string, args []string) (string, error) {
		if len(args) >= 3 && args[2] == "upload" {
			return "", errors.New(redactString("docker exec " + name + " " + strings.Join(args, " ")))
		}
		return "", nil
	}

	err := PushToAura(context.Background(), cfg, fake, "dev", "neo4j", AuraTarget{
		URI:      "neo4j+s://abc.databases.neo4j.io",
		Username: "neo4j",
		Password: secret,
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret, "Aura target password must not leak via the upload argv echo")
}
