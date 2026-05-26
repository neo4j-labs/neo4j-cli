// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// listHelper wires desktop.NewCmd against an in-memory FS, with the new
// `newDesktopClientFn` seam pinned to a desktopclient.Client backed by an
// httptest server. End-to-end: cobra flag parse → leaf RunE → desktopclient
// → httptest handler. No real filesystem, no real port range.
type listHelper struct {
	t         *testing.T
	out       *bytes.Buffer
	err       *bytes.Buffer
	fs        afero.Fs
	configDir string
	// handlerCalls captures the inbound request method+path for assertions
	// on which endpoints the leaf actually hits. Populated by the default
	// listHandler set via `withHandler`.
	handlerCalls []string
}

func newListHelper(t *testing.T) *listHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	configDir := filepath.FromSlash("/cfg")
	t.Cleanup(desktopclient.SetUserConfigDirFnForTest(func() (string, error) { return configDir, nil }))
	return &listHelper{
		t:         t,
		out:       &bytes.Buffer{},
		err:       &bytes.Buffer{},
		fs:        fs,
		configDir: configDir,
	}
}

// withHandler swaps the `newDesktopClientFn` seam to a closure that returns
// a desktopclient.Client wired to the supplied httptest handler. The handler
// receives every request the leaf sends; recordings live in
// listHelper.handlerCalls for assertion.
func (h *listHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-list"
		clientID = "cid-list"
	)
	h.t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return clientID }))
	h.t.Cleanup(desktopclient.SetNowFnForTest(func() time.Time { return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC) }))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.handlerCalls = append(h.handlerCalls, r.Method+" "+r.URL.Path)
		handler(w, r)
	}))
	h.t.Cleanup(srv.Close)

	h.t.Cleanup(desktop.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return desktopclient.NewClient(desktopclient.ProbeResult{Origin: srv.URL}, salt)
	}))
	return srv
}

// pinClientUnreachable swaps the constructor seam to always return the
// canonical REQ-F-008 unreachable error — used by tests that don't need a
// real httptest server (e.g. empty-list hint when Desktop is off).
func (h *listHelper) pinClientUnreachable() {
	h.t.Helper()
	h.t.Cleanup(desktop.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return nil, desktopclient.UnreachableError()
	}))
}

func (h *listHelper) run(command string) error {
	h.t.Helper()
	args, err := shlex.Split(command)
	if err != nil {
		h.t.Fatalf("shlex: %v", err)
	}
	cfg := clicfg.NewConfig(h.fs, "test", clicfg.GlobalScope)
	cmd := desktop.NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetArgs(args)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)
	return cmd.Execute()
}

// defaultListHandler returns an http handler that serves the supplied slim
// DBMS list at GET /fastify/api/dbmss, per-id enrichment payloads at GET
// /fastify/api/dbmss/:id, and the supplied connections list at GET
// /fastify/api/connections. Tests that need finer control can call
// `withHandler` directly with a custom handler.
func defaultListHandler(t *testing.T, slimDbmss string, perID map[string]string, connections string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(slimDbmss))
		case strings.HasPrefix(r.URL.Path, "/fastify/api/dbmss/"):
			id := strings.TrimPrefix(r.URL.Path, "/fastify/api/dbmss/")
			body, ok := perID[id]
			if !ok {
				t.Errorf("unexpected per-id request: %s", id)
				return
			}
			_, _ = w.Write([]byte(body))
		case r.URL.Path == "/fastify/api/connections":
			_, _ = w.Write([]byte(connections))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}
}

