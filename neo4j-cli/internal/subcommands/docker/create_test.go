// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// constantReader is a deterministic io.Reader used to seed the password-byte
// generation seam (randSource) so tests can assert the exact base64 output.
type constantReader struct {
	b byte
}

func (c constantReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = c.b
	}
	return len(p), nil
}

// fakeListener is the in-memory net.Listener returned by the test
// listenerFactory when a port is "free". Close is a no-op; the other
// methods are unused because create.go only ever calls Close after Listen.
type fakeListener struct{}

func (fakeListener) Accept() (net.Conn, error) {
	return nil, errors.New("fakeListener: Accept not supported")
}
func (fakeListener) Close() error   { return nil }
func (fakeListener) Addr() net.Addr { return &net.TCPAddr{} }

// stubListenerFactory swaps the package-level listenerFactory seam so port
// pre-flight checks (REQ-F-013) are deterministic and hermetic — no real
// sockets are opened. Each port in `occupied` returns a sentinel error
// keyed by port number; all other ports return a no-op fakeListener.
// Returns a cleanup func and a *[]int the test can read to assert which
// ports were probed and in what order.
func stubListenerFactory(t *testing.T, occupied ...int) (calls *[]int) {
	t.Helper()
	busy := map[int]bool{}
	for _, p := range occupied {
		busy[p] = true
	}
	var probed []int
	orig := listenerFactory
	listenerFactory = func(port int) (net.Listener, error) {
		probed = append(probed, port)
		if busy[port] {
			return nil, fmt.Errorf("fakeListener: port %d is occupied (test stub)", port)
		}
		return fakeListener{}, nil
	}
	t.Cleanup(func() { listenerFactory = orig })
	return &probed
}

// runCreate builds the docker parent command (with create leaf wired), swaps
// the package-level clientFactory seam for the supplied fake, and executes the
// given shell-like argument string. It returns the fake (for argv assertions),
// the cfg (for credential assertions), and stdout (for output-format
// assertions).
//
// runCreate also stubs the listenerFactory with no occupied ports so the
// port-conflict pre-flight (REQ-F-013) never touches real sockets — keeping
// the package's tests hermetic per AGENTS.md "Hermetic Test Notes". Tests
// that need to simulate an occupied port use runCreateWithOccupiedPorts.
func runCreate(t *testing.T, args string) (*fakeDockerClient, *clicfg.Config, string, error) {
	t.Helper()
	return runCreateWithOccupiedPorts(t, args)
}

// shlexQuote wraps s in single quotes so google/shlex preserves backslashes
// when splitting the test args string. Windows temp-dir paths from
// t.TempDir() (e.g. `C:\Users\RUNNER~1\AppData\Local\Temp\...`) would
// otherwise have every `\X` consumed as an escape sequence by shlex,
// mangling the path before it reaches expandHostPath. Single quotes are
// literal in shlex — backslashes inside them survive. Embedded single
// quotes are POSIX-escaped via the standard close/escape/open dance.
// (Refactoring the runCreate* helpers to take []string would also fix
// this but would touch every call site in the file.)
func shlexQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// runCreateWithOccupiedPorts is the same as runCreate but installs a
// listenerFactory that simulates the given ports as already-bound. It
// exists so port-conflict cases can drive the pre-flight deterministically
// without ever opening a real socket.
func runCreateWithOccupiedPorts(t *testing.T, args string, occupiedPorts ...int) (*fakeDockerClient, *clicfg.Config, string, error) {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	fake := newFakeDockerClient()
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	stubListenerFactory(t, occupiedPorts...)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"create"}, argv...))

	execErr := cmd.Execute()
	return fake, cfg, out.String(), execErr
}

// runArgv returns the recorded argv from the fake client's first Run call.
// Tests use it to assert -p / -e / --label / image shape.
func runArgv(t *testing.T, fake *fakeDockerClient) []string {
	t.Helper()
	require.Len(t, fake.RunCalls, 1, "expected exactly one docker run invocation")
	return fake.RunCalls[0]
}

// containsPair returns true when argv contains the consecutive sequence
// [flag, value]. Used to assert flag-bearing argv pairs (e.g. -e KEY=val).
func containsPair(argv []string, flag, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

func TestCreate_HappyPath_StoresCredentialAndSetsExpectedArgs(t *testing.T) {
	// Use a deterministic randSource so the generated password is assertable.
	origRand := randSource
	randSource = constantReader{b: 0xAB}
	defer func() { randSource = origRand }()
	expectedPassword := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xAB}, generatedPasswordBytes))

	fake, cfg, _, err := runCreate(t, "--name dev")
	require.NoError(t, err)

	argv := runArgv(t, fake)

	// Container name.
	assert.True(t, containsPair(argv, "--name", "dev"), "argv missing --name dev: %v", argv)
	// Port publishings.
	assert.True(t, containsPair(argv, "-p", "7474:7474"), "argv missing HTTP -p mapping: %v", argv)
	assert.True(t, containsPair(argv, "-p", "7687:7687"), "argv missing Bolt -p mapping: %v", argv)
	// NEO4J_AUTH env carries the generated password.
	assert.True(t, containsPair(argv, "-e", "NEO4J_AUTH=neo4j/"+expectedPassword),
		"argv missing NEO4J_AUTH env: %v", argv)
	// Enterprise license env present and default value is "eval" (REQ-F-012).
	assert.True(t, containsPair(argv, "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval"),
		"argv missing license env: %v", argv)
	// All six required labels present.
	for _, lbl := range []string{
		LabelManaged + "=true",
		LabelEdition + "=enterprise",
		LabelVersion + "=latest",
		LabelBoltPort + "=7687",
		LabelHTTPPort + "=7474",
		LabelEphemeral + "=false",
	} {
		assert.True(t, containsPair(argv, "--label", lbl), "argv missing label %q: %v", lbl, argv)
	}
	// Image is last; default enterprise + latest → bare `neo4j:enterprise`
	// (Docker Hub does NOT publish a `latest-enterprise` tag).
	assert.Equal(t, "neo4j:enterprise", argv[len(argv)-1])

	// Credential persisted with name=dev, the generated password, and a
	// localhost URI keyed off --bolt-port.
	cred, err := cfg.Credentials.Dbms.Get("dev")
	require.NoError(t, err)
	assert.Equal(t, "neo4j", cred.Username)
	assert.Equal(t, expectedPassword, cred.Password)
	assert.Equal(t, "neo4j", cred.DatabaseName)
	assert.Equal(t, "neo4j://localhost:7687", cred.URI)
}

