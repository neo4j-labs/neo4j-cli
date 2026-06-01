// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRedactArgs verifies that the argv-redaction helper used by execClient.run
// on the error-message path (REQ-NF-004) masks credential-bearing env values
// while preserving every other argv element verbatim, and never mutates its
// input. The helper exists because a non-zero docker exit echoes the full argv
// — including `-e NEO4J_AUTH=neo4j/<password>` — back to the user's terminal
// and any captured shell/CI logs.
func TestRedactArgs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "neo4j auth env masked",
			in:   []string{"run", "-d", "-e", "NEO4J_AUTH=neo4j/hunter2", "neo4j:latest"},
			want: []string{"run", "-d", "-e", "NEO4J_AUTH=<redacted>", "neo4j:latest"},
		},
		{
			name: "license env preserved (no AUTH or PASSWORD substring)",
			in:   []string{"run", "-d", "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval", "neo4j:latest-enterprise"},
			want: []string{"run", "-d", "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval", "neo4j:latest-enterprise"},
		},
		{
			name: "arbitrary password env masked via PASSWORD substring",
			in:   []string{"run", "-e", "MY_PASSWORD=hunter2"},
			want: []string{"run", "-e", "MY_PASSWORD=<redacted>"},
		},
		{
			name: "lowercase auth still masked",
			in:   []string{"run", "-e", "neo4j_auth=neo4j/x"},
			want: []string{"run", "-e", "neo4j_auth=<redacted>"},
		},
		{
			name: "non-env arg with equals is preserved (no LHS letters before =)",
			in:   []string{"run", "=oddly-shaped"},
			want: []string{"run", "=oddly-shaped"},
		},
		{
			name: "label assignments preserved (no AUTH/PASSWORD)",
			in:   []string{"--label", "org.neo4j.cli.managed=true"},
			want: []string{"--label", "org.neo4j.cli.managed=true"},
		},
		{
			name: "empty slice returns empty slice",
			in:   []string{},
			want: []string{},
		},
		{
			name: "nil returns nil",
			in:   nil,
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Snapshot the input so we can assert non-mutation after the call.
			var inCopy []string
			if tc.in != nil {
				inCopy = make([]string, len(tc.in))
				copy(inCopy, tc.in)
			}

			got := redactArgs(tc.in)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, inCopy, tc.in, "redactArgs must not mutate its input slice")
		})
	}
}

// TestRedactString covers the stderr-redaction helper used by execClient.run
// when wrapping captured docker stderr (CLI-162). The helper is the single
// source of truth shared with redactArgs and must mask any
// `KEY=VALUE` assignment whose LHS contains AUTH or PASSWORD
// (case-insensitive, tolerating whitespace around `=`) across multi-line
// blobs, while leaving operational error sentences untouched. Cases mirror
// the Oplane verification subset for REQ-F-001 / REQ-NF-004.
func TestRedactString(t *testing.T) {
	cases := []struct {
		name           string
		in             string
		want           string
		wantNotContain []string // substrings that must NOT appear (secrets)
		wantContain    []string // substrings that MUST still appear (surrounding non-sensitive text)
	}{
		{
			name:           "single-line stderr with NEO4J_AUTH",
			in:             "docker: Error response from daemon: NEO4J_AUTH=neo4j/hunter2 is invalid",
			want:           "docker: Error response from daemon: NEO4J_AUTH=<redacted> is invalid",
			wantNotContain: []string{"hunter2"},
			wantContain:    []string{"Error response from daemon", "is invalid"},
		},
		{
			name: "multi-line blob with two PASSWORD mentions",
			in: "failed to start container:\n" +
				"  env MY_PASSWORD=hunter2 rejected\n" +
				"  env OTHER_PASSWORD=swordfish rejected",
			want: "failed to start container:\n" +
				"  env MY_PASSWORD=<redacted> rejected\n" +
				"  env OTHER_PASSWORD=<redacted> rejected",
			wantNotContain: []string{"hunter2", "swordfish"},
			wantContain:    []string{"failed to start container", "rejected"},
		},
		{
			name:           "unicode value masked",
			in:             "NEO4J_AUTH=neo4j/密码1234 not accepted",
			want:           "NEO4J_AUTH=<redacted> not accepted",
			wantNotContain: []string{"密码1234", "neo4j/密码"},
			wantContain:    []string{"not accepted"},
		},
		{
			name:           "mixed-case LHS with whitespace around equals",
			in:             "config rejected: Neo4j_Auth = secret xyz",
			want:           "config rejected: Neo4j_Auth = <redacted> xyz",
			wantNotContain: []string{"secret"},
			wantContain:    []string{"config rejected", "xyz"},
		},
		{
			name:        "operational sentence with no sensitive assignment returned verbatim",
			in:          "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
			want:        "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
			wantContain: []string{"Cannot connect", "docker daemon running"},
		},
		{
			name: "empty string returns empty string",
			in:   "",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactString(tc.in)
			assert.Equal(t, tc.want, got)
			for _, s := range tc.wantNotContain {
				assert.NotContains(t, got, s, "redactString must mask secret substring %q", s)
			}
			for _, s := range tc.wantContain {
				assert.Contains(t, got, s, "redactString must preserve non-sensitive substring %q", s)
			}
		})
	}
}