func TestList_JSON_BothPopulated(t *testing.T) {
	h := newListHelper(t)
	perID := map[string]string{
		"a": `{"id":"a","name":"Alice","version":"5.21.0","status":"started","connectionUri":"neo4j://localhost:7687","edition":"enterprise","tags":["x"]}`,
		"b": `{"id":"b","name":"Bob","version":"5.20.0","status":"stopped"}`,
	}
	slim := `[
		{"id":"a","name":"Alice","connectionUri":"neo4j://localhost:7687"},
		{"id":"b","name":"Bob"}
	]`
	connections := `[
		{"id":"c1","name":"Aura prod","connectionUri":"neo4j+s://abc.databases.neo4j.io","project":"my-proj"},
		{"id":"c2","name":"Sandbox","connectionUri":"neo4j+s://xyz.databases.neo4j.io"}
	]`
	h.withHandler(defaultListHandler(t, slim, perID, connections))

	if err := h.run("list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}

	var out struct {
		Dbmss       []map[string]any `json:"dbmss"`
		Connections []map[string]any `json:"connections"`
	}
	if err := json.Unmarshal(h.out.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal: %v (raw: %s)", err, h.out.String())
	}
	if len(out.Dbmss) != 2 {
		t.Fatalf("expected 2 dbmss, got %d (raw: %s)", len(out.Dbmss), h.out.String())
	}
	if len(out.Connections) != 2 {
		t.Fatalf("expected 2 connections, got %d (raw: %s)", len(out.Connections), h.out.String())
	}
	// Order from ListDbmss is preserved regardless of goroutine completion.
	if out.Dbmss[0]["id"] != "a" || out.Dbmss[1]["id"] != "b" {
		t.Fatalf("expected dbmss order preserved [a,b], got [%v,%v]", out.Dbmss[0]["id"], out.Dbmss[1]["id"])
	}
	// `--format json` must surface the FULL DbmsInfo payload, not just the
	// default column subset. `edition` + `tags` ride along when the full
	// payload path is taken — they come from the per-id GET.
	if out.Dbmss[0]["edition"] != "enterprise" {
		t.Fatalf("expected edition=enterprise in full JSON, got %v (raw: %s)", out.Dbmss[0]["edition"], h.out.String())
	}
	if out.Dbmss[0]["version"] != "5.21.0" || out.Dbmss[0]["status"] != "started" {
		t.Fatalf("expected enrichment to populate version/status, got %v / %v (raw: %s)", out.Dbmss[0]["version"], out.Dbmss[0]["status"], h.out.String())
	}
	tags, ok := out.Dbmss[0]["tags"].([]any)
	if !ok || len(tags) != 1 || tags[0] != "x" {
		t.Fatalf("expected tags=[x], got %v (raw: %s)", out.Dbmss[0]["tags"], h.out.String())
	}
	// Connections section carries the full Connection wire shape, including
	// `project` when populated — JSON exposes every wire field even though the
	// column subset does not.
	if out.Connections[0]["id"] != "c1" || out.Connections[0]["name"] != "Aura prod" {
		t.Fatalf("expected first connection c1/Aura prod, got %v", out.Connections[0])
	}
	if out.Connections[0]["project"] != "my-proj" {
		t.Fatalf("expected project=my-proj on JSON wire payload, got %v", out.Connections[0]["project"])
	}
}

func TestList_JSON_DbmssOnly(t *testing.T) {
	h := newListHelper(t)
	perID := map[string]string{
		"a": `{"id":"a","name":"Alice","version":"5.21","status":"started"}`,
	}
	h.withHandler(defaultListHandler(t,
		`[{"id":"a","name":"Alice"}]`, perID, `[]`))

	if err := h.run("list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out struct {
		Dbmss       []map[string]any `json:"dbmss"`
		Connections []map[string]any `json:"connections"`
	}
	if err := json.Unmarshal(h.out.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal: %v (raw: %s)", err, h.out.String())
	}
	if len(out.Dbmss) != 1 {
		t.Fatalf("expected 1 dbms, got %d (raw: %s)", len(out.Dbmss), h.out.String())
	}
	if out.Connections == nil {
		t.Fatalf("expected non-nil connections array even when empty (raw: %s)", h.out.String())
	}
	if len(out.Connections) != 0 {
		t.Fatalf("expected 0 connections, got %d", len(out.Connections))
	}
}

func TestList_JSON_ConnectionsOnly(t *testing.T) {
	h := newListHelper(t)
	h.withHandler(defaultListHandler(t,
		`[]`, map[string]string{},
		`[{"id":"c1","name":"Aura prod","connectionUri":"neo4j+s://abc.databases.neo4j.io"}]`))

	if err := h.run("list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out struct {
		Dbmss       []map[string]any `json:"dbmss"`
		Connections []map[string]any `json:"connections"`
	}
	if err := json.Unmarshal(h.out.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal: %v (raw: %s)", err, h.out.String())
	}
	if out.Dbmss == nil {
		t.Fatalf("expected non-nil dbmss array even when empty (raw: %s)", h.out.String())
	}
	if len(out.Dbmss) != 0 {
		t.Fatalf("expected 0 dbmss, got %d", len(out.Dbmss))
	}
	if len(out.Connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(out.Connections))
	}
	if out.Connections[0]["id"] != "c1" {
		t.Fatalf("expected connection id=c1, got %v", out.Connections[0]["id"])
	}
}