func TestCreate_NoStoreCredential_SkipsPersistence(t *testing.T) {
	fake, cfg, _, err := runCreate(t, "--name dev --no-store-credential")
	require.NoError(t, err)
	require.Len(t, fake.RunCalls, 1, "container should still be created")
	assert.Empty(t, cfg.Credentials.Dbms.List(), "no credential should be stored")
}

func TestCreate_CommunityEdition_NoLicenseEnvAndPlainImageTag(t *testing.T) {
	fake, _, _, err := runCreate(t, "--name dev --edition community --no-store-credential")
	require.NoError(t, err)
	argv := runArgv(t, fake)

	for _, a := range argv {
		assert.False(t, strings.HasPrefix(a, "NEO4J_ACCEPT_LICENSE_AGREEMENT"),
			"community edition must not pass NEO4J_ACCEPT_LICENSE_AGREEMENT; argv=%v", argv)
	}
	assert.True(t, containsPair(argv, "--label", LabelEdition+"=community"))
	assert.Equal(t, "neo4j:latest", argv[len(argv)-1], "community image must NOT carry -enterprise suffix")
}

// Regression guard for the explicit-version enterprise path: `--version 5.26
// --edition enterprise` MUST still produce `neo4j:5.26-enterprise`. Pins the
// versioned branch so a future refactor of the image-resolution block can't
// re-break it while fixing the `latest` case.
func TestCreate_EnterpriseExplicitVersion_UsesVersionEnterpriseSuffix(t *testing.T) {
	fake, _, _, err := runCreate(t, "--name dev --version 5.26 --edition enterprise --no-store-credential")
	require.NoError(t, err)
	argv := runArgv(t, fake)
	assert.Equal(t, "neo4j:5.26-enterprise", argv[len(argv)-1],
		"enterprise + explicit version must map to neo4j:<version>-enterprise")
}

// Pin the bug fix: `--version latest --edition enterprise` MUST resolve to
// the bare `neo4j:enterprise` tag because Docker Hub does NOT publish
// `neo4j:latest-enterprise`. This is the case the operator hit live.
func TestCreate_EnterpriseLatest_UsesBareEnterpriseTag(t *testing.T) {
	fake, _, _, err := runCreate(t, "--name dev --version latest --edition enterprise --no-store-credential")
	require.NoError(t, err)
	argv := runArgv(t, fake)
	assert.Equal(t, "neo4j:enterprise", argv[len(argv)-1],
		"enterprise + latest must map to bare neo4j:enterprise (Docker Hub does NOT publish neo4j:latest-enterprise)")
}

func TestCreate_EnterpriseAcceptLicense_UpgradesToYes(t *testing.T) {
	fake, _, _, err := runCreate(t, "--name dev --accept-license --no-store-credential")
	require.NoError(t, err)
	argv := runArgv(t, fake)
	assert.True(t, containsPair(argv, "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=yes"),
		"argv missing yes-licensed env: %v", argv)
	assert.False(t, containsPair(argv, "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval"),
		"argv must not retain the default eval value: %v", argv)
}

func TestCreate_ExplicitPassword_HonouredAndSurfaced(t *testing.T) {
	fake, cfg, stdout, err := runCreate(t, "--name dev --password mysecret --format json")
	require.NoError(t, err)
	argv := runArgv(t, fake)
	assert.True(t, containsPair(argv, "-e", "NEO4J_AUTH=neo4j/mysecret"))

	// JSON output carries the password verbatim.
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "mysecret", rows[0]["password"])

	cred, err := cfg.Credentials.Dbms.Get("dev")
	require.NoError(t, err)
	assert.Equal(t, "mysecret", cred.Password)
}

func TestCreate_GeneratedPassword_UsesRandSourceAndBase64URLEncoding(t *testing.T) {
	origRand := randSource
	randSource = constantReader{b: 0x10}
	defer func() { randSource = origRand }()

	expected := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x10}, generatedPasswordBytes))
	// Sanity: no padding, length matches 16 bytes → 22 characters.
	require.NotContains(t, expected, "=")
	require.Len(t, expected, 22)

	_, cfg, stdout, err := runCreate(t, "--name dev --format json")
	require.NoError(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, expected, rows[0]["password"], "rendered password must match base64 URL-safe of randSource bytes")

	cred, err := cfg.Credentials.Dbms.Get("dev")
	require.NoError(t, err)
	assert.Equal(t, expected, cred.Password)
}

func TestCreate_FormatJson_RendersDocumentedFields(t *testing.T) {
	origRand := randSource
	randSource = constantReader{b: 0x42}
	defer func() { randSource = origRand }()

	_, _, stdout, err := runCreate(t, "--name dev --bolt-port 7688 --http-port 7475 --format json")
	require.NoError(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	row := rows[0]

	for _, key := range []string{"name", "edition", "version", "bolt-port", "http-port", "uri", "username", "password"} {
		_, ok := row[key]
		assert.True(t, ok, "JSON output missing field %q (full row: %v)", key, row)
	}
	assert.Equal(t, "dev", row["name"])
	assert.Equal(t, "enterprise", row["edition"])
	assert.Equal(t, "latest", row["version"])
	assert.EqualValues(t, 7688, row["bolt-port"])
	assert.EqualValues(t, 7475, row["http-port"])
	assert.Equal(t, "neo4j://localhost:7688", row["uri"])
	assert.Equal(t, "neo4j", row["username"])
}

func TestCreate_InvalidEdition_ReturnsUsageError(t *testing.T) {
	fake, _, _, err := runCreate(t, "--name dev --edition foo --no-store-credential")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--edition")
	assert.Empty(t, fake.RunCalls, "docker run must not be invoked on invalid --edition")
}

func TestCreate_PortPreflight(t *testing.T) {
	tests := []struct {
		name            string
		args            string
		occupied        []int
		wantErr         bool
		wantErrContains []string
		// wantRun is the expected number of recorded docker run invocations.
		// All conflict cases must be 0 — pre-flight runs BEFORE any docker
		// side effect (REQ-F-013).
		wantRun int
	}{
		{
			name:    "both ports free succeeds",
			args:    "--name dev --no-store-credential",
			wantRun: 1,
		},
		{
			name:            "bolt port occupied surfaces --bolt-port hint",
			args:            "--name dev --no-store-credential",
			occupied:        []int{7687},
			wantErr:         true,
			wantErrContains: []string{"port 7687", "--bolt-port"},
			wantRun:         0,
		},
		{
			name:            "http port occupied surfaces --http-port hint",
			args:            "--name dev --no-store-credential",
			occupied:        []int{7474},
			wantErr:         true,
			wantErrContains: []string{"port 7474", "--http-port"},
			wantRun:         0,
		},
		{
			name:            "equal ports rejected before any Listen",
			args:            "--name dev --bolt-port 9999 --http-port 9999 --no-store-credential",
			wantErr:         true,
			wantErrContains: []string{"--bolt-port", "--http-port", "9999"},
			wantRun:         0,
		},
		{
			name:            "custom bolt port occupied is named correctly",
			args:            "--name dev --bolt-port 9999 --http-port 9000 --no-store-credential",
			occupied:        []int{9999},
			wantErr:         true,
			wantErrContains: []string{"port 9999", "--bolt-port"},
			wantRun:         0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake, _, _, err := runCreateWithOccupiedPorts(t, tc.args, tc.occupied...)
			if tc.wantErr {
				require.Error(t, err)
				for _, sub := range tc.wantErrContains {
					assert.Contains(t, err.Error(), sub)
				}
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, fake.RunCalls, tc.wantRun)
		})
	}
}

