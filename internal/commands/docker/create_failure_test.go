// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clicfg/credentials"
	"github.com/neo4j/cli/internal/clierr"
	engine "github.com/neo4j/cli/internal/docker"
	"github.com/neo4j/cli/internal/flags"
	"github.com/neo4j/cli/internal/testutil/testfs"
)

const emptyCredsJSON = `{
	"dbms": {"credentials": [], "default-credential": ""},
	"embed": {"credentials": [], "default-credential": ""}
}`

// runCreateWithCreds is runCreate with a caller-supplied credentials.json, so a
// test can make credential storage unavailable.
func runCreateWithCreds(t *testing.T, credsJSON, args string) (*engine.FakeClient, string, string, error) {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, credsJSON)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test")

	fake := engine.NewFakeClient()
	origFactory := clientFactory
	clientFactory = func(bool) engine.Client { return fake }
	t.Cleanup(func() { clientFactory = origFactory })
	stubListenerFactory(t)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"create"}, argv...))

	execErr := cmd.Execute()
	return fake, out.String(), errBuf.String(), execErr
}

// failStore makes the post-start credential write fail, the one failure that
// cannot be predicted before the container exists.
func failStore(t *testing.T, err error) {
	t.Helper()
	orig := storeCredentialFn
	storeCredentialFn = func(*credentials.DbmsCredentials, string, string, string) error { return err }
	t.Cleanup(func() { storeCredentialFn = orig })
}

func TestCreate_CredentialStorageUnavailable_FailsBeforeAnyDockerCall(t *testing.T) {
	fake, _, _, err := runCreateWithCreds(t, `{"dbms": null}`, "--name dev")

	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "credential storage is not available; use --no-store-credential")
	assert.Empty(t, fake.RunCalls, "no container may be started when its password could not be recorded")
	assert.Empty(t, fake.PsAllCalls, "and docker is not even consulted")
}

func TestCreate_CredentialStorageUnavailable_IsFineWhenNotStoring(t *testing.T) {
	for _, extra := range []string{"--no-store-credential", "--ephemeral"} {
		t.Run(extra, func(t *testing.T) {
			fake, _, _, err := runCreateWithCreds(t, `{"dbms": null}`, "--name dev "+extra+"")

			require.NoError(t, err)
			assert.Len(t, fake.RunCalls, 1)
		})
	}
}

func TestCreate_ReservedCredentialName_FailsBeforeStartingAContainer(t *testing.T) {
	for _, name := range []string{"desktop", "desktop-connection:abc"} {
		t.Run(name, func(t *testing.T) {
			fake, _, _, err := runCreateWithCreds(t, emptyCredsJSON, "--name "+name+"")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "reserved")
			assert.Empty(t, fake.RunCalls, "the container used to be created first and the credential rejected afterwards")
		})
	}
}

func TestCreate_StoringTheCredentialFailsAfterStart_PasswordShown_WarnsAndKeepsTheContainer(t *testing.T) {
	pw := stubRandSource(t)
	failStore(t, errors.New("keyring locked"))

	fake, stdout, stderr, err := runCreateWithCreds(t, emptyCredsJSON, "--name dev --format json")

	require.NoError(t, err, "the operator is given the password, so the container is usable")
	assert.Contains(t, stdout, pw, "the generated password is printed")
	assert.Contains(t, stderr, "Warning: failed to store credentials locally (keyring locked)")
	assert.Contains(t, stderr, "Save the password now")
	assert.Empty(t, fake.RemoveForceCalls, "nothing is removed when the operator holds the password")
}

func TestCreate_StoringTheCredentialFailsAfterStart_PasswordHidden_RemovesTheOrphan(t *testing.T) {
	pw := stubRandSource(t)
	failStore(t, errors.New("keyring locked"))

	fake, stdout, stderr, err := runCreateWithCreds(t, emptyCredsJSON, "--name dev --no-print-password --format json")

	require.Error(t, err)
	assert.Equal(t, []string{"dev"}, fake.RemoveForceCalls, "a container nobody can log in to is not left running")
	assert.Contains(t, err.Error(), "container was removed")
	assert.Contains(t, err.Error(), "keyring locked")
	assert.NotContains(t, stdout+stderr+err.Error(), pw, "the hidden password never leaks")
}