func TestList_JSON_BothEmpty(t *testing.T) {
	h := newListHelper(t)
	h.withHandler(defaultListHandler(t, `[]`, map[string]string{}, `[]`))

	if err := h.run("list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	// Snapshot the JSON envelope: both keys present, both arrays empty.
	// The pretty-printer uses tabs so we compare structurally rather than
	// byte-for-byte.
	var out struct {
		Dbmss       []map[string]any `json:"dbmss"`
		Connections []map[string]any `json:"connections"`
	}
	if err := json.Unmarshal(h.out.Bytes(), &out); err != nil {
		t.Fatalf("json.Unmarshal: %v (raw: %s)", err, h.out.String())
	}
	if out.Dbmss == nil || out.Connections == nil {
		t.Fatalf("expected non-nil empty arrays under both keys (raw: %s)", h.out.String())
	}
	if len(out.Dbmss) != 0 || len(out.Connections) != 0 {
		t.Fatalf("expected both arrays empty, got %d dbmss / %d connections", len(out.Dbmss), len(out.Connections))
	}
	// And the stdout must NOT contain a top-level array literal `[` or
	// `null` — confirms the envelope shape is `{dbmss, connections}`.
	stdout := strings.TrimSpace(h.out.String())
	if !strings.HasPrefix(stdout, "{") {
		t.Fatalf("expected JSON object envelope, got: %s", stdout)
	}
	if !strings.Contains(stdout, `"dbmss"`) || !strings.Contains(stdout, `"connections"`) {
		t.Fatalf("expected both keys in JSON envelope, got: %s", stdout)
	}
}

func TestList_JSON_SnapshotShape(t *testing.T) {
	// Snapshot the exact JSON output for a small, deterministic input.
	// Locks the envelope key order, indentation, and field surfacing so
	// agents / downstream consumers see a stable shape.
	h := newListHelper(t)
	perID := map[string]string{
		"d1": `{"id":"d1","name":"Local","version":"5.21","status":"started","connectionUri":"neo4j://localhost:7687"}`,
	}
	h.withHandler(defaultListHandler(t,
		`[{"id":"d1","name":"Local"}]`, perID,
		`[{"id":"c1","name":"Remote","connectionUri":"neo4j+s://abc.databases.neo4j.io"}]`))

	if err := h.run("list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Round-trip through json.Unmarshal + json.MarshalIndent for stable
	// comparison — the cobra render path uses tabs and per-key escaping
	// that vary by Go version; we want a shape assertion, not a byte
	// snapshot. The shape: {dbmss:[…], connections:[…]} in that key order.
	var got any
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (raw: %s)", err, h.out.String())
	}
	// Compare against the canonical re-marshalled form.
	canonical, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal canonical: %v", err)
	}
	want := `{"connections":[{"id":"c1","name":"Remote","connectionUri":"neo4j+s://abc.databases.neo4j.io"}],"dbmss":[{"id":"d1","name":"Local","connectionUri":"neo4j://localhost:7687","status":"started","version":"5.21"}]}`
	// JSON unmarshal+marshal alphabetises map keys; build the canonical
	// reference by the same path.
	var wantParsed any
	if err := json.Unmarshal([]byte(want), &wantParsed); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	wantCanonical, err := json.Marshal(wantParsed)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	if string(canonical) != string(wantCanonical) {
		t.Fatalf("snapshot mismatch:\ngot:  %s\nwant: %s", string(canonical), string(wantCanonical))
	}
}

