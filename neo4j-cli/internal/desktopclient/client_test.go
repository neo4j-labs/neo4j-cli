// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/neo4j/cli/common/clierr"
)

// newAuthedServer spins up an httptest.Server that wires the supplied handler
// after verifying the X-Client-Id + X-API-Token headers using the documented
// composite key. The handler runs only when auth checks pass; otherwise the
// test fails. Returns the server + the clientID/salt used so individual tests
// can derive their expected request shapes.
func newAuthedServer(t *testing.T, salt, clientID string, handler http.HandlerFunc) (*httptest.Server, ProbeResult) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID := r.Header.Get(HeaderClientID)
		gotTok := r.Header.Get(HeaderAPIToken)
		if gotID != clientID {
			t.Errorf("X-Client-Id = %q, want %q", gotID, clientID)
			http.Error(w, "bad client id", http.StatusUnauthorized)
			return
		}
		// Verify the JWT against the documented signing key.
		key := fmt.Sprintf("%s-%s-%s", salt, srv.URL, clientID)
		_, err := jwt.Parse(gotTok, func(_ *jwt.Token) (any, error) { return []byte(key), nil })
		if err != nil {
			t.Errorf("X-API-Token did not verify with composite key: %v", err)
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, ProbeResult{Origin: srv.URL}
}

// pinClientSeams pins the package-level uuid + clock seams so tests can reason
// about both the token payload and the X-Client-Id header value.
func pinClientSeams(t *testing.T, clientID string, now time.Time) {
	t.Helper()
	t.Cleanup(SetUUIDFnForTest(func() string { return clientID }))
	t.Cleanup(SetNowFnForTest(func() time.Time { return now }))
}

func TestNewClient_TokenSignedWithCompositeKey(t *testing.T) {
	const salt = "11111111-2222-3333-4444-555555555555"
	const clientID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	probe := ProbeResult{Port: 44222, Origin: "http://localhost:44222"}
	cl, err := NewClient(probe, salt)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if cl.ClientID() != clientID {
		t.Fatalf("ClientID() = %q, want %q", cl.ClientID(), clientID)
	}

	// Verify the token parses with the documented composite key.
	expectedKey := fmt.Sprintf("%s-%s-%s", salt, probe.Origin, clientID)
	parsed, err := jwt.Parse(cl.token, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Method)
		}
		return []byte(expectedKey), nil
	})
	if err != nil {
		t.Fatalf("token did not verify with composite key: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("claims are not MapClaims: %T", parsed.Claims)
	}
	iat, _ := claims["iat"].(float64)
	exp, _ := claims["exp"].(float64)
	if iat == 0 || exp == 0 {
		t.Fatalf("missing iat/exp claims: %+v", claims)
	}
	if int64(exp-iat) != int64((7 * 24 * time.Hour).Seconds()) {
		t.Fatalf("token lifetime = %ds, want %ds", int64(exp-iat), int64((7 * 24 * time.Hour).Seconds()))
	}
}

func TestNewClient_TokenWrongKeyDoesNotVerify(t *testing.T) {
	const salt = "salt"
	const clientID = "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	cl, err := NewClient(ProbeResult{Origin: "http://localhost:44222"}, salt)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = jwt.Parse(cl.token, func(_ *jwt.Token) (any, error) { return []byte("wrong-key"), nil })
	if err == nil {
		t.Fatalf("expected verification to fail with wrong key")
	}
}

func TestClient_ListDbmss_SendsAuthHeadersAndDecodes(t *testing.T) {
	const salt = "salt"
	const clientID = "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/fastify/api/dbmss" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"a","name":"A","status":"online","connectionUri":"neo4j://localhost:7687"}]`))
	})
	_ = srv

	cl, err := NewClient(probe, salt)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	got, err := cl.ListDbmss(context.Background())
	if err != nil {
		t.Fatalf("ListDbmss: %v", err)
	}
	if len(got) != 1 || got[0].ID != "a" || got[0].Status != "online" {
		t.Fatalf("unexpected list: %+v", got)
	}
}

func TestClient_ListDbmssInfo_HitsInfoEndpointAndDecodes(t *testing.T) {
	const salt = "salt"
	const clientID = "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/fastify/api/dbmss/info" {
			t.Errorf("got %s %s, want GET /fastify/api/dbmss/info", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"a","name":"A","status":"started","connectionUri":"neo4j://localhost:7687"}]`))
	})
	_ = srv

	cl, err := NewClient(probe, salt)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	got, err := cl.ListDbmssInfo(context.Background())
	if err != nil {
		t.Fatalf("ListDbmssInfo: %v", err)
	}
	if len(got) != 1 || got[0].ID != "a" || got[0].Status != "started" {
		t.Fatalf("unexpected list: %+v", got)
	}
}

