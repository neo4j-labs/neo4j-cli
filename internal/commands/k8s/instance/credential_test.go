// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"encoding/json"
	"testing"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func credentialEnv(t *testing.T, stored string) *instanceEnv {
	t.Helper()
	fs, err := testfs.GetTestFs("{}", stored)
	require.NoError(t, err)
	env := newInstanceEnv()
	env.cfg = clicfg.NewConfig(fs, "test")
	env.fake.GetSecretResult = map[string][]byte{"NEO4J_AUTH": []byte("neo4j/rotated/secret")}
	return env
}

func TestCredential_StoresScopedConnection(t *testing.T) {
	env := credentialEnv(t, `{"dbms":{"credentials":[]}}`)
	stdout, stderr, err := runInstance(t, env, "credential dev --port 7688 --format json --rw")
	require.NoError(t, err)
	assert.Contains(t, stderr, `Targeting Kubernetes context "kind-kind" in namespace "default"`)
	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &report))
	assert.Equal(t, "k8s-kind-kind-default-dev", report["name"])
	assert.Equal(t, "bolt://127.0.0.1:7688", report["uri"])
	assert.NotContains(t, stdout, "rotated/secret")
	stored, err := env.cfg.DbmsCredentials().Get("k8s-kind-kind-default-dev")
	require.NoError(t, err)
	assert.Equal(t, "rotated/secret", stored.Password)
	assert.Equal(t, "neo4j", stored.DatabaseName)
	assert.Equal(t, "k8s-kind-kind-default-dev", env.cfg.DbmsCredentials().DefaultCredential)
	assert.Equal(t, []string{"dev-auth"}, env.fake.GetSecretNames)
	assert.Zero(t, env.fake.ListPodsCalls)
}

func TestCredential_RestoreKeepsDefaultAndEmbedLink(t *testing.T) {
	env := credentialEnv(t, `{"dbms":{"default-credential":"other","credentials":[
		{"name":"other","username":"neo4j","password":"old","database-name":"neo4j","uri":"bolt://localhost:7687"},
		{"name":"k8s-kind-kind-default-dev","username":"neo4j","password":"stale","database-name":"neo4j","uri":"bolt://127.0.0.1:7687","embed-credential":"openai-default"}]}}`)
	_, _, err := runInstance(t, env, "credential dev --port 7688 --rw")
	require.NoError(t, err)
	dbms := env.cfg.DbmsCredentials()
	assert.Len(t, dbms.List(), 2)
	assert.Equal(t, "other", dbms.DefaultCredential)
	stored, err := dbms.Get("k8s-kind-kind-default-dev")
	require.NoError(t, err)
	assert.Equal(t, "rotated/secret", stored.Password)
	assert.Equal(t, "bolt://127.0.0.1:7688", stored.URI)
	assert.Equal(t, "openai-default", stored.EmbedCredential)
	credentialsFile, err := testfs.GetTestCredentials(env.cfg.Fs())
	require.NoError(t, err)
	assert.Contains(t, credentialsFile, `"embed-credential":"openai-default"`)
}

func TestCredential_BoltTLSStoresSelfSignedURIAndWarns(t *testing.T) {
	env := credentialEnv(t, `{"dbms":{"credentials":[]}}`)
	instance := env.fake.GetNeo4jResult
	instance.BoltTLS = true
	env.fake.GetNeo4jResult = instance
	stdout, stderr, err := runInstance(t, env, "credential dev --format json --rw")
	require.NoError(t, err)
	assert.Contains(t, stderr, "accepts any server certificate")
	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &report))
	assert.Equal(t, "bolt+ssc://127.0.0.1:7687", report["uri"])
	assert.Equal(t, true, report["bolt_tls"])
	stored, err := env.cfg.DbmsCredentials().Get("k8s-kind-kind-default-dev")
	require.NoError(t, err)
	assert.Equal(t, "bolt+ssc://127.0.0.1:7687", stored.URI)
}

func TestCredential_OutputNeverShowsPassword(t *testing.T) {
	for _, format := range []string{"json", "table", "toon"} {
		t.Run(format, func(t *testing.T) {
			env := credentialEnv(t, `{"dbms":{"credentials":[]}}`)
			stdout, _, err := runInstance(t, env, "credential dev --format "+format+" --rw")
			require.NoError(t, err)
			assert.NotContains(t, stdout, "rotated/secret")
			assert.Contains(t, stdout, "bolt://127.0.0.1:7687")
		})
	}
}

func TestCredential_RejectsBadPortAndUnavailableStorage(t *testing.T) {
	t.Run("port out of range", func(t *testing.T) {
		env := credentialEnv(t, `{"dbms":{"credentials":[]}}`)
		_, _, err := runInstance(t, env, "credential dev --port 65536 --rw")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--port must be between 1 and 65535")
		assert.Zero(t, env.fake.CurrentContextCalls)
	})
	t.Run("storage unavailable", func(t *testing.T) {
		env := credentialEnv(t, `{"dbms":null}`)
		_, _, err := runInstance(t, env, "credential dev --rw")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "credential storage is not available")
		assert.Zero(t, env.fake.GetNeo4jCalls)
	})
}

func TestCredential_RejectsInvalidNameBeforeTargetPrint(t *testing.T) {
	env := credentialEnv(t, `{"dbms":{"credentials":[]}}`)
	_, _, err := runInstance(t, env, "credential --rw -- --all")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Neo4j name")
	assert.Zero(t, env.fake.CurrentContextCalls)
	assert.Zero(t, env.fake.GetSecretCalls)
}

func TestCredential_RejectsCluster(t *testing.T) {
	env := credentialEnv(t, `{"dbms":{"credentials":[]}}`)
	instance := env.fake.GetNeo4jResult
	instance.Mode = "Cluster"
	env.fake.GetNeo4jResult = instance
	_, _, err := runInstance(t, env, "credential dev --rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "clusters are not supported yet")
	assert.Zero(t, env.fake.GetSecretCalls)
	assert.Empty(t, env.cfg.DbmsCredentials().List())
}