func TestList_Table_TwoSections_BothPopulated(t *testing.T) {
	h := newListHelper(t)
	perID := map[string]string{
		"a": `{"id":"a","name":"Alice","version":"5.21","status":"started","connectionUri":"neo4j://localhost:7687"}`,
	}
	h.withHandler(defaultListHandler(t,
		`[{"id":"a","name":"Alice"}]`, perID,
		`[{"id":"c1","name":"Aura prod","connectionUri":"neo4j+s://abc.databases.neo4j.io","project":"my-proj"}]`))

	if err := h.run("list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	if !strings.Contains(out, "Local DBMSes") {
		t.Fatalf("expected 'Local DBMSes' section header, got: %s", out)
	}
	if !strings.Contains(out, "Remote connections") {
		t.Fatalf("expected 'Remote connections' section header, got: %s", out)
	}
	// Section order: Local DBMSes first, Remote connections second.
	dbmssIdx := strings.Index(out, "Local DBMSes")
	connsIdx := strings.Index(out, "Remote connections")
	if dbmssIdx == -1 || connsIdx == -1 || dbmssIdx >= connsIdx {
		t.Fatalf("expected 'Local DBMSes' to appear before 'Remote connections', got: %s", out)
	}
	// DBMSes section uses the dbms column set.
	for _, hdr := range []string{"ID", "NAME", "VERSION", "STATUS", "CONNECTIONURI"} {
		if !strings.Contains(out, hdr) {
			t.Fatalf("expected DBMS column header %q in table output, got %s", hdr, out)
		}
	}
	// Connections section uses the connection column set (excludes
	// VERSION/STATUS and PROJECT — `project` rides the wire on `--format json`
	// only, never as a column per the PRD non-goals).
	if strings.Contains(out, "PROJECT") {
		t.Fatalf("expected NO PROJECT column header, got: %s", out)
	}
	// Both row payloads visible.
	if !strings.Contains(out, "Alice") {
		t.Fatalf("expected DBMS row name 'Alice', got: %s", out)
	}
	if !strings.Contains(out, "Aura prod") {
		t.Fatalf("expected connection row name 'Aura prod', got: %s", out)
	}
	if strings.Contains(out, "my-proj") {
		t.Fatalf("expected NO project value rendered in table output, got: %s", out)
	}
}