// TestClassifyInspectError exercises the stderr-substring classifier that
// execClient.Inspect uses to distinguish "container does not exist" from
// operational docker failures (daemon down, permission denied, rootless
// misconfig, …). The classifier is pulled out into its own function so we
// can drive it with crafted error strings without needing a real docker
// binary; the substring contract documented on classifyInspectError must
// match docker's stable stderr wording for missing containers across modern
// daemon versions.
func TestClassifyInspectError(t *testing.T) {
	cases := []struct {
		name           string
		in             error
		wantNotFound   bool
		wantContainsIn bool // assert returned message contains the name (only on ErrNotFound)
		wantSame       bool // assert returned error == input (operational pass-through)
	}{
		{
			name:           "nil input returns nil",
			in:             nil,
			wantNotFound:   false,
			wantContainsIn: false,
			wantSame:       false, // nil case is asserted separately
		},
		{
			name:           "modern docker 'No such object' stderr",
			in:             errors.New("docker inspect ghost: Error: No such object: ghost"),
			wantNotFound:   true,
			wantContainsIn: true,
		},
		{
			name:           "legacy docker 'No such container' stderr",
			in:             errors.New("docker inspect ghost: Error: No such container: ghost"),
			wantNotFound:   true,
			wantContainsIn: true,
		},
		{
			name:         "daemon down error preserved verbatim",
			in:           errors.New("docker inspect dev: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"),
			wantNotFound: false,
			wantSame:     true,
		},
		{
			name:         "permission denied error preserved verbatim",
			in:           errors.New("docker inspect dev: permission denied while trying to connect to the Docker daemon socket"),
			wantNotFound: false,
			wantSame:     true,
		},
		{
			name:         "rootless misconfig error preserved verbatim",
			in:           errors.New("docker inspect dev: Got permission denied while trying to connect to the Docker daemon socket at unix:///run/user/1000/docker.sock"),
			wantNotFound: false,
			wantSame:     true,
		},
		{
			name:         "context deadline preserved verbatim",
			in:           errors.New("docker inspect dev: signal: killed"),
			wantNotFound: false,
			wantSame:     true,
		},
		{
			name:         "arbitrary unknown error preserved verbatim",
			in:           errors.New("something else entirely"),
			wantNotFound: false,
			wantSame:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyInspectError(tc.in, "ghost")

			if tc.in == nil {
				assert.NoError(t, got, "nil input must return nil")
				return
			}

			require.Error(t, got)
			if tc.wantNotFound {
				assert.True(t, errors.Is(got, ErrNotFound),
					"expected errors.Is(_, ErrNotFound) for missing-container stderr, got %v", got)
				if tc.wantContainsIn {
					assert.Contains(t, got.Error(), "ghost",
						"ErrNotFound wrap should mention the container name")
				}
			} else {
				assert.False(t, errors.Is(got, ErrNotFound),
					"operational error must NOT match ErrNotFound, got %v", got)
				if tc.wantSame {
					// Operational errors must propagate verbatim — same
					// underlying value so the stderr the operator needs to
					// read is preserved exactly.
					assert.Equal(t, tc.in, got,
						"operational error must be returned verbatim (no wrap)")
					assert.Equal(t, tc.in.Error(), got.Error(),
						"error message must be unchanged on operational pass-through")
				}
			}
		})
	}
}