func TestCreate_PortPreflight_ProbesBoltThenHTTP_OnSuccess(t *testing.T) {
	// Verifies the listener factory is invoked exactly twice and in the
	// documented order (bolt-port first, then http-port) on the happy path.
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	fake := newFakeDockerClient()
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	probed := stubListenerFactory(t)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"create", "--name", "dev", "--bolt-port", "7688", "--http-port", "7475", "--no-store-credential"})

	require.NoError(t, cmd.Execute())
	require.Equal(t, []int{7688, 7475}, *probed, "pre-flight must probe --bolt-port then --http-port")
	require.Len(t, fake.RunCalls, 1, "docker run must execute exactly once after a clean pre-flight")
}

// runCreateWithSeed is the same as runCreateWithOccupiedPorts but lets the
// caller pre-populate the fake docker client's PsAll-returned names AND the
// dbms credential store before the command runs. Used by name-collision tests
// (REQ-F-014) so they can assert auto-suffix behaviour against deterministic
// state without leaking real docker / credential I/O.
func runCreateWithSeed(t *testing.T, args string, dockerNames []string, credentialNames []string) (*fakeDockerClient, *clicfg.Config, string, string, error) {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	// Seed dbms credentials. Each call to Add gives the credential a fresh
	// URI so the entries are distinguishable but otherwise meaningless to
	// the collision check (which only inspects names).
	for i, n := range credentialNames {
		require.NoError(t, cfg.Credentials.Dbms.Add(n, "neo4j", "pw", "neo4j", fmt.Sprintf("neo4j://localhost:%d", 7700+i)))
	}

	fake := newFakeDockerClient()
	for _, n := range dockerNames {
		fake.PsEntries = append(fake.PsEntries, PsEntry{Names: n})
	}
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	stubListenerFactory(t)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"create"}, argv...))

	execErr := cmd.Execute()
	return fake, cfg, out.String(), errBuf.String(), execErr
}

func TestCreate_NameCollision_NoCollision_UsesRequestedName(t *testing.T) {
	fake, cfg, stdout, stderr, err := runCreateWithSeed(t, "--name dev --format json", nil, nil)
	require.NoError(t, err)

	// stderr must NOT carry an info: line because the requested name was free.
	assert.NotContains(t, stderr, "info: name")

	// Container created with the requested name; credential stored under it.
	argv := runArgv(t, fake)
	assert.True(t, containsPair(argv, "--name", "dev"), "argv missing --name dev: %v", argv)
	_, getErr := cfg.Credentials.Dbms.Get("dev")
	assert.NoError(t, getErr)

	// Rendered output reflects the chosen name.
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "dev", rows[0]["name"])
}

func TestCreate_NameCollision_DockerOnly_Suffixes(t *testing.T) {
	// docker already has a container called "dev"; credentials are empty.
	fake, cfg, stdout, stderr, err := runCreateWithSeed(t, "--name dev --format json", []string{"dev"}, nil)
	require.NoError(t, err)

	// stderr names both the requested and the chosen name.
	assert.Contains(t, stderr, `info: name "dev" already in use; using "dev-1"`)

	// Container created with the suffixed name; credential stored under the
	// suffixed name.
	argv := runArgv(t, fake)
	assert.True(t, containsPair(argv, "--name", "dev-1"), "argv missing --name dev-1: %v", argv)
	_, getErr := cfg.Credentials.Dbms.Get("dev-1")
	assert.NoError(t, getErr)
	_, getErrOriginal := cfg.Credentials.Dbms.Get("dev")
	assert.Error(t, getErrOriginal, "no credential should be stored under the original requested name")

	// Rendered output mirrors the chosen name.
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "dev-1", rows[0]["name"])
}

func TestCreate_NameCollision_CredentialOnly_Suffixes(t *testing.T) {
	// docker is empty; a dbms credential called "dev" already exists.
	fake, cfg, _, stderr, err := runCreateWithSeed(t, "--name dev", nil, []string{"dev"})
	require.NoError(t, err)

	assert.Contains(t, stderr, `info: name "dev" already in use; using "dev-1"`)
	argv := runArgv(t, fake)
	assert.True(t, containsPair(argv, "--name", "dev-1"), "argv missing --name dev-1: %v", argv)
	_, getErr := cfg.Credentials.Dbms.Get("dev-1")
	assert.NoError(t, getErr)
}

func TestCreate_NameCollision_Cascading_WalksPastDockerAndCredential(t *testing.T) {
	// Existing docker containers: "dev", "dev-2"; existing credential: "dev-1".
	// Expected chosen name: "dev-3".
	fake, cfg, stdout, stderr, err := runCreateWithSeed(t,
		"--name dev --format json",
		[]string{"dev", "dev-2"},
		[]string{"dev-1"},
	)
	require.NoError(t, err)

	assert.Contains(t, stderr, `info: name "dev" already in use; using "dev-3"`)
	argv := runArgv(t, fake)
	assert.True(t, containsPair(argv, "--name", "dev-3"), "argv missing --name dev-3: %v", argv)
	_, getErr := cfg.Credentials.Dbms.Get("dev-3")
	assert.NoError(t, getErr)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "dev-3", rows[0]["name"])
}

func TestCreate_NameCollision_All99Taken_ReturnsUsageError(t *testing.T) {
	// docker has "dev"; credentials hold "dev-1" through "dev-99". No free
	// suffix in the documented range → usage error, no docker run executed.
	credentialNames := make([]string, 0, maxNameSuffix)
	for i := 1; i <= maxNameSuffix; i++ {
		credentialNames = append(credentialNames, fmt.Sprintf("dev-%d", i))
	}
	fake, _, _, _, err := runCreateWithSeed(t, "--name dev --no-store-credential", []string{"dev"}, credentialNames)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dev-1")
	assert.Contains(t, err.Error(), fmt.Sprintf("dev-%d", maxNameSuffix))
	assert.Empty(t, fake.RunCalls, "docker run must not execute when no free name is available")
}