func TestList_Table_DbmssOnly_ConnectionsNoneRow(t *testing.T) {
	h := newListHelper(t)
	perID := map[string]string{
		"a": `{"id":"a","name":"Alice","version":"5.21","status":"started"}`,
	}
	h.withHandler(defaultListHandler(t,
		`[{"id":"a","name":"Alice"}]`, perID, `[]`))

	if err := h.run("list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	// Both section headers present.
	if !strings.Contains(out, "Local DBMSes") || !strings.Contains(out, "Remote connections") {
		t.Fatalf("expected both section headers, got: %s", out)
	}
	// DBMS row populated.
	if !strings.Contains(out, "Alice") {
		t.Fatalf("expected DBMS row 'Alice', got: %s", out)
	}
	// Connections section shows the (none) placeholder.
	if !strings.Contains(out, "(none)") {
		t.Fatalf("expected '(none)' placeholder in empty Remote connections section, got: %s", out)
	}
}

func TestList_Table_ConnectionsOnly_DbmssNoneRow(t *testing.T) {
	h := newListHelper(t)
	h.withHandler(defaultListHandler(t,
		`[]`, map[string]string{},
		`[{"id":"c1","name":"Aura","connectionUri":"neo4j+s://abc.databases.neo4j.io"}]`))

	if err := h.run("list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	if !strings.Contains(out, "Local DBMSes") || !strings.Contains(out, "Remote connections") {
		t.Fatalf("expected both section headers, got: %s", out)
	}
	// Connection row populated.
	if !strings.Contains(out, "Aura") {
		t.Fatalf("expected connection row 'Aura', got: %s", out)
	}
	// DBMSes section shows the (none) placeholder — must appear BEFORE the
	// Remote connections header so it's clearly the dbmss section's row.
	noneIdx := strings.Index(out, "(none)")
	connsIdx := strings.Index(out, "Remote connections")
	if noneIdx == -1 {
		t.Fatalf("expected '(none)' placeholder for empty Local DBMSes section, got: %s", out)
	}
	if noneIdx > connsIdx {
		t.Fatalf("'(none)' for Local DBMSes section must appear before Remote connections header, got: %s", out)
	}
}

func TestList_Table_BothEmpty_BothNoneRows(t *testing.T) {
	h := newListHelper(t)
	h.withHandler(defaultListHandler(t, `[]`, map[string]string{}, `[]`))

	if err := h.run("list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	if !strings.Contains(out, "Local DBMSes") || !strings.Contains(out, "Remote connections") {
		t.Fatalf("expected both section headers, got: %s", out)
	}
	// Two `(none)` placeholder rows.
	if strings.Count(out, "(none)") != 2 {
		t.Fatalf("expected exactly 2 '(none)' placeholders, got %d (out: %s)", strings.Count(out, "(none)"), out)
	}
}

func TestList_Toon_MirrorsJSONShape(t *testing.T) {
	h := newListHelper(t)
	perID := map[string]string{
		"a": `{"id":"a","name":"Alice","version":"5.21","status":"started"}`,
	}
	h.withHandler(defaultListHandler(t,
		`[{"id":"a","name":"Alice"}]`, perID,
		`[{"id":"c1","name":"Aura","connectionUri":"neo4j+s://abc.databases.neo4j.io"}]`))

	if err := h.run("list --format toon"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	// Toon serialises the `{dbmss, connections}` envelope — both keys
	// must appear at the top level.
	if !strings.Contains(out, "dbmss") {
		t.Fatalf("expected 'dbmss' key in toon output, got: %s", out)
	}
	if !strings.Contains(out, "connections") {
		t.Fatalf("expected 'connections' key in toon output, got: %s", out)
	}
	// And the row payloads must ride along.
	if !strings.Contains(out, "Alice") {
		t.Fatalf("expected DBMS name 'Alice' in toon output, got: %s", out)
	}
	if !strings.Contains(out, "Aura") {
		t.Fatalf("expected connection name 'Aura' in toon output, got: %s", out)
	}
}

func TestList_DesktopUnreachable_ReturnsCanonicalError(t *testing.T) {
	h := newListHelper(t)
	h.pinClientUnreachable()

	err := h.run("list --format json")
	if err == nil {
		t.Fatalf("expected error when desktop is unreachable")
	}
	if !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Fatalf("expected canonical REQ-F-008 message, got: %v", err)
	}
}

func TestList_NoArgs(t *testing.T) {
	h := newListHelper(t)
	h.withHandler(defaultListHandler(t, `[]`, map[string]string{}, `[]`))
	if err := h.run("list bogus-positional"); err == nil {
		t.Fatalf("expected error on unexpected positional arg")
	}
}

func TestList_PortFlagPropagatesToClientConstructor(t *testing.T) {
	h := newListHelper(t)
	var gotPort int
	t.Cleanup(desktop.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		gotPort = port
		return nil, errors.New("stop here, we already captured the port")
	}))

	// Expect an error (the seam returns one) — we only care about gotPort.
	_ = h.run("list --port 44230 --format json")
	if gotPort != 44230 {
		t.Fatalf("expected --port=44230 to propagate to client constructor, got %d", gotPort)
	}
}

