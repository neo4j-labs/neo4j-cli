// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/clievents"
	"github.com/neo4j/cli/internal/testutil/testfs"
)

func TestInstanceCreateBody(t *testing.T) {
	paid := InstanceCreate{
		Name: "n", Version: "5", Region: "europe-west1", Type: "professional", CloudProvider: "aws",
		Memory: "8GB", VectorOptimized: true,
	}

	t.Run("sized tiers send what the caller chose", func(t *testing.T) {
		b := paid.body("proj-1")
		assert.Equal(t, "8GB", b["memory"])
		assert.Equal(t, "europe-west1", b["region"])
		assert.Equal(t, "aws", b["cloud_provider"])
		assert.Equal(t, "5", b["version"])
		assert.Equal(t, "proj-1", b["tenant_id"])
		assert.Equal(t, true, b["vector_optimized"])
		assert.NotContains(t, b, "graph_analytics")
		assert.NotContains(t, b, "customer_managed_key_id")
	})

	t.Run("free ignores sizing flags and uses the free-tier contract", func(t *testing.T) {
		b := InstanceCreate{Name: "n", Type: "free", Version: "4", Region: "mars", CloudProvider: "azure", Memory: "64GB"}.body("p")
		assert.Equal(t, "1GB", b["memory"])
		assert.Equal(t, "europe-west1", b["region"])
		assert.Equal(t, "gcp", b["cloud_provider"])
		assert.Equal(t, "5", b["version"])
		assert.NotContains(t, b, "vector_optimized")
	})

	t.Run("the graph analytics plugin is professional-only and sent only when requested", func(t *testing.T) {
		on := paid
		on.GraphAnalyticsPlugin = true
		assert.Equal(t, "plugin", on.body("p")["graph_analytics"])

		other := on
		other.Type = "business-critical"
		assert.NotContains(t, other.body("p"), "graph_analytics", "silently ignored upstream, so not sent")

		assert.NotContains(t, paid.body("p"), "graph_analytics", "false omits the field and lets the API default apply")
	})

	t.Run("a customer managed key is sent only when set", func(t *testing.T) {
		k := paid
		k.CustomerManagedKeyID = "cmk-1"
		assert.Equal(t, "cmk-1", k.body("p")["customer_managed_key_id"])
	})
}

// instanceAPI is a method-aware fake of the scoped instances collection.
type instanceAPI struct {
	mu        sync.Mutex
	listBody  string
	postReply string
	posted    map[string]any
	calls     []string
}

func (a *instanceAPI) client(t *testing.T) Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v2beta1/organizations/org-1/projects/proj-1/instances", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.calls = append(a.calls, r.Method)
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(a.listBody)) //nolint:errcheck
		case http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &a.posted)
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(a.postReply)) //nolint:errcheck
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfgJSON := fmt.Sprintf(`{"format":"json","aura":{"auth-url":"%s/oauth/token","base-url":"%s"}}`, srv.URL, srv.URL)
	fs, err := testfs.GetTestFs(cfgJSON, testCredJSON)
	require.NoError(t, err)
	return New(clicfg.NewConfig(fs, "test", clicfg.AuraScope))
}

var createScope = Scope{OrgID: "org-1", ProjectID: "proj-1"}

func TestInstancesCreate_RegistersPasswordAndReturnsRecord(t *testing.T) {
	const password = "generated-pw-Zq81-unique"
	api := &instanceAPI{postReply: `{"data":{"id":"abc","name":"mine","tenant_id":"proj-1","username":"neo4j","password":"` + password + `","connection_url":"neo4j+s://x"}}`}

	inst, err := api.client(t).Instances().Create(context.Background(), createScope, InstanceCreate{Name: "mine", Type: "professional", Version: "5", Region: "r", CloudProvider: "gcp", Memory: "4GB"})

	require.NoError(t, err)
	assert.Equal(t, "abc", inst.ID)
	assert.Equal(t, "proj-1", inst.ProjectID, "tenant_id is exposed as project_id")
	assert.Equal(t, password, inst.Record["password"], "the one-time password stays in the record for the caller")
	assert.NotContains(t, clievents.RedactText("output: "+password), password, "and is registered for redaction as soon as it is received")
	assert.Equal(t, []string{http.MethodPost}, api.calls, "an explicit name needs no list call")
	assert.Equal(t, "mine", api.posted["name"])
	assert.Equal(t, "proj-1", api.posted["tenant_id"])
}

