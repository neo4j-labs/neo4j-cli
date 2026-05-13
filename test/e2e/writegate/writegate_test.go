// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package writegate drives the freshly-built `neo4j-cli` binary as a
// subprocess and asserts the agent-detect / TTY / --rw write gate behaves
// end-to-end.
//
// Today the gate is unit-tested through the detectAgent / stdoutIsTerminal
// seams in common/flags. This package closes the remaining gap by exercising
// the production wiring with real env vars + a real (non-TTY) stdout, so a
// future drift between agent.Detect() and the gate or a typo in the agent
// env-var list would surface in CI.
//
// Scope: the regression-dangerous paths (gate blocks an agent or a piped
// script). The TTY-success path is intentionally out of scope because
// synthesising a PTY in pure Go test requires a new dependency
// (creack/pty); the unit-level gate-matrix test in common/flags already
// covers stdoutIsTerminal == true.
package writegate_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// agentEnvVars enumerates every env var that common/agent inspects. The
// child process needs all of them stripped to reproduce the "no agent, no
// TTY" branch reliably — leaking even one (e.g. a dev shell with
// CLAUDECODE=1) flips the gate to "agent detected" and the piped_blocks
// assertion would still pass but for the wrong reason. Keep this list in
// sync with common/agent/agent.go.
var agentEnvVars = []string{
	"CLAUDECODE",
	"CLAUDE_CODE",
	"REPL_ID",
	"GEMINI_CLI",
	"CODEX_SANDBOX",
	"CODEX_THREAD_ID",
	"OPENCODE",
	"AUGMENT_AGENT",
	"GOOSE_PROVIDER",
	"CURSOR_AGENT",
	"EDITOR",
	"TERM_PROGRAM",
	// NOTE: PATH is also inspected (substring ".pi/agent") but stripping
	// it would break `go build` / child-process invocation. We rely on
	// PATH not containing ".pi/agent" in CI.
}

// canonicalGateErr is the user-facing string EnforceWriteGate returns when
// the gate blocks a write. Tests grep for it verbatim across the codebase;
// keeping it stable is part of the PRD contract.
const canonicalGateErr = "this command writes; pass --rw to allow it"

// binPath holds the absolute path to the freshly built neo4j-cli binary.
// Populated by TestMain so all subtests share a single build (cuts test
// runtime ~3x vs. building per-subtest) and the binary's parent dir is
// cleaned up at process exit.
var binPath string

// repoRoot walks up from this test file to the repo root (directory
// containing go.mod). Mirrors the helper in test/e2e/exitcodes.
func repoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("could not determine caller file path")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("could not locate repo root (go.mod) walking up from test file")
}

// TestMain builds the neo4j-cli binary once into a temp dir, runs the
// tests, and removes the temp dir. CGO_ENABLED=0 mirrors release builds
// (.goreleaser.yaml).
func TestMain(m *testing.M) {
	code, err := runMain(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

func runMain(m *testing.M) (int, error) {
	root, err := repoRoot()
	if err != nil {
		return 0, err
	}

	dir, err := os.MkdirTemp("", "neo4j-cli-writegate-bin-*")
	if err != nil {
		return 0, fmt.Errorf("mkdir bin dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	name := "neo4j-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, name)

	cmd := exec.Command("go", "build", "-o", out, "./neo4j-cli")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if combined, buildErr := cmd.CombinedOutput(); buildErr != nil {
		return 0, fmt.Errorf("go build failed: %w\n%s", buildErr, combined)
	}
	binPath = out

	return m.Run(), nil
}

// configHomeFor returns the env-var assignments that point the CLI at
// `dir` for its config/credentials. Mirrors the helper in
// test/e2e/exitcodes; see the comment there for the per-OS rationale.
//
// IMPORTANT: on darwin the binary uses os/user.Current() (via
// common/clicfg/darwin.go) which reads from the passwd database rather
// than $HOME. Setting HOME has NO EFFECT on the resolved ConfigPrefix.
// Callers that require true write-isolation on darwin must skip the
// subtest (see redirectsConfigDir).
func configHomeFor(dir string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"HOME=" + dir}
	case "linux":
		return []string{
			"HOME=" + dir,
			"XDG_CONFIG_HOME=" + filepath.Join(dir, ".config"),
		}
	case "windows":
		return []string{"LOCALAPPDATA=" + dir}
	default:
		return nil
	}
}

// configDirFor returns the absolute on-disk directory under which
// `neo4j-cli` will look for `config.json` and `credentials.json` given
// the HOME / XDG / LOCALAPPDATA override produced by configHomeFor.
func configDirFor(dir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(dir, "Library", "Preferences", "neo4j", "cli")
	case "linux":
		return filepath.Join(dir, ".config", "neo4j", "cli")
	case "windows":
		return filepath.Join(dir, "neo4j", "cli")
	default:
		return ""
	}
}