// stubDocker writes a fake `docker` executable into a temp dir, prepends that
// dir to PATH, and returns the dir. The stub records its full argv (one per
// line) to a file the caller can read back, and exits with the supplied code
// after emitting stdoutLine to stdout and stderrLine to stderr. This lets the
// execClient.Exec argv-shaping and error-wrapping paths be exercised without a
// real docker daemon. Unix-only (shell script); the test skips on Windows.
func stubDocker(t *testing.T, exitCode int, stdoutLine, stderrLine string) (argvFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("docker stub is a POSIX shell script; skipped on Windows")
	}
	dir := t.TempDir()
	argvFile = filepath.Join(dir, "argv.txt")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do printf '%s\\n' \"$a\" >> \"" + argvFile + "\"; done\n" +
		"printf '%s' \"" + stdoutLine + "\"\n" +
		"printf '%s' \"" + stderrLine + "\" 1>&2\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"
	dockerStub := filepath.Join(dir, "docker")
	require.NoError(t, os.WriteFile(dockerStub, []byte(script), 0o755))
	t.Setenv("PATH", dir)
	return argvFile
}

// TestExec_Success drives execClient.Exec against a stub docker binary and
// asserts the argv shape is `exec <name> <args...>` and that trimmed stdout is
// returned to the caller.
func TestExec_Success(t *testing.T) {
	argvFile := stubDocker(t, 0, "  hello world  \n", "")

	ec := &execClient{}
	out, err := ec.Exec(context.Background(), "my-neo4j", []string{"cypher-shell", "-u", "neo4j"})
	require.NoError(t, err)
	assert.Equal(t, "hello world", out, "stdout must be returned trimmed")

	raw, readErr := os.ReadFile(argvFile)
	require.NoError(t, readErr)
	wantArgv := "exec\nmy-neo4j\ncypher-shell\n-u\nneo4j\n"
	assert.Equal(t, wantArgv, string(raw), "argv must be `exec <name> <args...>`")
}

// TestExecWithEnv_PassesEnvNotArgv drives execClient.ExecWithEnv against a stub
// docker binary and asserts (a) each env entry contributes a `-e NAME`
// passthrough flag (name only) placed BEFORE the container name, (b) the secret
// VALUE never appears in argv, and (c) the value is forwarded via the docker
// process environment (the stub echoes it back from $NEO4J_PASSWORD).
func TestExecWithEnv_PassesEnvNotArgv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("docker stub is a POSIX shell script; skipped on Windows")
	}
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv.txt")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do printf '%s\\n' \"$a\" >> \"" + argvFile + "\"; done\n" +
		"printf '%s' \"$NEO4J_PASSWORD\"\n" +
		"exit 0\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	t.Setenv("PATH", dir)

	ec := &execClient{}
	out, err := ec.ExecWithEnv(context.Background(), "my-neo4j",
		[]string{"neo4j-admin", "database", "upload", "neo4j"},
		[]string{"NEO4J_USERNAME=neo4j", "NEO4J_PASSWORD=aurasecret"},
	)
	require.NoError(t, err)
	assert.Equal(t, "aurasecret", out, "env value must reach the docker process environment")

	raw, readErr := os.ReadFile(argvFile)
	require.NoError(t, readErr)
	wantArgv := "exec\n-e\nNEO4J_USERNAME\n-e\nNEO4J_PASSWORD\nmy-neo4j\nneo4j-admin\ndatabase\nupload\nneo4j\n"
	assert.Equal(t, wantArgv, string(raw),
		"argv must be `exec -e NAME ... <name> <args...>` with NAMES only (no =value)")
	assert.NotContains(t, string(raw), "aurasecret", "secret value must not appear in argv")
}

func TestExecAs_PlacesUserBeforeContainer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("docker stub is a POSIX shell script; skipped on Windows")
	}
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv.txt")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do printf '%s\\n' \"$a\" >> \"" + argvFile + "\"; done\n" +
		"exit 0\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	t.Setenv("PATH", dir)

	ec := &execClient{}
	_, err := ec.ExecAs(context.Background(), "my-neo4j", "neo4j",
		[]string{"neo4j-admin", "database", "dump", "neo4j"}, nil)
	require.NoError(t, err)

	raw, readErr := os.ReadFile(argvFile)
	require.NoError(t, readErr)
	wantArgv := "exec\n-u\nneo4j\nmy-neo4j\nneo4j-admin\ndatabase\ndump\nneo4j\n"
	assert.Equal(t, wantArgv, string(raw),
		"argv must be `exec -u <user> <name> <args...>` with -u before the container name")
}