// withCreateWaitProbe swaps the package-level waitForBoltFn seam for the
// duration of the test and restores it on cleanup. Tests pass the prober they
// want exercised; counting/inspection happens inside the prober via captured
// pointers. The companion waitTimeout/pollInterval shrinking lives in the
// caller — keep this helper single-purpose.
func withCreateWaitProbe(t *testing.T, prober func(ctx context.Context, uri, user, pass string, timeout time.Duration) error) {
	t.Helper()
	orig := waitForBoltFn
	waitForBoltFn = prober
	t.Cleanup(func() { waitForBoltFn = orig })
}

// withShortWaitTimeout shrinks waitTimeout for the duration of the test so
// `--wait` timeout cases don't burn the production 60s budget. Restored on
// cleanup so other tests still see the production value.
func withShortWaitTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	orig := waitTimeout
	waitTimeout = d
	t.Cleanup(func() { waitTimeout = orig })
}

func TestCreate_Wait_HappyPath_SucceedsAndNarrates(t *testing.T) {
	// The fake prober returns nil on the first call → WaitForBolt-equivalent
	// returns immediately. The seam is the entire waitForBoltFn (not the
	// inner probeBoltFn) so we never enter the readiness polling loop here.
	var calls int32
	withCreateWaitProbe(t, func(_ context.Context, uri, user, pass string, _ time.Duration) error {
		atomic.AddInt32(&calls, 1)
		// Probe must target the localhost URI built from --bolt-port and the
		// neo4j user with the generated password — verifies create.go wires
		// the right arguments through.
		assert.Equal(t, "neo4j://localhost:7687", uri)
		assert.Equal(t, "neo4j", user)
		assert.NotEmpty(t, pass)
		return nil
	})

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	fake := newFakeDockerClient()
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	stubListenerFactory(t)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"create", "--name", "dev", "--wait"})

	require.NoError(t, cmd.Execute())

	// Prober called exactly once.
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))

	// Stderr narration is the documented single line, including the bolt port.
	assert.Contains(t, errBuf.String(), "info: waiting for Bolt on localhost:7687...")

	// Container created exactly once; --wait NEVER triggers a tear-down.
	assert.Len(t, fake.RunCalls, 1)
	assert.Empty(t, fake.StopCalls, "successful --wait must not stop the container")
	assert.Empty(t, fake.RemoveForceCalls, "successful --wait must not remove the container")

	// Credential persisted as usual on the wait happy-path.
	_, getErr := cfg.Credentials.Dbms.Get("dev")
	assert.NoError(t, getErr)
}

func TestCreate_Wait_Timeout_ReturnsErrorAndLeavesContainerRunning(t *testing.T) {
	// Shrink the production budget so the test runs in <100ms even when the
	// CI host is loaded.
	withShortWaitTimeout(t, 50*time.Millisecond)

	// The fake prober echoes whatever WaitForBolt would return on timeout —
	// we don't go through the real polling loop here, so emit a clierr-style
	// error directly and assert it round-trips back through create.go.
	const timeoutMsg = "container started but Bolt did not become ready within 50ms; check 'docker logs <name>'"
	var calls int32
	withCreateWaitProbe(t, func(_ context.Context, _, _, _ string, _ time.Duration) error {
		atomic.AddInt32(&calls, 1)
		return errors.New(timeoutMsg)
	})

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	fake := newFakeDockerClient()
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	stubListenerFactory(t)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"create", "--name", "dev", "--wait"})

	execErr := cmd.Execute()
	require.Error(t, execErr)
	assert.Contains(t, execErr.Error(), "Bolt did not become ready")

	// Stderr still carries the pre-poll narration so the operator knows we
	// committed to waiting before the failure.
	assert.Contains(t, errBuf.String(), "info: waiting for Bolt on localhost:7687...")

	// Probe was exercised exactly once.
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))

	// Container is NOT torn down on timeout (REQ-F-018 contract — the
	// partially-started Neo4j may still finish booting after the CLI returns).
	assert.Len(t, fake.RunCalls, 1)
	assert.Empty(t, fake.StopCalls, "timeout must not stop the container")
	assert.Empty(t, fake.RemoveForceCalls, "timeout must not remove the container")
}

func TestCreate_Wait_AwaitAlias_StillWorks(t *testing.T) {
	// The flags.RegisterWait helper also registers the deprecated --await
	// alias (CLI-87). Make sure passing --await reaches the same code path
	// so users with stale muscle memory don't silently miss the readiness
	// probe. The alias prints a cobra deprecation notice to stderr; we
	// only assert behavioural equivalence here.
	var calls int32
	withCreateWaitProbe(t, func(_ context.Context, _, _, _ string, _ time.Duration) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	fake := newFakeDockerClient()
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	stubListenerFactory(t)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"create", "--name", "dev", "--await"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestCreate_NoWait_DoesNotInvokeBoltProbe(t *testing.T) {
	// Without --wait, the readiness probe must never run — create.go is
	// strictly fire-and-forget.
	var calls int32
	withCreateWaitProbe(t, func(_ context.Context, _, _, _ string, _ time.Duration) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	fake, _, _, err := runCreate(t, "--name dev")
	require.NoError(t, err)
	require.Len(t, fake.RunCalls, 1)
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls), "probe must not run without --wait")
}

// runCreateForEphemeral is the test rig for --ephemeral / --env-out-file cases.
// It mirrors runCreate but exposes stderr (where the env-file write narration
// lands) and returns the cfg.Aura.Fs() handle so tests can stat / read any
// file written via the afero seam.
func runCreateForEphemeral(t *testing.T, args string) (*fakeDockerClient, *clicfg.Config, afero.Fs, string, string, error) {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	fake := newFakeDockerClient()
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	stubListenerFactory(t)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"create"}, argv...))

	execErr := cmd.Execute()
	return fake, cfg, cfg.Aura.Fs(), out.String(), errBuf.String(), execErr
}