func TestClient_GetDbms_RoundTrips(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fastify/api/dbmss/abc" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"abc","name":"X","status":"offline"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.GetDbms(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetDbms: %v", err)
	}
	if got.ID != "abc" || got.Status != "offline" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestClient_CreateDbms_OmitsEdition(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var seenBody map[string]any
	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/fastify/api/desktop/dbmss" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"new","name":"X","version":"5.20.0","status":"created"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	out, err := cl.CreateDbms(context.Background(), CreateDbmsRequest{
		Name:     "X",
		Version:  "5.20.0",
		Password: "neo4j-secret",
	})
	if err != nil {
		t.Fatalf("CreateDbms: %v", err)
	}
	if out.ID != "new" {
		t.Fatalf("returned id = %q", out.ID)
	}
	// REQ-F-002 / impl.md: body MUST NOT contain `edition`.
	if _, present := seenBody["edition"]; present {
		t.Fatalf("CreateDbms body included 'edition': %+v", seenBody)
	}
	// Required fields are present.
	wantFields := []string{"name", "version", "credentials"}
	for _, f := range wantFields {
		if _, ok := seenBody[f]; !ok {
			t.Errorf("CreateDbms body missing %q field: %+v", f, seenBody)
		}
	}
	if seenBody["credentials"] != "neo4j-secret" {
		t.Errorf("credentials = %v, want %q", seenBody["credentials"], "neo4j-secret")
	}
}