func TestCreate_StoringTheCredentialFailsAfterStart_PasswordHidden_ButSupplied_KeepsTheContainer(t *testing.T) {
	failStore(t, errors.New("keyring locked"))

	fake, _, stderr, err := runCreateWithCreds(t, emptyCredsJSON, "--name dev --password mine --no-print-password --format json")

	require.NoError(t, err, "the operator chose the password, so nothing is lost")
	assert.Empty(t, fake.RemoveForceCalls)
	assert.Contains(t, stderr, "Warning: failed to store credentials locally")
}

func TestCreate_StoringTheCredentialFailsAfterStart_CleanupFailureTellsTheOperator(t *testing.T) {
	stubRandSource(t)
	failStore(t, errors.New("keyring locked"))
	fakeRemoveFails := func(fake *engine.FakeClient) {
		fake.RemoveForceFn = func(context.Context, string) error { return errors.New("daemon gone") }
	}
	origFactory := clientFactory
	t.Cleanup(func() { clientFactory = origFactory })

	fake := engine.NewFakeClient()
	fakeRemoveFails(fake)
	fs, err := testfs.GetTestFs(`{}`, emptyCredsJSON)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test")
	clientFactory = func(bool) engine.Client { return fake }
	stubListenerFactory(t)
	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	argv, _ := shlex.Split("--name dev --no-print-password")
	cmd.SetArgs(append([]string{"create"}, argv...))

	execErr := cmd.Execute()

	require.Error(t, execErr)
	assert.Contains(t, execErr.Error(), "could not remove the container")
	assert.Contains(t, execErr.Error(), "docker delete dev")
}

func TestCreate_Plugins(t *testing.T) {
	t.Run("repeated flags become NEO4J_PLUGINS, between the licence variable and the labels", func(t *testing.T) {
		fake, _, _, err := runCreate(t, "--name dev --plugin apoc --plugin graph-data-science")

		require.NoError(t, err)
		argv := runArgv(t, fake)
		joined := strings.Join(argv, " ")
		assert.Contains(t, joined, `-e NEO4J_PLUGINS=["apoc","graph-data-science"]`)
		assert.Less(t, strings.Index(joined, "NEO4J_ACCEPT_LICENSE_AGREEMENT"), strings.Index(joined, "NEO4J_PLUGINS"))
		assert.Less(t, strings.Index(joined, "NEO4J_PLUGINS"), strings.Index(joined, "--label"))
	})

	t.Run("a comma-separated value works and duplicates collapse", func(t *testing.T) {
		fake, _, _, err := runCreate(t, "--name dev --plugin apoc,n10s,apoc")

		require.NoError(t, err)
		assert.Contains(t, strings.Join(runArgv(t, fake), " "), `NEO4J_PLUGINS=["apoc","n10s"]`)
	})

	t.Run("community and ephemeral containers take plugins too", func(t *testing.T) {
		fake, _, _, err := runCreate(t, "--name dev --edition community --ephemeral --plugin apoc")

		require.NoError(t, err)
		assert.Contains(t, strings.Join(runArgv(t, fake), " "), `NEO4J_PLUGINS=["apoc"]`)
	})

	t.Run("no flag, no variable", func(t *testing.T) {
		fake, _, _, err := runCreate(t, "--name dev")

		require.NoError(t, err)
		assert.NotContains(t, strings.Join(runArgv(t, fake), " "), "NEO4J_PLUGINS")
	})

	t.Run("the result lists the plugins that were requested", func(t *testing.T) {
		_, _, stdout, err := runCreate(t, "--name dev --plugin apoc --format json")

		require.NoError(t, err)
		assert.Contains(t, stdout, `"plugins"`)
		assert.Contains(t, stdout, `"apoc"`)
	})

	t.Run("a bad plugin name is a usage error and nothing reaches docker", func(t *testing.T) {
		for _, bad := range []string{"Bad Name", "UPPER", "../apoc", "apoc;rm"} {
			fake, _, _, err := runCreate(t, "--name dev --plugin "+shlexQuote(bad)+"")

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce), bad)
			assert.Equal(t, 2, ce.Code)
			assert.Empty(t, fake.RunCalls, bad)
			assert.Empty(t, fake.PsAllCalls, bad)
		}
	})
}