func TestCreate_Ephemeral_HappyPath_EmitsEnvBlobAndSkipsCredential(t *testing.T) {
	origRand := randSource
	randSource = constantReader{b: 0xCD}
	defer func() { randSource = origRand }()
	expectedPassword := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xCD}, generatedPasswordBytes))

	fake, cfg, _, stdout, _, err := runCreateForEphemeral(t, "--name tmp --ephemeral")
	require.NoError(t, err)

	// docker run argv carries --rm and label ephemeral=true.
	argv := runArgv(t, fake)
	assert.Contains(t, argv, "--rm", "ephemeral must add --rm to docker run argv: %v", argv)
	assert.True(t, containsPair(argv, "--label", LabelEphemeral+"=true"),
		"argv missing ephemeral=true label: %v", argv)
	// Sanity: ephemeral=false label must NOT be present.
	assert.False(t, containsPair(argv, "--label", LabelEphemeral+"=false"),
		"argv must not carry ephemeral=false when --ephemeral: %v", argv)

	// No credential was stored — ephemeral leaves no on-disk footprint.
	assert.Empty(t, cfg.Credentials.Dbms.List(), "ephemeral must not persist a dbms credential")

	// Stdout carries exactly the documented env blob (header + four NEO4J_*
	// lines, in the documented order), in plain text — NOT through the
	// table/JSON renderer.
	expectedBlob := fmt.Sprintf(
		"# neo4j-cli docker — tmp @ neo4j:enterprise\nNEO4J_URI=neo4j://localhost:7687\nNEO4J_USERNAME=neo4j\nNEO4J_PASSWORD=%s\nNEO4J_DATABASE=neo4j\n",
		expectedPassword,
	)
	assert.Equal(t, expectedBlob, stdout, "stdout must be the literal env-file blob")
}

func TestCreate_Ephemeral_EnvOutFile_WritesFileAndStaysSilent(t *testing.T) {
	origRand := randSource
	randSource = constantReader{b: 0xEF}
	defer func() { randSource = origRand }()
	expectedPassword := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xEF}, generatedPasswordBytes))

	envPath := "/tmp/n.env"
	fake, cfg, fs, stdout, stderr, err := runCreateForEphemeral(t,
		"--name tmp --ephemeral --env-out-file "+envPath)
	require.NoError(t, err)

	// --rm + ephemeral label still applied.
	argv := runArgv(t, fake)
	assert.Contains(t, argv, "--rm")
	assert.True(t, containsPair(argv, "--label", LabelEphemeral+"=true"))

	// No credential persisted.
	assert.Empty(t, cfg.Credentials.Dbms.List())

	// Stdout MUST be empty so the caller can pipe.
	assert.Empty(t, stdout, "--env-out-file must keep stdout silent for piping")

	// Stderr carries the one-line confirmation pointing at the path.
	assert.Contains(t, stderr, "info: wrote credentials to "+envPath)

	// File written to the in-memory fs with mode 0600 and the documented blob.
	info, statErr := fs.Stat(envPath)
	require.NoError(t, statErr, "env-file must exist on the in-memory fs")
	assert.Equal(t, "-rw-------", info.Mode().Perm().String(),
		"env-file must be mode 0600 (REQ-NF-004); got %s", info.Mode().Perm())

	contents, readErr := afero.ReadFile(fs, envPath)
	require.NoError(t, readErr)
	expectedBlob := fmt.Sprintf(
		"# neo4j-cli docker — tmp @ neo4j:enterprise\nNEO4J_URI=neo4j://localhost:7687\nNEO4J_USERNAME=neo4j\nNEO4J_PASSWORD=%s\nNEO4J_DATABASE=neo4j\n",
		expectedPassword,
	)
	assert.Equal(t, expectedBlob, string(contents))
}

func TestCreate_EnvOutFileWithoutEphemeral_UsageError(t *testing.T) {
	fake, _, _, _, _, err := runCreateForEphemeral(t, "--name dev --env-out-file /tmp/n.env")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--env-out-file")
	assert.Contains(t, err.Error(), "--ephemeral")
	assert.Empty(t, fake.RunCalls, "docker run must not execute when --env-out-file is misused")
}

func TestCreate_EphemeralWithNoStoreCredential_UsageError(t *testing.T) {
	fake, _, _, _, _, err := runCreateForEphemeral(t, "--name tmp --ephemeral --no-store-credential")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--no-store-credential")
	assert.Contains(t, err.Error(), "--ephemeral")
	assert.Empty(t, fake.RunCalls, "docker run must not execute on incompatible-flag usage error")
}

func TestCreate_Ephemeral_HonoursExplicitPassword(t *testing.T) {
	// --ephemeral with --password must still surface that password verbatim
	// in the env-file blob (operators do this to pre-share a known secret).
	_, _, _, stdout, _, err := runCreateForEphemeral(t, "--name tmp --ephemeral --password mysecret")
	require.NoError(t, err)
	assert.Contains(t, stdout, "NEO4J_PASSWORD=mysecret\n")
	// Username and database are always the documented defaults.
	assert.Contains(t, stdout, "NEO4J_USERNAME=neo4j\n")
	assert.Contains(t, stdout, "NEO4J_DATABASE=neo4j\n")
	assert.Contains(t, stdout, "NEO4J_URI=neo4j://localhost:7687\n")
}

func TestCreate_Ephemeral_EnvOutFile_ChmodsPreexistingFileTo0600(t *testing.T) {
	// Defense-in-depth (REQ-NF-004): OpenFile's mode arg is honoured only on
	// create. If --env-out-file points at a path that already exists with a
	// permissive mode, the writeEnvFile call must Chmod the file down to
	// 0o600 after the write so the credential blob never lands on disk in
	// a world-readable state. Pre-seed a 0o644 file at the target path and
	// assert the post-write mode is 0o600.
	envPath := "/tmp/preexisting.env"

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	// Pre-seed the file with a permissive mode so we can verify Chmod ran.
	require.NoError(t, afero.WriteFile(cfg.Aura.Fs(), envPath, []byte("stale\n"), 0o644))
	pre, err := cfg.Aura.Fs().Stat(envPath)
	require.NoError(t, err)
	require.Equal(t, "-rw-r--r--", pre.Mode().Perm().String(), "preflight: file must be seeded at 0o644")

	fake := newFakeDockerClient()
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	stubListenerFactory(t)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"create", "--name", "tmp", "--ephemeral", "--env-out-file", envPath})

	require.NoError(t, cmd.Execute())

	info, statErr := cfg.Aura.Fs().Stat(envPath)
	require.NoError(t, statErr)
	assert.Equal(t, "-rw-------", info.Mode().Perm().String(),
		"pre-existing env-file must be chmod'd to 0o600 (REQ-NF-004); got %s", info.Mode().Perm())
}

func TestCreate_Ephemeral_EnvBlobLineOrder(t *testing.T) {
	// REQ-F-017 fixes the order of the four NEO4J_* lines (URI → USERNAME →
	// PASSWORD → DATABASE) right after the header. Assert literally so a
	// reorder doesn't slip past review.
	_, _, _, stdout, _, err := runCreateForEphemeral(t, "--name tmp --ephemeral --password p")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	require.Len(t, lines, 5, "blob must have header + four NEO4J_* lines; got %v", lines)
	assert.True(t, strings.HasPrefix(lines[0], "# neo4j-cli docker —"))
	assert.True(t, strings.HasPrefix(lines[1], "NEO4J_URI="))
	assert.True(t, strings.HasPrefix(lines[2], "NEO4J_USERNAME="))
	assert.True(t, strings.HasPrefix(lines[3], "NEO4J_PASSWORD="))
	assert.True(t, strings.HasPrefix(lines[4], "NEO4J_DATABASE="))
}

