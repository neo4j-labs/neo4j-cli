// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/test/utils/testfs"
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
	// Image is last; enterprise → -enterprise suffix.
	assert.Equal(t, "neo4j:latest-enterprise", argv[len(argv)-1])

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
