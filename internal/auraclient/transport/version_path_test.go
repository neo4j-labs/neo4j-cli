// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package transport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neo4j/cli/internal/auraclient/transport"
)

// TestVersionPathMapping pins each AuraApiVersion to its wire path. It guards
// against a future change silently repointing a version constant (which would
// migrate every command using it), keeping the non-migrated commands
// (instance pause/resume/update/overwrite, snapshot, customer-managed-key on
// v1; graphql on v1beta5) on their intended API version.
func TestVersionPathMapping(t *testing.T) {
	testCases := []struct {
		version  transport.AuraApiVersion
		wantPath string
	}{
		{transport.AuraApiVersion1, "v1"},
		{transport.AuraApiVersionBeta1, "v1beta5"},
		{transport.AuraApiVersion2, "v2beta1"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.version), func(t *testing.T) {
			if got := transport.VersionPathForTest(tc.version); got != tc.wantPath {
				t.Errorf("getVersionPath(%q) = %q, want %q", tc.version, got, tc.wantPath)
			}
		})
	}
}

// TestVersionPathPanicsOnUnknown guards the fail-closed behaviour: an
// unrecognised version must panic rather than silently building a request
// against a wrong or empty path.
func TestVersionPathPanicsOnUnknown(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("getVersionPath did not panic on an unknown version")
		}
	}()
	transport.VersionPathForTest("nonexistent")
}

// TestMakeRequest_DefaultsToV1 guards that a RequestConfig with no Version set
// still issues against the v1 flat path. This is the actual regression vector
// for the non-migrated commands (pause/update/snapshot/CMK omit Version and
// rely on this default), which getVersionPath alone does not cover.
func TestMakeRequest_DefaultsToV1(t *testing.T) {
	var gotPath string

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v1/instances", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`)) //nolint:errcheck
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := buildTestConfig(t, srv.URL, `{
		"aura": {
			"credentials": [{"name":"c","client-id":"id","client-secret":"s","access-token":"tok","token-expiry":9999999999}],
			"default-credential": "c"
		}
	}`)

	_, statusCode, err := transport.MakeRequest(context.Background(), cfg, "instances", &transport.RequestConfig{
		Method: http.MethodGet,
	})
	if err != nil {
		t.Fatalf("MakeRequest returned error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", statusCode)
	}
	if gotPath != "/v1/instances" {
		t.Errorf("request path = %q, want %q", gotPath, "/v1/instances")
	}
}
