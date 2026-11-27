// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func validManifestOptions() Neo4jManifestOptions {
	return Neo4jManifestOptions{
		Name: "dev", Namespace: "default", Mode: "standalone", Primaries: 3,
		Version: DefaultNeo4jVersion, StorageSize: DefaultStorageSize,
		Edition: "enterprise", AcceptLicense: "eval", ServiceType: "ClusterIP",
	}
}

func TestBuildNeo4jManifest_RendersTypedOptions(t *testing.T) {
	opts := validManifestOptions()
	opts.StorageClass = "managed-csi"
	opts.Plugins = []string{"apoc", "apoc"}
	opts.Memory = "8Gi"
	opts.CPU = "2"
	opts.Heap = "2g"
	opts.PageCache = "1g"
	opts.Config = []string{"db.transaction.timeout=60s", "dbms.security.auth_enabled=true"}

	manifest, err := BuildNeo4jManifest(opts)
	require.NoError(t, err)
	text := string(manifest)
	for _, expected := range []string{
		"apiVersion: neo4j.com/v1",
		"kind: Neo4j",
		"app.kubernetes.io/managed-by: neo4j-cli",
		"version: \"2026.05.0\"",
		"accept: \"eval\"",
		"mode: Standalone",
		"size: \"10Gi\"",
		"storageClassName: \"managed-csi\"",
		"db.transaction.timeout: \"60s\"",
		"dbms.security.auth_enabled: \"true\"",
		"server.memory.heap.initial_size: \"2g\"",
		"server.memory.heap.max_size: \"2g\"",
		"server.memory.pagecache.size: \"1g\"",
		"cpu: \"2\"",
		"memory: \"8Gi\"",
		"- apoc",
	} {
		assert.Contains(t, text, expected)
	}
	assert.Equal(t, 1, strings.Count(text, "- apoc"), "repeated plugins are deduplicated")

	var decoded map[string]any
	require.NoError(t, yaml.Unmarshal(manifest, &decoded))
	spec := decoded["spec"].(map[string]any)
	config := spec["config"].(map[string]any)["neo4j"].(map[string]any)
	assert.IsType(t, "", config["dbms.security.auth_enabled"], "YAML booleans remain strings")
}