func TestCreate_PortPreflight_EqualPorts_SkipsListenCalls(t *testing.T) {
	// Equal-ports check must fire BEFORE any Listen call so the operator
	// sees the conflict instead of a misleading "port in use" error.
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	fake := newFakeDockerClient()
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	probed := stubListenerFactory(t)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"create", "--name", "dev", "--bolt-port", "9999", "--http-port", "9999", "--no-store-credential"})

	err = cmd.Execute()
	require.Error(t, err)
	assert.Empty(t, *probed, "no port may be probed when --bolt-port and --http-port are equal")
	assert.Empty(t, fake.RunCalls, "docker run must not be invoked on equal-port usage error")
}

// renameFailFs wraps an afero.Fs and forces Rename to return a sentinel
// error. Used to drive the writeEnvFile rename-failure cleanup path
// without needing a real disk failure.
type renameFailFs struct {
	afero.Fs
	err error
}

func (f *renameFailFs) Rename(oldname, newname string) error { return f.err }

// chmodFailFs wraps an afero.Fs and forces Chmod to return a sentinel
// error. Used to drive the writeEnvFile chmod-failure cleanup path.
type chmodFailFs struct {
	afero.Fs
	err error
}

func (f *chmodFailFs) Chmod(name string, mode os.FileMode) error { return f.err }

// listTempLeftovers returns all `.neo4j-cli-env-*` filenames currently
// present under dir. Used by the rename/chmod failure tests to assert the
// cleanup actually removed the temp file (zero leftover entries).
func listTempLeftovers(t *testing.T, fs afero.Fs, dir string) []string {
	t.Helper()
	entries, err := afero.ReadDir(fs, dir)
	if err != nil {
		// Dir does not exist → no leftovers possible; treat as empty.
		return nil
	}
	var leftovers []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".neo4j-cli-env-") {
			leftovers = append(leftovers, e.Name())
		}
	}
	return leftovers
}

func TestWriteEnvFile_ReplacesPreExistingSymlink_OsFs(t *testing.T) {
	// afero.MemMapFs does not support symlinks, so the symlink-replacement
	// path is exercised against a real afero.NewOsFs on t.TempDir(). The
	// temp dir is isolated to this test and cleaned up automatically.
	dir := t.TempDir()
	envPath := filepath.Join(dir, "n.env")
	otherPath := filepath.Join(dir, "other.txt")

	const otherOriginal = "do-not-touch\n"
	require.NoError(t, os.WriteFile(otherPath, []byte(otherOriginal), 0o644))

	// Plant a symlink at envPath pointing at otherPath. A naive
	// OpenFile(envPath, O_CREATE|O_TRUNC) would follow this and clobber
	// otherPath's contents with the credential blob — the very class of
	// attack the temp+rename strategy defends against.
	require.NoError(t, os.Symlink(otherPath, envPath))

	fs := afero.NewOsFs()
	const blob = "# neo4j-cli docker — tmp @ neo4j:enterprise\nNEO4J_URI=neo4j://localhost:7687\nNEO4J_USERNAME=neo4j\nNEO4J_PASSWORD=p\nNEO4J_DATABASE=neo4j\n"
	require.NoError(t, writeEnvFile(fs, envPath, blob))

	// envPath must now be a REGULAR file (symlink replaced) with mode 0600
	// and the documented blob.
	info, err := os.Lstat(envPath)
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular(),
		"env-file path must be a regular file after writeEnvFile; got mode %s", info.Mode())
	assert.Zero(t, info.Mode()&os.ModeSymlink,
		"env-file path must NOT be a symlink anymore; got mode %s", info.Mode())
	// POSIX-only mode-bit assertion: Windows os.Chmod only honors the
	// read-only bit, so 0o600 lands as 0o666 there. The temp+rename strategy
	// is still effective on Windows (the symlink-follow window is closed by
	// using a fresh O_EXCL temp path); the mode bit is just an OS-level
	// gotcha. AGENTS.md "Windows CI Gotchas" — guard the assertion.
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
			"env-file must be mode 0600 (REQ-NF-004); got %s", info.Mode().Perm())
	}

	contents, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Equal(t, blob, string(contents))

	// Crucially: the original symlink target was NOT followed and NOT
	// clobbered. Anyone who could trick neo4j-cli into writing to a
	// symlinked path can no longer commandeer the credential blob into
	// arbitrary locations.
	otherContents, err := os.ReadFile(otherPath)
	require.NoError(t, err)
	assert.Equal(t, otherOriginal, string(otherContents),
		"symlink target must be untouched — the symlink was replaced, not followed")

	// No `.neo4j-cli-env-*` leftover in the dir.
	assert.Empty(t, listTempLeftovers(t, fs, dir),
		"no temp leftovers permitted on a successful write")
}

func TestWriteEnvFile_RenameFailure_RemovesTempFile(t *testing.T) {
	// Wrap the in-memory fs so Rename fails deterministically.
	mem := afero.NewMemMapFs()
	require.NoError(t, mem.MkdirAll("/tmp", 0o755))

	sentinel := errors.New("simulated rename failure")
	fs := &renameFailFs{Fs: mem, err: sentinel}

	const path = "/tmp/n.env"
	err := writeEnvFile(fs, path, "blob")
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel, "writeEnvFile must wrap the underlying Rename error")
	assert.Contains(t, err.Error(), "rename env-file to "+path)

	// Cleanup MUST have removed the temp file. Walk the dir on the
	// underlying memfs (rename failed, so the final path was never
	// created either — both assertions hold).
	leftovers := listTempLeftovers(t, mem, "/tmp")
	assert.Empty(t, leftovers, "no temp file may remain after a rename failure; got %v", leftovers)

	// Final path was never created (rename failed).
	_, statErr := mem.Stat(path)
	assert.True(t, os.IsNotExist(statErr) || statErr != nil,
		"final env-file path must not exist after a rename failure; got stat err %v", statErr)
}