// TestExec_ErrorRedacted drives execClient.Exec against a stub docker binary
// that exits non-zero and emits a PASSWORD-bearing stderr line. The returned
// error must wrap docker's stderr verbatim except for the redacted secret, and
// the secret value must never appear in the message.
func TestExec_ErrorRedacted(t *testing.T) {
	stubDocker(t, 1, "", "failed: NEO4J_PASSWORD=hunter2 rejected")

	ec := &execClient{}
	out, err := ec.Exec(context.Background(), "my-neo4j", []string{"neo4j-admin", "database", "dump"})
	require.Error(t, err)
	assert.Empty(t, out)
	msg := err.Error()
	assert.Contains(t, msg, "NEO4J_PASSWORD=<redacted>", "secret env value must be redacted")
	assert.NotContains(t, msg, "hunter2", "raw secret must not leak into the error")
	assert.Contains(t, msg, "rejected", "surrounding stderr text must be preserved")
}

// TestAltRuntimeHint_PodmanMissing verifies altRuntimeHint returns the empty
// string when the injected lookPath stub reports podman is not on PATH. An
// empty return means "no alternative detected" — the caller appends nothing
// to the docker-missing usage error so the message ends with the existing
// install hint and a single period.
func TestAltRuntimeHint_PodmanMissing(t *testing.T) {
	stub := func(name string) (string, error) {
		return "", exec.ErrNotFound
	}
	got := altRuntimeHint(stub)
	assert.Empty(t, got, "expected empty string when podman is not on PATH, got %q", got)
}

// TestAltRuntimeHint_PodmanPresent verifies altRuntimeHint returns a hint
// referencing podman, the literal `alias docker=podman` shell example, and
// the Windows PowerShell `Set-Alias docker podman` alternative when the
// injected lookPath stub reports podman is on PATH. The leading space is
// intentional: it sits after the existing usage error's terminating period
// so the concatenation reads as a single well-formed sentence.
func TestAltRuntimeHint_PodmanPresent(t *testing.T) {
	stub := func(name string) (string, error) {
		assert.Equal(t, "podman", name, "altRuntimeHint should only look up podman")
		return "/usr/local/bin/podman", nil
	}
	got := altRuntimeHint(stub)
	require.NotEmpty(t, got, "expected a hint string when podman is on PATH")
	assert.True(t, got[0] == ' ', "hint must begin with a leading space (got %q)", got)
	assert.Contains(t, got, "podman is a drop-in")
	assert.Contains(t, got, "`alias docker=podman`")
	assert.Contains(t, got, "`Set-Alias docker podman`")
}

// TestResolve_DockerMissing_NoPodman drives execClient.resolve() with PATH
// emptied so the real exec.LookPath miss fires for docker, and lookPathFn
// swapped to a stub that reports podman is NOT on PATH. The returned usage
// error must carry the standard install hint and must NOT mention podman.
func TestResolve_DockerMissing_NoPodman(t *testing.T) {
	t.Setenv("PATH", "")

	orig := lookPathFn
	lookPathFn = func(name string) (string, error) {
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { lookPathFn = orig })

	ec := &execClient{}
	_, err := ec.resolve()
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "docker not found in PATH")
	assert.Contains(t, msg, "install Docker Desktop")
	assert.NotContains(t, msg, "podman", "podman hint must be omitted when podman is not on PATH")
}

// TestResolve_DockerMissing_PodmanPresent drives execClient.resolve() with
// PATH emptied (forcing the docker miss) and lookPathFn swapped to a stub
// that reports podman IS on PATH. The returned usage error must carry both
// the standard install hint AND the podman-alias suggestion so operators
// who already have podman installed see the one-step workaround.
func TestResolve_DockerMissing_PodmanPresent(t *testing.T) {
	t.Setenv("PATH", "")

	orig := lookPathFn
	lookPathFn = func(name string) (string, error) {
		if name == "podman" {
			return "/usr/local/bin/podman", nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { lookPathFn = orig })

	ec := &execClient{}
	_, err := ec.resolve()
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "docker not found in PATH")
	assert.Contains(t, msg, "install Docker Desktop")
	assert.Contains(t, msg, "podman is a drop-in")
	assert.Contains(t, msg, "`alias docker=podman`")
}