func TestBuildNeo4jManifest_LocalAdmissionRules(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*Neo4jManifestOptions)
		wantErr string
	}{
		{name: "enterprise licence is explicit", mutate: func(o *Neo4jManifestOptions) { o.AcceptLicense = "" }, wantErr: "licence acceptance"},
		{name: "primaries are odd", mutate: func(o *Neo4jManifestOptions) {
			o.Mode, o.Primaries, o.InsecureAdminConnection = "cluster", 2, true
		}, wantErr: "must be odd"},
		{name: "primaries have an upper bound", mutate: func(o *Neo4jManifestOptions) {
			o.Mode, o.Primaries, o.InsecureAdminConnection = "cluster", 17, true
		}, wantErr: "must be odd"},
		{name: "cluster needs an admin path", mutate: func(o *Neo4jManifestOptions) { o.Mode = "cluster" }, wantErr: "requires --insecure-admin-connection"},
		{name: "community cannot cluster", mutate: func(o *Neo4jManifestOptions) {
			o.Mode, o.Edition, o.InsecureAdminConnection = "cluster", "community", true
		}, wantErr: "only standalone"},
		{name: "standalone rejects primaries", mutate: func(o *Neo4jManifestOptions) { o.PrimariesSet = true }, wantErr: "--primaries"},
		{name: "standalone rejects insecure cluster admin", mutate: func(o *Neo4jManifestOptions) { o.InsecureAdminConnection = true }, wantErr: "--insecure-admin-connection"},
		{name: "cluster rejects top-level plugins", mutate: func(o *Neo4jManifestOptions) {
			o.Mode, o.InsecureAdminConnection, o.Plugins = "cluster", true, []string{"apoc"}
		}, wantErr: "only in standalone mode"},
		{name: "plugin catalogue is enforced", mutate: func(o *Neo4jManifestOptions) { o.Plugins = []string{"unknown"} }, wantErr: "not one of"},
		{name: "load balancer needs a source range", mutate: func(o *Neo4jManifestOptions) { o.ServiceType = "LoadBalancer" }, wantErr: "source-range"},
		{name: "source ranges need a load balancer", mutate: func(o *Neo4jManifestOptions) {
			o.LoadBalancerSourceRanges = []string{"10.0.0.0/8"}
		}, wantErr: "requires --service-type LoadBalancer"},
		{name: "default memory cannot hold the documentation example", mutate: func(o *Neo4jManifestOptions) {
			o.Heap, o.PageCache = "2g", "4g"
		}, wantErr: "Raise --memory"},
		{name: "operator-owned keys are refused", mutate: func(o *Neo4jManifestOptions) {
			o.Config = []string{"server.bolt.enabled=true"}
		}, wantErr: "owned by the operator"},
		{name: "dedicated heap flag owns heap keys", mutate: func(o *Neo4jManifestOptions) {
			o.Config = []string{"server.memory.heap.max_size=2g"}
		}, wantErr: "with --heap, not --config"},
		{name: "dedicated page cache flag owns its key", mutate: func(o *Neo4jManifestOptions) {
			o.Config = []string{"server.memory.pagecache.size=1g"}
		}, wantErr: "with --pagecache, not --config"},
		{name: "configuration cannot repeat a key", mutate: func(o *Neo4jManifestOptions) {
			o.Config = []string{"db.transaction.timeout=1s", "db.transaction.timeout=2s"}
		}, wantErr: "repeated"},
		{name: "memory takes a Kubernetes quantity", mutate: func(o *Neo4jManifestOptions) {
			o.Memory, o.Heap = "8g", "2g"
		}, wantErr: "invalid memory limit"},
		{name: "storage ceiling matches the CRD", mutate: func(o *Neo4jManifestOptions) { o.StorageSize = "17Ti" }, wantErr: "16Ti"},
		{name: "resource name must be a DNS label", mutate: func(o *Neo4jManifestOptions) { o.Name = "My_DB" }, wantErr: "invalid Neo4j name"},
		{name: "resource name rejects leading hyphen", mutate: func(o *Neo4jManifestOptions) { o.Name = "-dev" }, wantErr: "invalid Neo4j name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := validManifestOptions()
			tc.mutate(&opts)
			_, err := BuildNeo4jManifest(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestBuildNeo4jManifest_MemoryCheckUsesExplicitLimit(t *testing.T) {
	opts := validManifestOptions()
	opts.Memory, opts.Heap, opts.PageCache = "8Gi", "2g", "4g"
	_, err := BuildNeo4jManifest(opts)
	require.NoError(t, err)
}

func TestPrepareNeo4jManifestFromFile_PreservesTLSAndAddsIdentity(t *testing.T) {
	input := `apiVersion: neo4j.com/v1
kind: Neo4j
metadata:
  name: old-name
  labels:
    team: database
spec:
  edition: enterprise
  version: "2026.05.0"
  license:
    accept: "yes"
  topology:
    mode: Cluster
    primaries:
      members: 3
  trust:
    enabled: true
    certificates:
      bolt:
        secretName: bolt-tls
      cluster:
        secretName: cluster-tls
  config:
    neo4j:
      db.transaction.timeout: "60s"
`
	manifest, err := PrepareNeo4jManifestFromFile([]byte(input), "dev", "team-a")
	require.NoError(t, err)
	text := string(manifest)
	assert.Contains(t, text, "name: dev")
	assert.Contains(t, text, "namespace: team-a")
	assert.Contains(t, text, "team: database")
	assert.Contains(t, text, "app.kubernetes.io/managed-by: neo4j-cli")
	assert.Contains(t, text, "secretName: bolt-tls")
	assert.Contains(t, text, "secretName: cluster-tls")
	assert.NotContains(t, text, "old-name")

	certManagerInput := `apiVersion: neo4j.com/v1
kind: Neo4j
metadata:
spec:
  edition: enterprise
  version: "2026.05.0"
  license:
    accept: "eval"
  topology:
    mode: Cluster
    primaries:
      members: 3
  trust:
    enabled: true
    certificates:
      bolt:
        secretName: bolt-tls
    certManager:
      issuerRef:
        name: neo4j-ca
        kind: ClusterIssuer
`
	certManagerManifest, err := PrepareNeo4jManifestFromFile([]byte(certManagerInput), "dev", "")
	require.NoError(t, err)
	assert.Contains(t, string(certManagerManifest), "issuerRef:")
	assert.NotContains(t, string(certManagerManifest), "namespace:")
}

func TestPrepareNeo4jManifestFromFile_CertManagerNeedsABoltPolicy(t *testing.T) {
	input := `apiVersion: neo4j.com/v1
kind: Neo4j
spec:
  edition: enterprise
  version: "2026.05.0"
  license: {accept: "eval"}
  topology:
    mode: Cluster
    primaries:
      members: 3
  trust:
    enabled: true
    certManager:
      issuerRef:
        name: neo4j-ca
`
	// The operator rejects a cluster whose only trust setting is certManager,
	// because cert-manager issues just the policies listed under certificates.
	_, err := PrepareNeo4jManifestFromFile([]byte(input), "dev", "default")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cluster mode requires")

	withBolt := strings.Replace(input, "    certManager:\n",
		"    certificates:\n      bolt:\n        secretName: bolt-tls\n    certManager:\n", 1)
	manifest, err := PrepareNeo4jManifestFromFile([]byte(withBolt), "dev", "default")
	require.NoError(t, err)
	assert.Contains(t, string(manifest), "name: neo4j-ca")

	clusterOnly := strings.Replace(input,
		"    certManager:\n      issuerRef:\n        name: neo4j-ca\n",
		"    certificates:\n      cluster:\n        secretName: cluster-tls\n", 1)
	_, err = PrepareNeo4jManifestFromFile([]byte(clusterOnly), "dev", "default")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cluster mode requires")
}

func TestPrepareNeo4jManifestFromFile_RejectsInvalidInput(t *testing.T) {
	valid := `apiVersion: neo4j.com/v1
kind: Neo4j
spec:
  edition: enterprise
  version: "2026.05.0"
  license: {accept: "eval"}
  topology: {mode: Standalone}
`
	for _, tc := range []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "empty file", input: "", wantErr: "one Neo4j object"},
		{name: "wrong kind", input: strings.Replace(valid, "kind: Neo4j", "kind: Pod", 1), wantErr: "kind Neo4j"},
		{name: "multiple documents", input: valid + "---\n" + valid, wantErr: "exactly one"},
		{name: "missing enterprise licence", input: strings.Replace(valid, "  license: {accept: \"eval\"}\n", "", 1), wantErr: "licence acceptance"},
		{name: "scalar metadata", input: strings.Replace(valid, "kind: Neo4j", "kind: Neo4j\nmetadata: wrong", 1), wantErr: "metadata must be a mapping"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := PrepareNeo4jManifestFromFile([]byte(tc.input), "dev", "default")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestParseKubeQuantity(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  uint64
	}{
		{input: "2Gi", want: 2 << 30},
		{input: "512Mi", want: 512 << 20},
		{input: "1.5G", want: 1_500_000_000},
		{input: "1e3", want: 1000},
		{input: "4000m", want: 4},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseKubeQuantity(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	// Kubernetes has no lowercase g, so a Neo4j-style size is refused here
	// rather than by the API server.
	for _, input := range []string{"", "0", "0e500", "1e3Gi", "-1Gi", "2 GB", "2XB", "8g", "1m"} {
		t.Run("invalid "+input, func(t *testing.T) {
			_, err := parseKubeQuantity(input)
			require.Error(t, err)
		})
	}
}

func TestParseNeo4jSize(t *testing.T) {
	// Neo4j sizes are binary whatever the suffix's case.
	for _, tc := range []struct {
		input string
		want  uint64
	}{
		{input: "2g", want: 2 << 30},
		{input: "2GB", want: 2 << 30},
		{input: "512m", want: 512 << 20},
		{input: "1.5k", want: 1536},
		{input: "1024", want: 1024},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseNeo4jSize(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	for _, input := range []string{"", "0", "2Gi", "2gib", "1e3", "2t"} {
		t.Run("invalid "+input, func(t *testing.T) {
			_, err := parseNeo4jSize(input)
			require.Error(t, err)
		})
	}
}