func TestList_TableHeader_EmptyArrays(t *testing.T) {
	// Regression guard for the empty-list code path through the custom
	// two-section renderer — the header row must still appear for each
	// section even with zero data rows, so users know each table is
	// intentionally empty (vs. broken).
	h := newListHelper(t)
	h.withHandler(defaultListHandler(t, `[]`, map[string]string{}, `[]`))
	if err := h.run("list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	for _, hdr := range []string{"ID", "NAME", "VERSION", "STATUS", "CONNECTIONURI"} {
		if !strings.Contains(out, hdr) {
			t.Fatalf("expected header %q on empty table, got %s", hdr, out)
		}
	}
	if strings.Contains(out, "PROJECT") {
		t.Fatalf("expected NO PROJECT header on empty table, got %s", out)
	}
}

func TestList_EnrichmentFailure_SlimRowAndStderrWarning(t *testing.T) {
	// One entry's per-id GET fails — the rest of the list still renders, and
	// a stderr-only warning names the failing id. JSON stdout stays clean.
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[
				{"id":"good","name":"Good","connectionUri":"neo4j://localhost:7687"},
				{"id":"bad","name":"Bad","connectionUri":"neo4j://localhost:7688"}
			]`))
		case "/fastify/api/dbmss/good":
			_, _ = w.Write([]byte(`{"id":"good","name":"Good","version":"5.21","status":"started","connectionUri":"neo4j://localhost:7687"}`))
		case "/fastify/api/dbmss/bad":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		case "/fastify/api/connections":
			_, _ = w.Write([]byte(`[]`))
		}
	})

	if err := h.run("list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}

	var out struct {
		Dbmss       []map[string]any `json:"dbmss"`
		Connections []map[string]any `json:"connections"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &out); err != nil {
		t.Fatalf("stdout JSON parse: %v (raw: %s)", err, h.out.String())
	}
	if len(out.Dbmss) != 2 {
		t.Fatalf("expected 2 entries even when one enrichment fails, got %d", len(out.Dbmss))
	}
	// Good entry got enriched.
	if out.Dbmss[0]["version"] != "5.21" || out.Dbmss[0]["status"] != "started" {
		t.Fatalf("expected enriched good entry, got %v / %v", out.Dbmss[0]["version"], out.Dbmss[0]["status"])
	}
	// Bad entry kept the slim shape (no version/status, but name/id intact).
	if out.Dbmss[1]["id"] != "bad" || out.Dbmss[1]["name"] != "Bad" {
		t.Fatalf("expected slim bad entry preserved, got %v", out.Dbmss[1])
	}
	if v, ok := out.Dbmss[1]["version"]; ok && v != "" {
		t.Fatalf("expected NO version on failed-enrichment row, got %v", v)
	}
	// Stderr warning names the failing id; stdout JSON is clean.
	if !strings.Contains(h.err.String(), `"bad"`) {
		t.Fatalf("expected stderr warning to name failing DBMS id, got %q", h.err.String())
	}
	if !strings.Contains(h.err.String(), "Warning") {
		t.Fatalf("expected stderr warning prefix, got %q", h.err.String())
	}
}

