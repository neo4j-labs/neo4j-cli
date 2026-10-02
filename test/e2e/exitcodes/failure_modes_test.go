// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build e2e_exitcodes

package exitcodes_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestExitCode_UnreachableServerIsUpstreamNotSuccess pins the fix for a
// network failure panicking out of the Aura transport: the top-level recover
// used to swallow that panic and exit 0, so a command that never reached the
// API reported success.
func TestExitCode_UnreachableServerIsUpstreamNotSuccess(t *testing.T) {
	bin := buildBinary(t)
	home := t.TempDir()
	seedCreds(t, configDirFor(t, home))

	args := append([]string{"aura", "instance", "list", "--format", "json"}, scopeFlags...)
	// Nothing listens on port 1: the very first request is refused.
	code, stdout, stderr := runCLI(t, bin, args, home, "http://127.0.0.1:1")

	if code != 8 {
		t.Fatalf("exit code = %d (want 8, upstream/retryable); a failed command must never exit 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.Contains(stdout+stderr, "Unexpected error running CLI") {
		t.Fatalf("a network failure is an ordinary error, not a crash report\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}

// TestExitCode_SIGINTDuringARequestExits130 starts a command whose request the
// server never answers, interrupts it, and checks the process stops promptly
// with the shell's 128+SIGINT exit code instead of waiting out the 60s client
// timeout.
func TestExitCode_SIGINTDuringARequestExits130(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cannot deliver SIGINT to a child process on Windows")
	}
	bin := buildBinary(t)
	home := t.TempDir()
	seedCreds(t, configDirFor(t, home))

	var once sync.Once
	inFlight := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(inFlight) })
		<-r.Context().Done() // never answer; return when the client goes away
	}))
	defer srv.Close()

	args := append([]string{"aura", "instance", "list", "--format", "json"}, scopeFlags...)
	cmd := exec.Command(bin, args...)
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"AURA_BASE_URL=" + srv.URL,
		"AURA_AUTH_URL=" + srv.URL + "/oauth/token",
		"NEO4J_CLI_NO_UPDATE_NAG=1",
		"DO_NOT_TRACK=1",
	}, configHomeFor(t, home)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	select {
	case <-inFlight:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("the CLI never reached the server\nstderr:\n%s", stderr.String())
	}

	start := time.Now()
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("signal: %v", err)
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	select {
	case err := <-waitErr:
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("expected a non-zero exit, got %v\nstderr:\n%s", err, stderr.String())
		}
		if exitErr.ExitCode() != 130 {
			t.Fatalf("exit code = %d (want 130)\nstderr:\n%s", exitErr.ExitCode(), stderr.String())
		}
		if took := time.Since(start); took > 10*time.Second {
			t.Fatalf("took %s to stop after SIGINT; cancellation must not wait for the request", took)
		}
		if !strings.Contains(stderr.String(), "interrupted") {
			t.Fatalf("the user should be told the command was interrupted\nstderr:\n%s", stderr.String())
		}
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("the CLI did not exit after SIGINT\nstderr:\n%s", stderr.String())
	}
}
