//go:build smoke

// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSmoke_Lifecycle exercises the docker create → list → get → delete
// lifecycle against a REAL docker daemon. It is gated behind:
//  1. The `//go:build smoke` tag at file top, so default `go test ./...`
//     never compiles this file. Opt-in with `go test -tags=smoke ./...`.
//  2. A runtime `exec.LookPath("docker")` guard that t.Skips when docker is
//     not on PATH, so `-tags=smoke` on a docker-less host is also safe.
//
// The test:
//   - picks a unique container name keyed on time.Now().UnixNano()
//   - grabs two ephemeral free host ports via net.Listen("tcp", ":0")
//   - registers a t.Cleanup that best-effort `docker rm -f`s the container
//     so an aborted test never leaves a managed container behind
//   - uses an in-memory afero fs + --no-store-credential, so the smoke does
//     not touch the developer's real cli config / credential store
//   - asserts the four lifecycle phases each succeed and the documented
//     `get --format json` field set is present (including image=neo4j:enterprise,
//     a task-022 regression guard)
//
// NOT part of `make test`; expect ~5s when the neo4j:enterprise image is
// cached on the host, ~30s on first pull.
func TestSmoke_Lifecycle(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH; skipping live smoke")
	}

	name := fmt.Sprintf("neo4j-cli-smoke-%d", time.Now().UnixNano())
	boltPort := pickFreePort(t)
	httpPort := pickFreePort(t)
	require.NotEqual(t, boltPort, httpPort, "free-port helper handed out the same port twice")

	// Best-effort cleanup — runs even if the test t.Fatals partway through
	// or if a phase errors. Ignore the rm error: the container may already
	// be gone (e.g. after a successful delete phase) and we don't want
	// double-failures masking the real assertion failure.
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})

	// Hermetic config — memfs so the smoke does not touch the operator's
	// real ~/.config/neo4j/cli/{config,credentials}.json. Combined with
	// --no-store-credential below, the on-disk footprint is zero.
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "smoke-test", clicfg.GlobalScope)

	// Phase 1 — create. --rw is documented as required on write invocations
	// (see SKILL.md / README); the write gate is not wired into this rig but
	// we pass --rw for fidelity to the documented operator invocation.
	_, _, err = runDockerSubcommand(t, cfg, "create",
		"--name", name,
		"--no-store-credential",
		"--bolt-port", strconv.Itoa(boltPort),
		"--http-port", strconv.Itoa(httpPort),
		"--rw",
	)
	require.NoError(t, err, "create phase failed")

	// Phase 2 — list. Parse the JSON array and locate our row.
	listStdout, _, err := runDockerSubcommand(t, cfg, "list", "--format", "json")
	require.NoError(t, err, "list phase failed")

	var listRows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(listStdout), &listRows), "list stdout is not JSON: %s", listStdout)

	var listRow map[string]any
	for _, row := range listRows {
		if asString(row["name"]) == name {
			listRow = row
			break
		}
	}
	require.NotNil(t, listRow, "list output did not include container %q; stdout=%s", name, listStdout)
	assert.Equal(t, "enterprise", asString(listRow["edition"]), "list edition mismatch")
	assert.Equal(t, strconv.Itoa(boltPort), asString(listRow["bolt-port"]), "list bolt-port mismatch")

	// Phase 3 — get. singleRow.MarshalJSON wraps the row in a one-element
	// JSON array, so we parse as []map[string]any and pull row[0].
	getStdout, _, err := runDockerSubcommand(t, cfg, "get", name, "--format", "json")
	require.NoError(t, err, "get phase failed")

	var getRows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(getStdout), &getRows), "get stdout is not JSON: %s", getStdout)
	require.Len(t, getRows, 1, "get must emit exactly one row; got %s", getStdout)
	getRow := getRows[0]

	assert.Equal(t, name, asString(getRow["name"]), "get name mismatch")
	assert.Equal(t, fmt.Sprintf("neo4j://localhost:%d", boltPort), asString(getRow["uri"]), "get uri mismatch")
	// task-022 regression guard: `--edition enterprise --version latest` MUST
	// resolve to the bare `neo4j:enterprise` tag (Docker Hub does NOT publish
	// neo4j:latest-enterprise). Pin it live so a future image-resolution
	// refactor that silently re-breaks the tag is caught by the smoke.
	assert.Equal(t, "neo4j:enterprise", asString(getRow["image"]), "get image must be bare neo4j:enterprise")

	// All 9 documented fields are present; status is Docker's
	// "Up X seconds" or similar (non-empty); ephemeral is bool false.
	for _, field := range []string{"name", "status", "edition", "version", "bolt-port", "http-port", "ephemeral", "uri", "image"} {
		_, ok := getRow[field]
		assert.True(t, ok, "get row missing field %q; row=%v", field, getRow)
	}
	assert.NotEmpty(t, asString(getRow["status"]), "get status must be non-empty (Docker emits 'Up X seconds')")
	assert.Equal(t, false, getRow["ephemeral"], "ephemeral must be bool false for non-ephemeral container")
	assert.Equal(t, "enterprise", asString(getRow["edition"]), "get edition mismatch")
	assert.Equal(t, strconv.Itoa(boltPort), asString(getRow["bolt-port"]), "get bolt-port mismatch")
	assert.Equal(t, strconv.Itoa(httpPort), asString(getRow["http-port"]), "get http-port mismatch")

	// Phase 4 — delete. --force skips the TTY confirmation; --rw passes the
	// write gate even when stdout is not a TTY in CI. Then verify with a
	// direct `docker ps -a --filter name=^<name>$` that the daemon no longer
	// knows about the container.
	_, _, err = runDockerSubcommand(t, cfg, "delete", name, "--force", "--rw")
	require.NoError(t, err, "delete phase failed")

	out, dockerErr := exec.Command("docker", "ps", "-a",
		"--filter", "name=^"+name+"$",
		"--format", "{{.Names}}",
	).Output()
	require.NoError(t, dockerErr, "docker ps probe failed")
	assert.Empty(t, strings.TrimSpace(string(out)),
		"docker delete should have removed the container; docker ps still reports: %q", string(out))
}

