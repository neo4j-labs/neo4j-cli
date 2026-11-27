// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"strings"
	"testing"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/clievents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNeo4j_DetectsBoltTLS(t *testing.T) {
	for _, tc := range []struct {
		name  string
		trust string
		want  bool
	}{
		{name: "absent", trust: `null`},
		{name: "certificates", trust: `{"enabled":true,"certificates":{"bolt":{"secretName":"bolt-tls"}}}`, want: true},
		{name: "cert-manager bolt policy", trust: `{"enabled":true,"certificates":{"bolt":{"secretName":"bolt-tls"}},"certManager":{"issuerRef":{"name":"neo4j-ca"}}}`, want: true},
		{name: "cert-manager without bolt policy", trust: `{"enabled":true,"certificates":{"https":{"secretName":"https-tls"}},"certManager":{"issuerRef":{"name":"neo4j-ca"}}}`},
		{name: "cluster only", trust: `{"enabled":true,"certificates":{"cluster":{"secretName":"cluster-tls"}}}`},
		{name: "HTTPS only", trust: `{"enabled":true,"certificates":{"https":{"secretName":"https-tls"}}}`},
		{name: "disabled", trust: `{"enabled":false,"certificates":{"bolt":{"secretName":"bolt-tls"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := strings.Replace(readyNeo4jJSON,
				`"topology":{"mode":"Standalone"}`, `"topology":{"mode":"Standalone"},"trust":`+tc.trust, 1)
			instance, err := parseNeo4j([]byte(record))
			require.NoError(t, err)
			assert.Equal(t, tc.want, instance.BoltTLS)
		})
	}
}

func TestReadCredential_ScopesNameAndPreservesPassword(t *testing.T) {
	for _, tc := range []struct {
		name string
		tls  bool
		uri  string
	}{
		{name: "plain Bolt", uri: "bolt://127.0.0.1:7688"},
		{name: "Bolt TLS", tls: true, uri: "bolt+ssc://127.0.0.1:7688"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := NewFakeClient()
			instance := defaultFakeNeo4j()
			instance.Namespace = "team-a"
			instance.Record["metadata"].(map[string]any)["namespace"] = "team-a"
			instance.BoltTLS = tc.tls
			fake.GetNeo4jResult = instance
			fake.GetSecretResult = map[string][]byte{"NEO4J_AUTH": []byte("neo4j/password/with/slashes")}
			credential, err := ReadCredential(t.Context(), fake, "arn:aws:eks:eu-west-1:123456789012:cluster/dev", "dev", 7688)
			require.NoError(t, err)
			assert.Equal(t, "k8s-team-a/dev@arn:aws:eks:eu-west-1:123456789012:cluster/dev", credential.Name)
			assert.Equal(t, "team-a", credential.Namespace)
			assert.Equal(t, tc.uri, credential.URI)
			assert.Equal(t, "neo4j", credential.Username)
			assert.Equal(t, "password/with/slashes", credential.Password)
			assert.Equal(t, tc.tls, credential.BoltTLS)
			assert.Equal(t, []string{"dev-auth"}, fake.GetSecretNames)
			assert.NotContains(t, clievents.RedactText("password="+credential.Password), credential.Password)
		})
	}
}

func TestReadCredential_RejectsInvalidAuth(t *testing.T) {
	for _, tc := range []struct {
		name string
		data map[string][]byte
		want string
	}{
		{name: "missing key", data: map[string][]byte{}, want: "username/password"},
		{name: "disabled", data: map[string][]byte{"NEO4J_AUTH": []byte("none")}, want: "authentication is disabled"},
		{name: "missing user", data: map[string][]byte{"NEO4J_AUTH": []byte("/s3cr3t-value")}, want: "username/password"},
		{name: "empty password", data: map[string][]byte{"NEO4J_AUTH": []byte("neo4j/")}, want: "username/password"},
		{name: "malformed", data: map[string][]byte{"NEO4J_AUTH": []byte("not-a-credential")}, want: "username/password"},
		{name: "terminal control", data: map[string][]byte{"NEO4J_AUTH": []byte("neo4j/bad\x1bs3cr3t-value")}, want: "control characters"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := NewFakeClient()
			fake.GetSecretResult = tc.data
			_, err := ReadCredential(t.Context(), fake, "kind-kind", "dev", DefaultBoltPort)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			for _, value := range tc.data {
				assert.NotContains(t, err.Error(), string(value))
			}
		})
	}
}

func TestReadCredential_DoesNotReadClusterOrMissingSecret(t *testing.T) {
	t.Run("cluster", func(t *testing.T) {
		fake := NewFakeClient()
		instance := defaultFakeNeo4j()
		instance.Mode = "Cluster"
		fake.GetNeo4jResult = instance
		_, err := ReadCredential(t.Context(), fake, "kind-kind", "dev", DefaultBoltPort)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "clusters are not supported")
		assert.Zero(t, fake.GetSecretCalls)
	})
	t.Run("missing Secret", func(t *testing.T) {
		fake := NewFakeClient()
		fake.GetSecretErr = clierr.NewNotFoundError("missing")
		_, err := ReadCredential(t.Context(), fake, "kind-kind", "dev", DefaultBoltPort)
		assert.True(t, IsNotFound(err))
		assert.Contains(t, err.Error(), "dev-auth")
	})
}

func TestStandalonePod_SelectsReadyServerByLabels(t *testing.T) {
	fake := NewFakeClient()
	fake.ListPodsResult = []Pod{
		{Name: "dev-server-1", Ready: true, Phase: "Running", BoltPort: 17687},
		{Name: "dev-server-0", Ready: true, Phase: "Running", BoltPort: 7687},
	}
	pod, err := StandalonePod(t.Context(), fake, "dev")
	require.NoError(t, err)
	assert.Equal(t, "dev-server-0", pod.Name)
	assert.Equal(t, []string{
		"app.kubernetes.io/name=neo4j,app.kubernetes.io/instance=dev,app.kubernetes.io/managed-by=neo4j-operator,neo4j.com/component=workload,neo4j.com/pool=server",
	}, fake.PodSelectors)

	fake.ListPodsResult = []Pod{
		{Name: "pending", Phase: "Pending", BoltPort: 7687},
		{Name: "not-ready", Phase: "Running", BoltPort: 7687},
		{Name: "deleting", Phase: "Running", Ready: true, Deleting: true, BoltPort: 7687},
		{Name: "no-bolt", Phase: "Running", Ready: true},
	}
	_, err = StandalonePod(t.Context(), fake, "dev")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no ready standalone pod")
}

func TestCredentialName_KeepsScopesApart(t *testing.T) {
	// Joining the parts with hyphens alone would give these two the same name.
	assert.NotEqual(t,
		CredentialName("kind-dev", "team-a", "graph"),
		CredentialName("kind", "dev-team-a", "graph"))
	assert.Equal(t, "k8s-default/dev@kind-kind", CredentialName("kind-kind", "default", "dev"))
}