func TestInstancesCreate_DefaultsTheNameFromExistingInstances(t *testing.T) {
	api := &instanceAPI{
		listBody:  `{"data":[{"id":"1","name":"Instance01"},{"id":"2","name":"instance02"},{"id":"3","name":"other"}]}`,
		postReply: `{"data":{"id":"abc","name":"Instance03"}}`,
	}

	_, err := api.client(t).Instances().Create(context.Background(), createScope, InstanceCreate{Type: "free", Version: "5"})

	require.NoError(t, err)
	assert.Equal(t, []string{http.MethodGet, http.MethodPost}, api.calls)
	assert.Equal(t, "Instance03", api.posted["name"], "lowest unused InstanceNN, case-insensitively")
}

func TestInstancesCreate_ValidatesScopeBeforeAnyRequest(t *testing.T) {
	_, err := instanceService{}.Create(context.Background(), Scope{OrgID: "a/b", ProjectID: "p"}, InstanceCreate{Name: "n"})
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	_, err = instanceService{}.Create(context.Background(), Scope{OrgID: "o", ProjectID: ".."}, InstanceCreate{Name: "n"})
	require.True(t, errors.As(err, &ce))
}

func TestInstancesOverwrite(t *testing.T) {
	var (
		mu      sync.Mutex
		posted  map[string]any
		methods []string
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v1/instances/inst-1", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, "preflight "+r.Method)
		mu.Unlock()
		w.Write([]byte(`{"data":{"id":"inst-1","tenant_id":"proj-1"}}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v1/instances/inst-1/overwrite", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		methods = append(methods, "overwrite "+r.Method)
		_ = json.Unmarshal(raw, &posted)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"data":{"id":"inst-1","tenant_id":"proj-1","status":"overwriting"}}`)) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cfgJSON := fmt.Sprintf(`{"format":"json","aura":{"auth-url":"%s/oauth/token","base-url":"%s"}}`, srv.URL, srv.URL)
	fs, err := testfs.GetTestFs(cfgJSON, testCredJSON)
	require.NoError(t, err)
	instances := New(clicfg.NewConfig(fs, "test", clicfg.AuraScope)).Instances()
	scope := Scope{OrgID: "org-1", ProjectID: "proj-1"}

	t.Run("defaults the source to the instance itself and verifies ownership first", func(t *testing.T) {
		posted, methods = nil, nil
		inst, err := instances.Overwrite(context.Background(), scope, "inst-1", OverwriteSource{SnapshotID: "snap-1"})

		require.NoError(t, err)
		assert.Equal(t, "overwriting", inst.Status)
		assert.Equal(t, "proj-1", inst.ProjectID)
		assert.Equal(t, map[string]any{"source_instance_id": "inst-1", "source_snapshot_id": "snap-1"}, posted)
		assert.Equal(t, []string{"preflight GET", "overwrite POST"}, methods)
	})

	t.Run("a hostile source id is rejected and nothing is overwritten", func(t *testing.T) {
		posted, methods = nil, nil
		for _, src := range []OverwriteSource{{InstanceID: "../x"}, {SnapshotID: "a/b"}} {
			_, err := instances.Overwrite(context.Background(), scope, "inst-1", src)
			var ce *clierr.CLIError
			assert.True(t, errors.As(err, &ce), "%+v", src)
		}
		assert.NotContains(t, methods, "overwrite POST")
	})
}