// TestSmoke_PortFallback exercises the auto-port-fallback path (CLI-131)
// against a REAL docker daemon. Like TestSmoke_Lifecycle it is gated behind
// the `//go:build smoke` tag plus a runtime `exec.LookPath("docker")` skip.
//
// The flow:
//   - pick two distinct ephemeral free host ports (P, Q).
//   - create container A bound to (P, Q) so those ports are genuinely in
//     use by the docker daemon at the OS level.
//   - create container B requesting the SAME (P, Q). The fallback loop
//     in create.go must walk the pair upward by the same offset until a
//     free pair is found, narrate the resolution on stderr, and bake the
//     resolved pair into both the `docker run -p` argv and the container
//     labels.
//   - assert B's stderr carries the `info: ports P/Q in use; using ...`
//     line; parse `get <B-name> --format json` and confirm bolt-port /
//     http-port / uri all reflect the resolved (higher) pair; confirm
//     the offset is >= 1 and identical on both ports (preserved-delta
//     invariant from REQ-F-004).
//   - cross-check the container labels via `docker inspect` so a future
//     regression that desyncs the label set from the published ports
//     gets caught here.
//
// Cleanup runs even on partial failure: both containers are best-effort
// `docker rm -f`'d so an aborted run never leaks managed containers.
//
// NOT part of `make test`; expect ~5s when neo4j:enterprise is cached,
// ~30s on first pull (same cost profile as TestSmoke_Lifecycle).
func TestSmoke_PortFallback(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH; skipping live smoke")
	}

	// Distinct base names so list/get can't accidentally cross-match.
	stamp := time.Now().UnixNano()
	nameA := fmt.Sprintf("neo4j-cli-smoke-fb-a-%d", stamp)
	nameB := fmt.Sprintf("neo4j-cli-smoke-fb-b-%d", stamp)

	boltPort := pickFreePort(t)
	httpPort := pickFreePort(t)
	require.NotEqual(t, boltPort, httpPort, "free-port helper handed out the same port twice")

	// Best-effort cleanup for BOTH containers — runs even if a phase fails.
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", nameA).Run()
		_ = exec.Command("docker", "rm", "-f", nameB).Run()
	})

	// Hermetic config — see TestSmoke_Lifecycle for the rationale.
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "smoke-test", clicfg.GlobalScope)

	// Phase 1 — create container A on the chosen pair. After this the host
	// ports P and Q are genuinely in use, so the next create's listener
	// pre-flight will see them as taken.
	_, _, err = runDockerSubcommand(t, cfg, "create",
		"--name", nameA,
		"--no-store-credential",
		"--bolt-port", strconv.Itoa(boltPort),
		"--http-port", strconv.Itoa(httpPort),
		"--rw",
	)
	require.NoError(t, err, "create A failed")

	// Phase 2 — request the SAME pair for container B. The fallback loop
	// must resolve to (boltPort+offset, httpPort+offset) for some offset>=1.
	_, stderrB, err := runDockerSubcommand(t, cfg, "create",
		"--name", nameB,
		"--no-store-credential",
		"--bolt-port", strconv.Itoa(boltPort),
		"--http-port", strconv.Itoa(httpPort),
		"--rw",
	)
	require.NoError(t, err, "create B failed; stderr=%s", stderrB)

	// Phase 3 — stderr must carry the resolution narration. We assert the
	// prefix (request side) and that "using" appears; the resolved pair
	// is value-checked via `get --format json` below for a single source
	// of truth.
	wantPrefix := fmt.Sprintf("info: ports %d/%d in use; using ", boltPort, httpPort)
	assert.Contains(t, stderrB, wantPrefix,
		"stderr B missing fallback narration prefix; stderr=%s", stderrB)

	// Phase 4 — pull B's resolved ports via `get --format json` and compute
	// the offset. The bolt-port / http-port fields are strings (label-derived)
	// so we coerce via asString + strconv.Atoi.
	getStdout, _, err := runDockerSubcommand(t, cfg, "get", nameB, "--format", "json")
	require.NoError(t, err, "get B failed")

	var getRows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(getStdout), &getRows), "get stdout is not JSON: %s", getStdout)
	require.Len(t, getRows, 1, "get must emit exactly one row; got %s", getStdout)
	row := getRows[0]

	resolvedBolt, err := strconv.Atoi(asString(row["bolt-port"]))
	require.NoError(t, err, "B bolt-port not parseable; row=%v", row)
	resolvedHTTP, err := strconv.Atoi(asString(row["http-port"]))
	require.NoError(t, err, "B http-port not parseable; row=%v", row)

	boltOffset := resolvedBolt - boltPort
	httpOffset := resolvedHTTP - httpPort
	assert.GreaterOrEqual(t, boltOffset, 1, "B bolt-port should have walked at least 1 (was %d, started %d)", resolvedBolt, boltPort)
	assert.Equal(t, boltOffset, httpOffset, "preserved-delta invariant: bolt and http offsets must match (got bolt=%d http=%d)", boltOffset, httpOffset)

	// uri must reflect the resolved bolt port — operators copy/paste this.
	assert.Equal(t, fmt.Sprintf("neo4j://localhost:%d", resolvedBolt), asString(row["uri"]), "B uri mismatch")

	// Phase 5 — cross-check container labels via `docker inspect` so a
	// regression that desyncs labels from the published port set is caught.
	// `--format '{{json .Config.Labels}}'` keeps the parse trivial.
	inspectOut, inspectErr := exec.Command("docker", "inspect",
		"--format", "{{json .Config.Labels}}", nameB,
	).Output()
	require.NoError(t, inspectErr, "docker inspect B failed")

	var labels map[string]string
	require.NoError(t, json.Unmarshal(inspectOut, &labels), "docker inspect labels JSON parse failed: %s", string(inspectOut))
	assert.Equal(t, strconv.Itoa(resolvedBolt), labels[LabelBoltPort], "label %s desync", LabelBoltPort)
	assert.Equal(t, strconv.Itoa(resolvedHTTP), labels[LabelHTTPPort], "label %s desync", LabelHTTPPort)
}

