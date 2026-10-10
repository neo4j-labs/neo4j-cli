// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// pinDiscoverMiss stubs both mDNS tiers so Discover falls through to the
// legacy port scan (which pinProbeTo then controls).
func pinDiscoverMiss(t *testing.T) {
	t.Helper()
	t.Cleanup(SetMDNSBrowseFnForTest(func(_ context.Context) (int, bool) { return 0, false }))
	t.Cleanup(SetDNSSDLookupFnForTest(func(_ context.Context) (int, bool) { return 0, false }))
}

// stubAppInfo routes the unauthenticated /info/app fetch (via httpDoFn) to a
// handler serving the given AppInfo JSON body with HTTP 200.
func stubAppInfo(t *testing.T, body string) {
	t.Helper()
	t.Cleanup(SetHTTPDoFnForTest(func(_ *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusOK)
		_, _ = rec.WriteString(body)
		return rec.Result(), nil
	}))
}

func TestConnect_Success(t *testing.T) {
	pinDiscoverMiss(t)
	pinProbeTo(t, "127.0.0.1", map[int]bool{ProbePortStart: true})

	fs := afero.NewMemMapFs()
	dataDir := "/data"
	stubAppInfo(t, fmt.Sprintf(`{"dataPath": %q}`, dataDir))
	if err := afero.WriteFile(fs, filepath.Join(dataDir, SaltFilename), []byte("test-salt"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	client, err := Connect(context.Background(), fs, 0)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if client == nil {
		t.Fatal("Connect returned nil client")
	}
	if client.ClientID() == "" {
		t.Fatal("client.ClientID() is empty")
	}
	if client.token == "" {
		t.Fatal("client.token is empty")
	}
}

func TestConnect_ProbeMissIsUnreachable(t *testing.T) {
	pinDiscoverMiss(t)
	pinProbeTo(t, "127.0.0.1", map[int]bool{})

	_, err := Connect(context.Background(), afero.NewMemMapFs(), 0)
	if err == nil {
		t.Fatal("Connect: expected error")
	}
	if !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Fatalf("err = %q, want canonical unreachable message", err.Error())
	}
}

func TestConnect_ProbeErrorNonSentinel(t *testing.T) {
	pinDiscoverMiss(t)
	pinProbeTo(t, "127.0.0.1", map[int]bool{ProbePortStart: true})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Connect(ctx, afero.NewMemMapFs(), 0)
	if err == nil {
		t.Fatal("Connect: expected error")
	}
	if !strings.Contains(err.Error(), "desktop: probe failed: context canceled") {
		t.Fatalf("err = %q, want probe-failed wrapper", err.Error())
	}
}

func TestConnect_ResolveDataDirError(t *testing.T) {
	pinDiscoverMiss(t)
	pinProbeTo(t, "127.0.0.1", map[int]bool{ProbePortStart: true})
	t.Setenv("NEO4J_DESKTOP_DATA_PATH", "")

	// /info/app transport error → ResolveDataDir falls through to the env
	// JSON (absent in memFs) and then the per-OS default, which fails here.
	t.Cleanup(SetHTTPDoFnForTest(func(_ *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("connection refused (test)")
	}))
	t.Cleanup(SetHomeDirFnForTest(func() (string, error) {
		return "", fmt.Errorf("no home (test)")
	}))

	_, err := Connect(context.Background(), afero.NewMemMapFs(), 0)
	if err == nil {
		t.Fatal("Connect: expected error")
	}
	if !strings.Contains(err.Error(), "desktop: could not resolve relate data dir") {
		t.Fatalf("err = %q, want data-dir wrapper", err.Error())
	}
}

func TestConnect_MissingSaltIsUnreachable(t *testing.T) {
	pinDiscoverMiss(t)
	pinProbeTo(t, "127.0.0.1", map[int]bool{ProbePortStart: true})

	fs := afero.NewMemMapFs()
	dataDir := "/data"
	stubAppInfo(t, fmt.Sprintf(`{"dataPath": %q}`, dataDir))
	// No relate.secret.key written — Desktop has not finished first-run auth.

	_, err := Connect(context.Background(), fs, 0)
	if err == nil {
		t.Fatal("Connect: expected error")
	}
	if !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Fatalf("err = %q, want canonical unreachable message", err.Error())
	}
}

func TestConnect_SeamOverride(t *testing.T) {
	sentinel := &Client{clientID: "sentinel"}
	t.Cleanup(SetConnectFnForTest(func(_ context.Context, _ afero.Fs, port int) (*Client, error) {
		if port != 44229 {
			t.Fatalf("seam got port %d, want 44229", port)
		}
		return sentinel, nil
	}))

	client, err := Connect(context.Background(), afero.NewMemMapFs(), 44229)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if client != sentinel {
		t.Fatal("Connect did not route through the seam")
	}
}