func TestWriteEnvFile_ChmodFailure_RemovesTempFile(t *testing.T) {
	mem := afero.NewMemMapFs()
	require.NoError(t, mem.MkdirAll("/tmp", 0o755))

	sentinel := errors.New("simulated chmod failure")
	fs := &chmodFailFs{Fs: mem, err: sentinel}

	const path = "/tmp/n.env"
	err := writeEnvFile(fs, path, "blob")
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "chmod temp env-file")

	leftovers := listTempLeftovers(t, mem, "/tmp")
	assert.Empty(t, leftovers, "no temp file may remain after a chmod failure; got %v", leftovers)

	_, statErr := mem.Stat(path)
	assert.True(t, os.IsNotExist(statErr) || statErr != nil,
		"final env-file path must not exist after a chmod failure; got stat err %v", statErr)
}

func TestWriteEnvFile_MissingDir_Errors(t *testing.T) {
	// Target directory does not exist → afero.TempFile fails, we surface
	// the wrapped error mentioning the dir.
	//
	// Uses OsFs against t.TempDir(): MemMapFs's OpenFile creates files in
	// non-existent directories implicitly (a memfs quirk that does NOT
	// reflect production OsFs behaviour), so a missing-dir test on memfs
	// would always succeed and miss the real check.
	dir := filepath.Join(t.TempDir(), "no-such-subdir")
	envPath := filepath.Join(dir, "n.env")

	fs := afero.NewOsFs()
	err := writeEnvFile(fs, envPath, "blob")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create temp env-file in "+dir)
}

// withHomeDirFn swaps the package-level homeDirFn seam for the duration of
// the test so `--data-dir ~/...` tests can assert against a deterministic
// home directory without depending on the host's actual HOME.
func withHomeDirFn(t *testing.T, home string) {
	t.Helper()
	orig := homeDirFn
	homeDirFn = func() (string, error) { return home, nil }
	t.Cleanup(func() { homeDirFn = orig })
}

// containsVolume returns true if argv contains a `-v <host>:<container>`
// consecutive pair anywhere. Used to assert volume-mount argv pairs.
func containsVolume(argv []string, host, container string) bool {
	return containsPair(argv, "-v", host+":"+container)
}

// countOccurrences returns the number of argv slots whose value equals the
// supplied needle. Used to assert that each `-v` arg appears exactly once
// when multiple volume flags are combined.
func countOccurrences(argv []string, needle string) int {
	n := 0
	for _, a := range argv {
		if a == needle {
			n++
		}
	}
	return n
}

func TestCreate_DataDir_HappyPath_AddsVolumeArgAndCreatesDir(t *testing.T) {
	// hostPath sourced via t.TempDir() so it's OS-native and survives
	// expandHostPath idempotently on Windows (where filepath.Abs prepends
	// drive + uses backslashes). AGENTS.md "Windows CI Gotchas".
	hostPath := filepath.Join(t.TempDir(), "n4j-data-happy")
	fake, _, fs, _, stderr, err := runCreateForEphemeral(t,
		"--name dev --no-store-credential --data-dir "+shlexQuote(hostPath))
	require.NoError(t, err)

	argv := runArgv(t, fake)
	assert.True(t, containsVolume(argv, hostPath, "/data"),
		"argv missing -v %s:/data: %v", hostPath, argv)

	// Directory was created on the memfs (didn't exist before) at 0o755.
	info, statErr := fs.Stat(hostPath)
	require.NoError(t, statErr, "--data-dir target must exist after RunE")
	assert.True(t, info.IsDir(), "--data-dir target must be a directory")
	// POSIX-only mode-bit assertion: Windows MkdirAll mode handling differs.
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(),
			"--data-dir target must be mode 0o755; got %s", info.Mode().Perm())
	}

	// Stderr carries the documented info line.
	assert.Contains(t, stderr, "info: created host directory "+hostPath)
}

func TestCreate_LogsDir_HappyPath_AddsVolumeArg(t *testing.T) {
	hostPath := filepath.Join(t.TempDir(), "n4j-logs-happy")
	fake, _, _, _, _, err := runCreateForEphemeral(t,
		"--name dev --no-store-credential --logs-dir "+shlexQuote(hostPath))
	require.NoError(t, err)

	argv := runArgv(t, fake)
	assert.True(t, containsVolume(argv, hostPath, "/logs"),
		"argv missing -v %s:/logs: %v", hostPath, argv)
}

func TestCreate_ImportDir_HappyPath_AddsVolumeArg(t *testing.T) {
	hostPath := filepath.Join(t.TempDir(), "n4j-import-happy")
	fake, _, _, _, _, err := runCreateForEphemeral(t,
		"--name dev --no-store-credential --import-dir "+shlexQuote(hostPath))
	require.NoError(t, err)

	argv := runArgv(t, fake)
	assert.True(t, containsVolume(argv, hostPath, "/import"),
		"argv missing -v %s:/import: %v", hostPath, argv)
}

func TestCreate_AllVolumeFlags_AppendsThreeUniqueMounts(t *testing.T) {
	base := t.TempDir()
	data := filepath.Join(base, "data")
	logs := filepath.Join(base, "logs")
	imp := filepath.Join(base, "import")
	fake, _, _, _, _, err := runCreateForEphemeral(t,
		fmt.Sprintf("--name dev --no-store-credential --data-dir %s --logs-dir %s --import-dir %s",
			shlexQuote(data), shlexQuote(logs), shlexQuote(imp)))
	require.NoError(t, err)

	argv := runArgv(t, fake)
	assert.True(t, containsVolume(argv, data, "/data"))
	assert.True(t, containsVolume(argv, logs, "/logs"))
	assert.True(t, containsVolume(argv, imp, "/import"))

	// Each `-v` value must appear exactly once; total `-v` slots == 3.
	assert.Equal(t, 3, countOccurrences(argv, "-v"),
		"expected exactly three -v flags; got argv=%v", argv)
}

func TestCreate_NoVolumeFlags_NoVolumeArgs(t *testing.T) {
	// Sanity / regression guard: when no volume flag is set, there must be
	// no `-v` argument in argv. Earlier happy-path tests assert this
	// indirectly; pin it explicitly.
	fake, _, _, err := runCreate(t, "--name dev --no-store-credential")
	require.NoError(t, err)
	argv := runArgv(t, fake)
	assert.Zero(t, countOccurrences(argv, "-v"),
		"no volume flag set, no -v expected in argv: %v", argv)
}