// pickFreePort allocates and immediately releases a TCP port on 127.0.0.1.
// The tiny TOCTOU window between Close and `docker run -p` claiming the
// port is acceptable for a dev-only smoke test; the OS rarely re-uses an
// ephemeral port that quickly. Mirrors query/query_bolt_smoke_test.go's
// freeBoltPort helper.
func pickFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

// runDockerSubcommand builds a fresh `docker` cobra subtree wired with the
// same persistent flags (`--format`, `--rw`) the production root registers,
// captures stdout/stderr into buffers, and executes the given argv. Each
// phase rebuilds the cmd because cobra commands are stateful across
// invocations (parsed flags, args, errors leak between Execute calls).
//
// Returns stdout, stderr, and Execute's error. On error the caller surfaces
// stderr in the require.NoError failure message for easier triage.
func runDockerSubcommand(t *testing.T, cfg *clicfg.Config, argv ...string) (stdout, stderr string, err error) {
	t.Helper()

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	flags.RegisterRwFlag(cmd)

	// Wrap with a tiny root so cobra resolves the subcommand path and the
	// persistent --format / --rw flags propagate the way they do under the
	// real neo4j-cli root.
	root := &cobra.Command{Use: "neo4j-cli"}
	root.AddCommand(cmd)

	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(append([]string{"docker"}, argv...))

	err = root.Execute()
	if err != nil {
		t.Logf("subcommand %q stderr: %s", strings.Join(argv, " "), errBuf.String())
	}
	return outBuf.String(), errBuf.String(), err
}

// asString coerces a JSON-decoded value into a string. The list / get JSON
// shapes return strings for label-derived fields (edition, version,
// bolt-port, http-port) because Docker labels are strings end-to-end; this
// helper centralises the type assertion so the smoke's assertions read clean.
func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