// redirectsConfigDir reports whether the running platform honours an
// env-var-only redirect of clicfg.ConfigPrefix. Returns false on darwin
// where os/user.Current() resolves homedir via the passwd database and
// silently ignores $HOME — there is no env-var seam in common/clicfg to
// route around it, so the agent_with_rw_succeeds subtest cannot run in
// isolation without mutating the dev's real credentials.json.
func redirectsConfigDir() bool {
	return runtime.GOOS != "darwin"
}

// childEnv assembles the minimal environment for a subprocess run of the
// freshly built binary. The base set is intentionally tiny:
//   - PATH so the binary can find anything it shells out to
//   - SystemRoot on windows so DLL loading works
//   - DO_NOT_TRACK / NEO4J_CLI_NO_UPDATE_NAG so telemetry / version-check
//     side channels don't pollute stderr
//
// Every agent env var listed in agentEnvVars is *explicitly omitted* so
// stale dev-shell vars (CLAUDECODE=1, …) don't leak into "no agent"
// scenarios. Callers pass extra entries (e.g. an agent var, HOME
// redirect) via the `extra` argument.
func childEnv(extra ...string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"DO_NOT_TRACK=1",
		"NEO4J_CLI_NO_UPDATE_NAG=1",
	}
	if sr := os.Getenv("SystemRoot"); sr != "" {
		env = append(env, "SystemRoot="+sr)
	}
	env = append(env, extra...)
	return env
}

// runCLI launches the freshly built binary with the supplied args and
// env. Returns exit code, stdout, stderr. Stdout/stderr are piped (not
// inherited) so the child sees a non-TTY stdout — exactly the "no TTY"
// branch the gate matrix tests cover via the stdoutIsTerminal seam.
func runCLI(t *testing.T, args []string, env []string) (int, string, string) {
	t.Helper()

	cmd := exec.Command(binPath, args...)
	cmd.Env = env

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("running %s failed without ExitError: %v", binPath, runErr)
		}
	}
	return exitCode, outBuf.String(), errBuf.String()
}

// dbmsAddArgs returns the credential-dbms-add args used across the three
// subtests. The values are deliberately throwaway — the gate is checked
// before any URI validation, and the writes (when allowed) land inside
// t.TempDir().
func dbmsAddArgs(extra ...string) []string {
	args := []string{
		"credential", "dbms", "add",
		"--name", "e2e-writegate",
		"--uri", "neo4j+s://example.test",
		"--username", "neo4j",
		"--password", "test-password",
	}
	return append(args, extra...)
}

func TestWriteGate(t *testing.T) {
	t.Run("agent_blocks", func(t *testing.T) {
		home := t.TempDir()

		env := childEnv(
			"CLAUDECODE=1",
		)
		env = append(env, configHomeFor(home)...)

		code, stdout, stderr := runCLI(t, dbmsAddArgs(), env)

		if code != 2 {
			t.Fatalf("exit code = %d (want 2)\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, canonicalGateErr) {
			t.Fatalf("stderr does not contain canonical gate error %q\nstderr:\n%s", canonicalGateErr, stderr)
		}
	})

	t.Run("piped_blocks", func(t *testing.T) {
		home := t.TempDir()

		// Build env that explicitly excludes every agent env var listed
		// in agentEnvVars. childEnv already omits them — we just need
		// to NOT re-add any. The non-TTY stdout comes for free from
		// exec.Command's pipe.
		env := childEnv()
		env = append(env, configHomeFor(home)...)

		// Defensive: assert no agent var slipped through.
		for _, e := range env {
			for _, agentVar := range agentEnvVars {
				if strings.HasPrefix(e, agentVar+"=") {
					t.Fatalf("piped_blocks env unexpectedly contains agent var %q (%q)", agentVar, e)
				}
			}
		}

		code, stdout, stderr := runCLI(t, dbmsAddArgs(), env)

		if code != 2 {
			t.Fatalf("exit code = %d (want 2)\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, canonicalGateErr) {
			t.Fatalf("stderr does not contain canonical gate error %q\nstderr:\n%s", canonicalGateErr, stderr)
		}
	})

	t.Run("agent_with_rw_succeeds", func(t *testing.T) {
		if !redirectsConfigDir() {
			t.Skipf("skipping on %s: clicfg.ConfigPrefix is derived from os/user.Current() and cannot be redirected via env vars on this platform; running the subtest would mutate the user's real credentials.json", runtime.GOOS)
		}

		home := t.TempDir()
		expectedCredsPath := filepath.Join(configDirFor(home), "credentials.json")

		env := childEnv(
			"CLAUDECODE=1",
		)
		env = append(env, configHomeFor(home)...)

		code, stdout, stderr := runCLI(t, dbmsAddArgs("--rw=true"), env)

		if code != 0 {
			t.Fatalf("exit code = %d (want 0)\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}

		// Verify the credentials file landed inside t.TempDir() —
		// proves (1) the gate let the write through and (2) the
		// env-override actually redirected ConfigPrefix, so no
		// dev-machine credentials were touched.
		if _, err := os.Stat(expectedCredsPath); err != nil {
			t.Fatalf("expected credentials.json at %q after write, got: %v\nstdout:\n%s\nstderr:\n%s",
				expectedCredsPath, err, stdout, stderr)
		}
	})
}