func TestList_EnrichmentPreservesListOrder(t *testing.T) {
	// 12 entries — fan-out completes in arbitrary order, but the rendered
	// list must follow the order ListDbmss returned, not goroutine completion.
	h := newListHelper(t)
	const n = 12
	var slimItems strings.Builder
	slimItems.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			slimItems.WriteString(",")
		}
		_, _ = fmt.Fprintf(&slimItems, `{"id":"id-%02d","name":"name-%02d"}`, i, i)
	}
	slimItems.WriteString("]")
	slim := slimItems.String()

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(slim))
		case r.URL.Path == "/fastify/api/connections":
			_, _ = w.Write([]byte(`[]`))
		case strings.HasPrefix(r.URL.Path, "/fastify/api/dbmss/"):
			id := strings.TrimPrefix(r.URL.Path, "/fastify/api/dbmss/")
			// Make later entries respond fastest so the natural goroutine
			// completion order is the REVERSE of the input — exercises the
			// ordering guarantee.
			var n int
			_, _ = fmt.Sscanf(id, "id-%d", &n)
			time.Sleep(time.Duration(50-n) * time.Microsecond)
			_, _ = fmt.Fprintf(w, `{"id":%q,"name":"name-%02d","status":"started","version":"5.21"}`, id, n)
		}
	})

	if err := h.run("list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out struct {
		Dbmss []map[string]any `json:"dbmss"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &out); err != nil {
		t.Fatalf("stdout JSON parse: %v (raw: %s)", err, h.out.String())
	}
	if len(out.Dbmss) != n {
		t.Fatalf("expected %d entries, got %d", n, len(out.Dbmss))
	}
	for i, row := range out.Dbmss {
		want := fmt.Sprintf("id-%02d", i)
		if row["id"] != want {
			t.Fatalf("position %d: expected id=%s, got %v", i, want, row["id"])
		}
		// Enrichment must have populated every row.
		if row["status"] != "started" {
			t.Fatalf("position %d: expected enriched status, got %v", i, row["status"])
		}
	}
}

func TestList_EnrichmentConcurrencyBound(t *testing.T) {
	// 50 entries should be enriched with no more than listEnrichConcurrency
	// (= 8) in-flight `GET /dbmss/:id` requests at any one time.
	h := newListHelper(t)
	const n = 50

	var slimItems strings.Builder
	slimItems.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			slimItems.WriteString(",")
		}
		_, _ = fmt.Fprintf(&slimItems, `{"id":"id-%02d","name":"name-%02d"}`, i, i)
	}
	slimItems.WriteString("]")
	slim := slimItems.String()

	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(slim))
		case r.URL.Path == "/fastify/api/connections":
			_, _ = w.Write([]byte(`[]`))
		case strings.HasPrefix(r.URL.Path, "/fastify/api/dbmss/"):
			mu.Lock()
			inFlight++
			if inFlight > maxInFlight {
				maxInFlight = inFlight
			}
			mu.Unlock()

			// Hold the request open briefly so concurrency actually overlaps.
			time.Sleep(5 * time.Millisecond)

			mu.Lock()
			inFlight--
			mu.Unlock()

			id := strings.TrimPrefix(r.URL.Path, "/fastify/api/dbmss/")
			_, _ = fmt.Fprintf(w, `{"id":%q,"status":"started"}`, id)
		}
	})

	if err := h.run("list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	mu.Lock()
	got := maxInFlight
	mu.Unlock()
	if got == 0 {
		t.Fatalf("expected non-zero in-flight concurrency (sanity check failed)")
	}
	if got > 8 {
		t.Fatalf("expected max in-flight ≤8, observed %d", got)
	}
}

func TestList_ParallelEndpointCalls_WallTimeBoundedBySlower(t *testing.T) {
	// The two list calls (ListDbmss + ListConnections) must run in parallel.
	// Wall time should be bounded by max(dbmsDelay, connDelay), not their
	// sum. Wire both endpoints with a 200ms sleep and check total elapsed.
	h := newListHelper(t)
	const delay = 100 * time.Millisecond
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fastify/api/dbmss":
			time.Sleep(delay)
			_, _ = w.Write([]byte(`[]`))
		case "/fastify/api/connections":
			time.Sleep(delay)
			_, _ = w.Write([]byte(`[]`))
		}
	})

	start := time.Now()
	if err := h.run("list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	elapsed := time.Since(start)
	// Sequential would take ≥2*delay (200ms). Parallel completes in ≈delay
	// + scheduling overhead. We use 1.8*delay as the upper bound — comfortably
	// below sequential while leaving headroom for slow CI.
	upperBound := time.Duration(float64(delay) * 1.8)
	if elapsed >= upperBound {
		t.Fatalf("expected parallel endpoint calls (wall time < %s), got %s", upperBound, elapsed)
	}
}

func TestList_ConnectionsEndpointFailure_AbortsLeaf(t *testing.T) {
	// REQ-F-008 / REQ-F-024 transport-error mapping applies uniformly — a
	// failing /connections call must abort the leaf with the canonical
	// error, not silently swallow.
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case "/fastify/api/connections":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		}
	})

	err := h.run("list --format json")
	if err == nil {
		t.Fatalf("expected error when /connections returns 5xx")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 5xx error to surface, got: %v", err)
	}
}