func TestCreate_DataDir_TildeExpansion(t *testing.T) {
	// Use t.TempDir() for the fake home so the resolved path is OS-native
	// (Windows: filepath.Abs prepends drive + backslashes).
	home := t.TempDir()
	withHomeDirFn(t, home)
	wantPath := filepath.Join(home, "foo")

	fake, _, fs, _, _, err := runCreateForEphemeral(t,
		"--name dev --no-store-credential --data-dir ~/foo")
	require.NoError(t, err)

	argv := runArgv(t, fake)
	assert.True(t, containsVolume(argv, wantPath, "/data"),
		"argv missing tilde-expanded -v: %v", argv)

	info, statErr := fs.Stat(wantPath)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestCreate_DataDir_EnvVarExpansion(t *testing.T) {
	envDir := filepath.Join(t.TempDir(), "n4j-from-env")
	t.Setenv("NEO4J_TEST_DIR", envDir)

	fake, _, fs, _, _, err := runCreateForEphemeral(t,
		"--name dev --no-store-credential --data-dir $NEO4J_TEST_DIR")
	require.NoError(t, err)

	argv := runArgv(t, fake)
	assert.True(t, containsVolume(argv, envDir, "/data"),
		"argv missing env-expanded -v: %v", argv)
	_, statErr := fs.Stat(envDir)
	require.NoError(t, statErr)
}

func TestCreate_DataDir_PreexistingDir_NoCreatedInfoLine(t *testing.T) {
	hostPath := filepath.Join(t.TempDir(), "n4j-data-existing")
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	// Pre-create the directory so the mkdir branch must be a no-op.
	require.NoError(t, fs.MkdirAll(hostPath, 0o755))

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	fake := newFakeDockerClient()
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })
	stubListenerFactory(t)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"create", "--name", "dev", "--no-store-credential", "--data-dir", hostPath})
	require.NoError(t, cmd.Execute())

	argv := runArgv(t, fake)
	assert.True(t, containsVolume(argv, hostPath, "/data"))
	assert.NotContains(t, errBuf.String(), "info: created host directory",
		"pre-existing dir must NOT emit the created info line; stderr=%q", errBuf.String())
}

func TestCreate_VolumeFlag_EphemeralIncompatible(t *testing.T) {
	tests := []struct {
		name string
		flag string // flag name (without leading --)
		args string
	}{
		{"data-dir", "data-dir", "--name dev --ephemeral --data-dir /tmp/x"},
		{"logs-dir", "logs-dir", "--name dev --ephemeral --logs-dir /tmp/x"},
		{"import-dir", "import-dir", "--name dev --ephemeral --import-dir /tmp/x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake, _, _, _, _, err := runCreateForEphemeral(t, tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--"+tc.flag)
			assert.Contains(t, err.Error(), "--ephemeral")
			assert.Empty(t, fake.RunCalls,
				"docker run must not execute when --%s + --ephemeral collide", tc.flag)
		})
	}
}

func TestExpandHostPath(t *testing.T) {
	// Drive expandHostPath directly to cover the unit (independent of the
	// cobra flow). The seam-based tests above exercise the full pipeline;
	// these pin the helper contract.
	//
	// Expected values are computed via filepath.Abs/Join so the test is
	// cross-platform: on Windows filepath.Abs prepends the current drive
	// and converts to backslashes (AGENTS.md "Windows CI Gotchas"). Use
	// t.TempDir() to source platform-native absolute paths for the home
	// and env-var-pointed dirs; they're never actually written to.
	home := t.TempDir()
	exampleDir := t.TempDir()
	withHomeDirFn(t, home)
	t.Setenv("EXAMPLE_DIR", exampleDir)

	// Build a platform-appropriate absolute path the helper should accept
	// unchanged (after filepath.Abs/Clean): "/var/data" on POSIX,
	// "<drive>:\var\data" on Windows.
	absoluteIn := filepath.Join(string(filepath.Separator), "var", "data")
	absoluteWant, err := filepath.Abs(absoluteIn)
	require.NoError(t, err)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"tilde alone", "~", home},
		{"tilde slash", "~/x", filepath.Join(home, "x")},
		{"env var", "$EXAMPLE_DIR/x", filepath.Join(exampleDir, "x")},
		{"braced env var", "${EXAMPLE_DIR}/x", filepath.Join(exampleDir, "x")},
		{"absolute unchanged", absoluteIn, absoluteWant},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandHostPath(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCreate_DataDir_OsFs_MkdirModeIs0755(t *testing.T) {
	// The memfs-backed mkdir tests already assert the 0o755 mode bit, but
	// memfs may not perfectly mirror OS mode semantics (umask etc). Drive
	// resolveHostDir directly against a real OsFs under t.TempDir() so we
	// catch a real-disk regression on CI even if memfs ever diverges.
	tmp := t.TempDir()
	hostPath := filepath.Join(tmp, "data")

	fs := afero.NewOsFs()
	// resolveHostDir only consumes cmd.ErrOrStderr() and the supplied fs —
	// no clicfg.Config is involved on this code path. Build a minimal cobra
	// command (no leaf body) so cmd.ErrOrStderr() resolves to our buffer.
	cmd := NewCmd(clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope))
	cmd.SetErr(bytes.NewBuffer(nil))

	resolved, err := resolveHostDir(cmd, fs, "data-dir", hostPath)
	require.NoError(t, err)
	assert.Equal(t, hostPath, resolved)

	info, err := os.Stat(hostPath)
	require.NoError(t, err)
	// Owner must have rwx, and at least group OR other must have r+x so
	// docker's container-side chown step can traverse the dir. The exact
	// "0o755" bit pattern depends on the host umask; assert behaviourally
	// rather than literally.
	mode := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0o700), mode&0o700, "owner rwx required, got %s", mode)
	assert.NotZero(t, mode&0o55, "group or other must have at least r+x for docker chown step, got %s", mode)
}

func TestCreate_Ephemeral_EnvOutFile_RenameFailure_NoTempLeftover(t *testing.T) {
	// End-to-end coverage: invoke `docker create --ephemeral --env-out-file`
	// through the cobra flow with a rename-failing fs and confirm the
	// command surfaces the error AND no temp file leaks into the target
	// directory. Mirrors TestWriteEnvFile_RenameFailure_RemovesTempFile
	// but through the full RunE path.
	mem, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	require.NoError(t, mem.MkdirAll("/tmp", 0o755))

	sentinel := errors.New("simulated rename failure")
	wrapped := &renameFailFs{Fs: mem, err: sentinel}
	cfg := clicfg.NewConfig(wrapped, "test", clicfg.GlobalScope)

	fake := newFakeDockerClient()
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	stubListenerFactory(t)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"create", "--name", "tmp", "--ephemeral", "--env-out-file", "/tmp/n.env"})

	execErr := cmd.Execute()
	require.Error(t, execErr)
	assert.ErrorIs(t, execErr, sentinel)

	assert.Empty(t, listTempLeftovers(t, mem, "/tmp"),
		"no temp file may remain after a rename failure in the cobra flow")
	_, statErr := mem.Stat("/tmp/n.env")
	assert.True(t, os.IsNotExist(statErr) || statErr != nil,
		"final env-file path must not exist after a rename failure; got stat err %v", statErr)
}