func TestClient_DeleteDbms(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/fastify/api/dbmss/zzz" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"zzz","name":"Z","status":"deleted"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.DeleteDbms(context.Background(), "zzz")
	if err != nil {
		t.Fatalf("DeleteDbms: %v", err)
	}
	if got.ID != "zzz" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestClient_StartStopDbms(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var lastPath string
	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.Method + " " + r.URL.Path
		_, _ = w.Write([]byte(`"started"`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	if err := cl.StartDbms(context.Background(), "id-1"); err != nil {
		t.Fatalf("StartDbms: %v", err)
	}
	if lastPath != "POST /fastify/api/dbmss/id-1/start" {
		t.Fatalf("StartDbms path = %q", lastPath)
	}
	if err := cl.StopDbms(context.Background(), "id-2"); err != nil {
		t.Fatalf("StopDbms: %v", err)
	}
	if lastPath != "POST /fastify/api/desktop/dbmss/id-2/stop" {
		t.Fatalf("StopDbms path = %q", lastPath)
	}
}

func TestClient_401_MapsToAuthErrorWithRestartHint(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	// Bypass the auth-checking helper — we want to assert the 401 mapping
	// regardless of header content.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.ListDbmss(context.Background())
	if err == nil {
		t.Fatalf("expected error on 401")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T, want *clierr.CLIError", err)
	}
	if ce.Code != 4 {
		t.Fatalf("exit code = %d, want 4 (auth)", ce.Code)
	}
	if !strings.Contains(ce.Message, "restart Neo4j Desktop 2") {
		t.Fatalf("message missing restart hint: %q", ce.Message)
	}
}

func TestClient_5xx_MapsToFatalWithTruncatedBody(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	bigBody := strings.Repeat("A", 500)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(bigBody))
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.ListDbmss(context.Background())
	if err == nil {
		t.Fatalf("expected error on 500")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T", err)
	}
	if ce.Code != 1 {
		t.Fatalf("exit code = %d, want 1 (fatal)", ce.Code)
	}
	if !strings.Contains(ce.Message, "returned 500") {
		t.Fatalf("message missing status: %q", ce.Message)
	}
	// Truncation: the body cap is 200 — full 500-byte body must NOT appear.
	if strings.Contains(ce.Message, bigBody) {
		t.Fatalf("message contains untruncated body")
	}
	// And the truncation marker should be in there.
	if !strings.Contains(ce.Message, "…") {
		t.Fatalf("message missing truncation marker: %q", ce.Message)
	}
}

func TestClient_ConnectionRefused_MapsToCanonicalUnreachable(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	// Spin up + immediately close a server so the port is unbound.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	dead := srv.URL
	srv.Close()

	cl, _ := NewClient(ProbeResult{Origin: dead}, salt)
	_, err := cl.ListDbmss(context.Background())
	if err == nil {
		t.Fatalf("expected error for dead server")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T", err)
	}
	if !strings.Contains(ce.Message, "doesn't appear to be running") {
		t.Fatalf("missing canonical message: %q", ce.Message)
	}
	if !strings.Contains(ce.Message, "--port") {
		t.Fatalf("missing --port hint: %q", ce.Message)
	}
}

func TestClient_RequestTimeout_MapsToCanonicalTimeout(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	// Use a custom httpDoFn that simulates a request that blocks past the
	// context deadline. We drive the timeout from outside by giving the
	// context a 5ms deadline so the test stays fast — the real
	// requestTimeout codepath is exercised by the same `errors.Is` check.
	restoreDo := SetHTTPDoFnForTest(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	t.Cleanup(restoreDo)

	cl, _ := NewClient(ProbeResult{Origin: "http://example.invalid"}, salt)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, err := cl.ListDbmss(ctx)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T", err)
	}
	if !strings.Contains(ce.Message, "did not respond within") {
		t.Fatalf("timeout did not map to canonicalTimeout: %q", ce.Message)
	}
	if strings.Contains(ce.Message, "doesn't appear to be running") {
		t.Fatalf("deadline-exceeded must NOT collapse to canonicalUnreachable: %q", ce.Message)
	}
}

func TestUnreachableError_MatchesCanonicalMessage(t *testing.T) {
	err := UnreachableError()
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T, want *clierr.CLIError", err)
	}
	if ce.Code != 1 {
		t.Fatalf("exit code = %d, want 1 (fatal)", ce.Code)
	}
	// Same text as the in-flight unreachable mappings (REQ-F-008 requires
	// these all converge on one message).
	if !strings.Contains(ce.Message, "doesn't appear to be running") {
		t.Fatalf("missing canonical text: %q", ce.Message)
	}
	if !strings.Contains(ce.Message, "--port") {
		t.Fatalf("missing --port hint: %q", ce.Message)
	}
	// REQ-F-008 / task-005: canonicalUnreachable points users at the new
	// `desktop doctor` leaf so a failed leaf call surfaces the scan hint
	// alongside the start-Desktop and --port suggestions.
	if !strings.Contains(ce.Message, "or run 'neo4j-cli desktop doctor' to scan.") {
		t.Fatalf("missing doctor hint: %q", ce.Message)
	}
}

func TestClient_GetCredentialsByKey_PopulatedBody_Dbms(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/fastify/api/credentials/dbms:abc" {
			t.Errorf("got %s %s, want GET /fastify/api/credentials/dbms:abc", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"username":"neo4j","password":"secret-pw"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.GetCredentialsByKey(context.Background(), "dbms:abc")
	if err != nil {
		t.Fatalf("GetCredentialsByKey: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil *Credentials")
	}
	if got.Username != "neo4j" || got.Password != "secret-pw" {
		t.Fatalf("unexpected creds: %+v", got)
	}
}

func TestClient_GetCredentialsByKey_PopulatedBody_Connection(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		const want = "/fastify/api/credentials/connection:11111111-2222-3333-4444-555555555555"
		if r.Method != http.MethodGet || r.URL.Path != want {
			t.Errorf("got %s %s, want GET %s", r.Method, r.URL.Path, want)
		}
		_, _ = w.Write([]byte(`{"username":"neo4j","password":"aura-pw"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.GetCredentialsByKey(context.Background(), "connection:11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatalf("GetCredentialsByKey: %v", err)
	}
	if got == nil || got.Username != "neo4j" || got.Password != "aura-pw" {
		t.Fatalf("unexpected creds: %+v", got)
	}
}

func TestClient_GetCredentialsByKey_NullBody_ReturnsNilNil(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
		// Whitespace around the literal must still parse as null — Desktop's
		// JSON encoder may add a trailing newline.
		_, _ = w.Write([]byte("null\n"))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.GetCredentialsByKey(context.Background(), "dbms:legacy-dbms")
	if err != nil {
		t.Fatalf("GetCredentialsByKey should not error on null body, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil *Credentials for null body, got %+v", got)
	}
}

func TestClient_GetCredentialsByKey_404_RoutesThroughErrorMapping(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"dbms not found"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.GetCredentialsByKey(context.Background(), "dbms:missing")
	if err == nil {
		t.Fatalf("expected error on 404")
	}
	if got != nil {
		t.Fatalf("expected nil result on error, got %+v", got)
	}
	if !strings.Contains(err.Error(), "dbms not found") {
		t.Fatalf("error missing body text: %q", err.Error())
	}
}

func TestClient_GetCredentialsByKey_401_MapsToAuthError(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.GetCredentialsByKey(context.Background(), "dbms:abc")
	if err == nil {
		t.Fatalf("expected error on 401")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T, want *clierr.CLIError", err)
	}
	if ce.Code != 4 {
		t.Fatalf("exit code = %d, want 4 (auth)", ce.Code)
	}
	if !strings.Contains(ce.Message, "restart Neo4j Desktop 2") {
		t.Fatalf("message missing restart hint: %q", ce.Message)
	}
}

func TestClient_GetCredentialsByKey_5xx_MapsToFatalWithTruncatedBody(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	bigBody := strings.Repeat("B", 500)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(bigBody))
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.GetCredentialsByKey(context.Background(), "dbms:abc")
	if err == nil {
		t.Fatalf("expected error on 500")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T", err)
	}
	if ce.Code != 1 {
		t.Fatalf("exit code = %d, want 1 (fatal)", ce.Code)
	}
	if !strings.Contains(ce.Message, "returned 500") {
		t.Fatalf("message missing status: %q", ce.Message)
	}
	if strings.Contains(ce.Message, bigBody) {
		t.Fatalf("message contains untruncated body")
	}
}

func TestClient_ListDbmsVersions_DecodesPopulated(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/fastify/api/dbmss/versions" {
			t.Errorf("got %s %s, want GET /fastify/api/dbmss/versions", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{"dist":"/cache/neo4j-enterprise-2026.04.0","edition":"enterprise","origin":"cached","version":"2026.04.0"},
			{"dist":"https://dist.neo4j.org/neo4j-enterprise-5.26.1-unix.tar.gz","edition":"enterprise","origin":"online","version":"5.26.1"}
		]`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.ListDbmsVersions(context.Background())
	if err != nil {
		t.Fatalf("ListDbmsVersions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Version != "2026.04.0" || got[0].Origin != "cached" || got[0].Edition != "enterprise" {
		t.Fatalf("entry 0 = %+v", got[0])
	}
	if got[1].Version != "5.26.1" || got[1].Origin != "online" {
		t.Fatalf("entry 1 = %+v", got[1])
	}
}

func TestClient_ListDbmsVersions_EmptyResponse(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.ListDbmsVersions(context.Background())
	if err != nil {
		t.Fatalf("ListDbmsVersions: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(got))
	}
}

func TestClient_ListDbmsVersions_401_MapsToAuthError(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.ListDbmsVersions(context.Background())
	if err == nil {
		t.Fatalf("expected error on 401")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T", err)
	}
	if ce.Code != 4 {
		t.Fatalf("exit code = %d, want 4 (auth)", ce.Code)
	}
}

func TestClient_ListDbmsVersions_5xx_MapsToFatal(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.ListDbmsVersions(context.Background())
	if err == nil {
		t.Fatalf("expected error on 500")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T", err)
	}
	if ce.Code != 1 {
		t.Fatalf("exit code = %d, want 1 (fatal)", ce.Code)
	}
	if !strings.Contains(ce.Message, "returned 500") {
		t.Fatalf("message missing status: %q", ce.Message)
	}
}

func TestClient_4xxOther_SurfacesBody(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"version not supported"}`))
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.GetDbms(context.Background(), "z")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "version not supported") {
		t.Fatalf("error missing body text: %q", err.Error())
	}
}

// ---- Connection endpoints ----

func TestClient_ListConnections_SendsAuthHeadersAndDecodes(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/fastify/api/connections" {
			t.Errorf("got %s %s, want GET /fastify/api/connections", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"11111111-2222-3333-4444-555555555555","name":"Aura prod","connectionUri":"neo4j+s://abc.databases.neo4j.io","description":"prod"},
			{"id":"66666666-7777-8888-9999-aaaaaaaaaaaa","name":"Local mirror","connectionUri":"neo4j://localhost:7687"}
		]`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.ListConnections(context.Background())
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "11111111-2222-3333-4444-555555555555" || got[0].Name != "Aura prod" {
		t.Fatalf("entry 0 = %+v", got[0])
	}
	if got[0].ConnectionURI != "neo4j+s://abc.databases.neo4j.io" || got[0].Description != "prod" {
		t.Fatalf("entry 0 details = %+v", got[0])
	}
	if got[1].ID != "66666666-7777-8888-9999-aaaaaaaaaaaa" {
		t.Fatalf("entry 1 = %+v", got[1])
	}
}

func TestClient_ListConnections_EmptyResponse(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.ListConnections(context.Background())
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(got))
	}
}

func TestClient_ListConnections_401_MapsToAuthError(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.ListConnections(context.Background())
	if err == nil {
		t.Fatalf("expected error on 401")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != 4 {
		t.Fatalf("error = %v (%T)", err, err)
	}
}

func TestClient_ListConnections_5xx_MapsToFatal(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.ListConnections(context.Background())
	if err == nil {
		t.Fatalf("expected error on 500")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != 1 {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(ce.Message, "returned 500") {
		t.Fatalf("message missing status: %q", ce.Message)
	}
}

func TestClient_CreateConnection_SendsAllRequiredFields(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var seenBody map[string]any
	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/fastify/api/connections" {
			t.Errorf("got %s %s, want POST /fastify/api/connections", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"new-id","name":"Aura prod","connectionUri":"neo4j+s://abc.databases.neo4j.io"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	out, err := cl.CreateConnection(context.Background(), ConnectionCreateArgs{
		Name:          "Aura prod",
		ConnectionURI: "neo4j+s://abc.databases.neo4j.io",
		Username:      "neo4j",
		Password:      "aura-pw",
		Description:   "prod env",
	})
	if err != nil {
		t.Fatalf("CreateConnection: %v", err)
	}
	if out.ID != "new-id" {
		t.Fatalf("returned id = %q", out.ID)
	}
	wantFields := map[string]any{
		"name":          "Aura prod",
		"connectionUri": "neo4j+s://abc.databases.neo4j.io",
		"username":      "neo4j",
		"password":      "aura-pw",
		"description":   "prod env",
	}
	for k, v := range wantFields {
		if seenBody[k] != v {
			t.Errorf("body[%q] = %v, want %v", k, seenBody[k], v)
		}
	}
}

func TestClient_CreateConnection_OmitsEmptyDescription(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var seenBody map[string]any
	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"x","name":"X","connectionUri":"neo4j://localhost:7687"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	_, err := cl.CreateConnection(context.Background(), ConnectionCreateArgs{
		Name:          "X",
		ConnectionURI: "neo4j://localhost:7687",
		Username:      "neo4j",
		Password:      "pw",
	})
	if err != nil {
		t.Fatalf("CreateConnection: %v", err)
	}
	if _, present := seenBody["description"]; present {
		t.Fatalf("body should NOT include description when empty: %+v", seenBody)
	}
}

func TestClient_CreateConnection_400_SurfacesBody(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"connection with name 'X' already exists"}`))
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.CreateConnection(context.Background(), ConnectionCreateArgs{
		Name: "X", ConnectionURI: "neo4j://localhost:7687", Username: "neo4j", Password: "pw",
	})
	if err == nil {
		t.Fatalf("expected error on 400")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error missing body text: %q", err.Error())
	}
}

func TestClient_UpdateConnection_OnlySendsSuppliedFields(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var seenBody map[string]any
	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		const wantPath = "/fastify/api/connections/abc-123"
		if r.Method != http.MethodPatch || r.URL.Path != wantPath {
			t.Errorf("got %s %s, want PATCH %s", r.Method, r.URL.Path, wantPath)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"abc-123","name":"renamed","connectionUri":"neo4j://localhost:7687"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	newName := "renamed"
	out, err := cl.UpdateConnection(context.Background(), "abc-123", ConnectionUpdateArgs{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateConnection: %v", err)
	}
	if out.ID != "abc-123" || out.Name != "renamed" {
		t.Fatalf("unexpected: %+v", out)
	}
	if seenBody["name"] != "renamed" {
		t.Errorf("body[name] = %v, want renamed", seenBody["name"])
	}
	// All other keys MUST be absent from the body.
	for _, k := range []string{"connectionUri", "username", "password", "description"} {
		if _, present := seenBody[k]; present {
			t.Errorf("body unexpectedly contains %q: %+v", k, seenBody)
		}
	}
}

func TestClient_UpdateConnection_PreservesEmptyStringDescription(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var seenBody map[string]any
	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"abc","name":"X","connectionUri":"neo4j://localhost:7687"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	empty := ""
	_, err := cl.UpdateConnection(context.Background(), "abc", ConnectionUpdateArgs{
		Description: &empty,
	})
	if err != nil {
		t.Fatalf("UpdateConnection: %v", err)
	}
	val, present := seenBody["description"]
	if !present {
		t.Fatalf("body missing description key for empty-string update: %+v", seenBody)
	}
	if val != "" {
		t.Fatalf("body[description] = %v, want empty string", val)
	}
	// And only description is present.
	if len(seenBody) != 1 {
		t.Fatalf("body should contain exactly one key, got %+v", seenBody)
	}
}

func TestClient_UpdateConnection_AllFields(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var seenBody map[string]any
	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"abc","name":"new","connectionUri":"neo4j+s://x.databases.neo4j.io"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	name, uri, user, pw, desc := "new", "neo4j+s://x.databases.neo4j.io", "neo4j", "new-pw", "new desc"
	_, err := cl.UpdateConnection(context.Background(), "abc", ConnectionUpdateArgs{
		Name: &name, ConnectionURI: &uri, Username: &user, Password: &pw, Description: &desc,
	})
	if err != nil {
		t.Fatalf("UpdateConnection: %v", err)
	}
	for k, v := range map[string]any{
		"name":          name,
		"connectionUri": uri,
		"username":      user,
		"password":      pw,
		"description":   desc,
	} {
		if seenBody[k] != v {
			t.Errorf("body[%q] = %v, want %v", k, seenBody[k], v)
		}
	}
}

func TestClient_DeleteConnection(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		const wantPath = "/fastify/api/connections/zzz"
		if r.Method != http.MethodDelete || r.URL.Path != wantPath {
			t.Errorf("got %s %s, want DELETE %s", r.Method, r.URL.Path, wantPath)
		}
		_, _ = w.Write([]byte(`{"id":"zzz","name":"gone","connectionUri":"neo4j://localhost:7687"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.DeleteConnection(context.Background(), "zzz")
	if err != nil {
		t.Fatalf("DeleteConnection: %v", err)
	}
	if got.ID != "zzz" || got.Name != "gone" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestClient_DeleteConnection_404(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"connection not found"}`))
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.DeleteConnection(context.Background(), "missing")
	if err == nil {
		t.Fatalf("expected error on 404")
	}
	if !strings.Contains(err.Error(), "connection not found") {
		t.Fatalf("error missing body: %q", err.Error())
	}
}

// ---- Plugin endpoints ----

func TestClient_ListInstalledPlugins_DecodesPopulated(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		const want = "/fastify/api/dbmss/abc-1/plugins/installed"
		if r.Method != http.MethodGet || r.URL.Path != want {
			t.Errorf("got %s %s, want GET %s", r.Method, r.URL.Path, want)
		}
		_, _ = w.Write([]byte(`[
			{"name":"apoc","version":"5.20.0","filePath":"/data/plugins/apoc.jar","pendingRestart":false},
			{"name":"gds","filePath":"/data/plugins/gds.jar","pendingRestart":true}
		]`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.ListInstalledPlugins(context.Background(), "abc-1")
	if err != nil {
		t.Fatalf("ListInstalledPlugins: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Name != "apoc" || got[0].Version != "5.20.0" || got[0].FilePath != "/data/plugins/apoc.jar" || got[0].PendingRestart {
		t.Fatalf("entry 0 = %+v", got[0])
	}
	if got[1].Name != "gds" || got[1].Version != "" || !got[1].PendingRestart {
		t.Fatalf("entry 1 = %+v", got[1])
	}
}

func TestClient_ListInstalledPlugins_EmptyReturnsNonNilSlice(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.ListInstalledPlugins(context.Background(), "abc-1")
	if err != nil {
		t.Fatalf("ListInstalledPlugins: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil slice, want non-nil empty slice (JSON renderers depend on `[]` vs `null`)")
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestClient_ListAvailablePlugins_DecodesPopulated(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		const want = "/fastify/api/dbmss/abc-1/plugins/available"
		if r.Method != http.MethodGet || r.URL.Path != want {
			t.Errorf("got %s %s, want GET %s", r.Method, r.URL.Path, want)
		}
		_, _ = w.Write([]byte(`[
			{"name":"apoc","version":"5.20.0","filePath":"/products/apoc.jar","pendingRestart":false},
			{"name":"neo-semantics","filePath":"/labs/n10s.jar","pendingRestart":false}
		]`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.ListAvailablePlugins(context.Background(), "abc-1")
	if err != nil {
		t.Fatalf("ListAvailablePlugins: %v", err)
	}
	if len(got) != 2 || got[0].Name != "apoc" || got[1].Name != "neo-semantics" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestClient_ListAvailablePlugins_EmptyReturnsNonNilSlice(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	got, err := cl.ListAvailablePlugins(context.Background(), "abc-1")
	if err != nil {
		t.Fatalf("ListAvailablePlugins: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil slice, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestClient_InstallPlugin_SendsBodyAndDecodesResponse(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var seenBody map[string]any
	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		const want = "/fastify/api/dbmss/abc-1/plugins/install"
		if r.Method != http.MethodPost || r.URL.Path != want {
			t.Errorf("got %s %s, want POST %s", r.Method, r.URL.Path, want)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"name":"apoc","version":"5.20.0","filePath":"/data/plugins/apoc.jar","pendingRestart":true}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	out, err := cl.InstallPlugin(context.Background(), "abc-1", "apoc")
	if err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}
	if seenBody["pluginName"] != "apoc" {
		t.Fatalf("body[pluginName] = %v, want apoc", seenBody["pluginName"])
	}
	if out == nil || out.Name != "apoc" || out.Version != "5.20.0" || !out.PendingRestart {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestClient_InstallPlugin_ForwardsPathVerbatim(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var seenBody map[string]any
	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"name":"local","filePath":"/Users/me/local.jar","pendingRestart":true}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	const localPath = "/Users/me/local.jar"
	_, err := cl.InstallPlugin(context.Background(), "abc-1", localPath)
	if err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}
	// Relate dispatches name-vs-path server-side (REQ-F-037) — client MUST
	// pass the value through unchanged.
	if seenBody["pluginName"] != localPath {
		t.Fatalf("body[pluginName] = %v, want %q", seenBody["pluginName"], localPath)
	}
}

// captureDeadlineSeam swaps httpDoFn so the test observes the per-request
// client-side context deadline directly. The server's request context is a
// fresh one created by `httptest.Server`, so we can't read the client-side
// `context.WithTimeout` budget from the handler — we must intercept the
// outbound request before it leaves the client. Returns the captured budget
// after the test invokes the endpoint.
func captureDeadlineSeam(t *testing.T, status int, body string) *time.Duration {
	t.Helper()
	var observed time.Duration
	t.Cleanup(SetHTTPDoFnForTest(func(req *http.Request) (*http.Response, error) {
		if dl, ok := req.Context().Deadline(); ok {
			observed = time.Until(dl)
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{},
		}, nil
	}))
	return &observed
}

func TestClient_InstallPlugin_UsesExtendedTimeout(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	// Inspect the client-side context deadline applied by `doRaw` — we
	// don't actually sleep. The deadline budget must reflect
	// `pluginWriteTimeout` (120s), not the default 30s.
	budget := captureDeadlineSeam(t, http.StatusOK, `{"name":"apoc","filePath":"/p/apoc.jar","pendingRestart":true}`)

	cl, _ := NewClient(ProbeResult{Origin: "http://example.invalid"}, salt)
	if _, err := cl.InstallPlugin(context.Background(), "abc-1", "apoc"); err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}
	// pluginWriteTimeout is 120s; allow generous slack for capture latency.
	// The default `requestTimeout` (30s) must NOT match here.
	if *budget <= 60*time.Second {
		t.Fatalf("install deadline budget = %s, want > 60s (extended 120s window)", *budget)
	}
	if *budget > 120*time.Second {
		t.Fatalf("install deadline budget = %s, must not exceed pluginWriteTimeout 120s", *budget)
	}
}

func TestClient_ListInstalledPlugins_UsesDefaultTimeout(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	budget := captureDeadlineSeam(t, http.StatusOK, `[]`)

	cl, _ := NewClient(ProbeResult{Origin: "http://example.invalid"}, salt)
	if _, err := cl.ListInstalledPlugins(context.Background(), "abc-1"); err != nil {
		t.Fatalf("ListInstalledPlugins: %v", err)
	}
	// Reads stay on the default requestTimeout — must NOT pick up the
	// extended pluginWriteTimeout.
	if *budget > 91*time.Second {
		t.Fatalf("list deadline budget = %s, want ≤ 90s (default requestTimeout)", *budget)
	}
}

func TestClient_UninstallPlugin_SendsBodyAndReturnsName(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var seenBody map[string]any
	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		const want = "/fastify/api/dbmss/abc-1/plugins/uninstall"
		if r.Method != http.MethodPost || r.URL.Path != want {
			t.Errorf("got %s %s, want POST %s", r.Method, r.URL.Path, want)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"name":"apoc"}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	gotName, err := cl.UninstallPlugin(context.Background(), "abc-1", "apoc")
	if err != nil {
		t.Fatalf("UninstallPlugin: %v", err)
	}
	if seenBody["pluginName"] != "apoc" {
		t.Fatalf("body[pluginName] = %v, want apoc", seenBody["pluginName"])
	}
	if gotName != "apoc" {
		t.Fatalf("returned name = %q, want apoc", gotName)
	}
}

func TestClient_UninstallPlugin_UsesExtendedTimeout(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	budget := captureDeadlineSeam(t, http.StatusOK, `{"name":"apoc"}`)

	cl, _ := NewClient(ProbeResult{Origin: "http://example.invalid"}, salt)
	if _, err := cl.UninstallPlugin(context.Background(), "abc-1", "apoc"); err != nil {
		t.Fatalf("UninstallPlugin: %v", err)
	}
	if *budget <= 60*time.Second {
		t.Fatalf("uninstall deadline budget = %s, want > 60s (extended 120s window)", *budget)
	}
	if *budget > 120*time.Second {
		t.Fatalf("uninstall deadline budget = %s, must not exceed pluginWriteTimeout 120s", *budget)
	}
}

func TestClient_PluginEndpoints_404_DbmsMessage_MapsToErrDbmsNotFound(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Could not find DBMS \"ghost\""}`))
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	cases := []struct {
		name string
		call func() error
	}{
		{"ListInstalledPlugins", func() error { _, e := cl.ListInstalledPlugins(context.Background(), "ghost"); return e }},
		{"ListAvailablePlugins", func() error { _, e := cl.ListAvailablePlugins(context.Background(), "ghost"); return e }},
		{"InstallPlugin", func() error { _, e := cl.InstallPlugin(context.Background(), "ghost", "apoc"); return e }},
		{"UninstallPlugin", func() error { _, e := cl.UninstallPlugin(context.Background(), "ghost", "apoc"); return e }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatalf("expected error")
			}
			if !errors.Is(err, ErrDbmsNotFound) {
				t.Fatalf("error = %v, want ErrDbmsNotFound", err)
			}
		})
	}
}

func TestClient_PluginEndpoints_404_PluginMessage_MapsToErrPluginNotFound(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Could not find plugin \"not-a-plugin\""}`))
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.InstallPlugin(context.Background(), "abc-1", "not-a-plugin")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("error = %v, want ErrPluginNotFound", err)
	}
	if errors.Is(err, ErrDbmsNotFound) {
		t.Fatalf("error matched ErrDbmsNotFound too — disambiguation broke")
	}
}

func TestClient_PluginEndpoints_404_AmbiguousBody_DefaultsToPluginNotFound(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	// Empty / plain-text 404 body — neither "dbms" nor "plugin" appears.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.InstallPlugin(context.Background(), "abc-1", "apoc")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrPluginNotFound) {
		t.Fatalf("ambiguous 404 should default to ErrPluginNotFound, got %v", err)
	}
}

func TestClient_PluginEndpoints_401_MapsToAuthError(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.ListInstalledPlugins(context.Background(), "abc-1")
	if err == nil {
		t.Fatalf("expected error on 401")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != 4 {
		t.Fatalf("error = %v (%T)", err, err)
	}
	if !strings.Contains(ce.Message, "restart Neo4j Desktop 2") {
		t.Fatalf("message missing restart hint: %q", ce.Message)
	}
}

func TestClient_PluginEndpoints_5xx_MapsToFatalWithTruncatedBody(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	bigBody := strings.Repeat("Z", 500)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(bigBody))
	}))
	t.Cleanup(srv.Close)

	cl, _ := NewClient(ProbeResult{Origin: srv.URL}, salt)
	_, err := cl.InstallPlugin(context.Background(), "abc-1", "apoc")
	if err == nil {
		t.Fatalf("expected error on 500")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != 1 {
		t.Fatalf("error = %v (%T)", err, err)
	}
	if !strings.Contains(ce.Message, "returned 500") {
		t.Fatalf("message missing status: %q", ce.Message)
	}
	if strings.Contains(ce.Message, bigBody) {
		t.Fatalf("message contains untruncated body")
	}
}

func TestClient_PluginEndpoints_ProbeMiss_MapsToCanonicalUnreachable(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	// Bind + immediately close so the port is unbound.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	dead := srv.URL
	srv.Close()

	cl, _ := NewClient(ProbeResult{Origin: dead}, salt)
	_, err := cl.ListInstalledPlugins(context.Background(), "abc-1")
	if err == nil {
		t.Fatalf("expected error for dead server")
	}
	if !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Fatalf("missing canonical message: %q", err.Error())
	}
}
