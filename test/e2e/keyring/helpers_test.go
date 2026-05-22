//go:build keyring_smoke

// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package keyring_test contains end-to-end smoke tests for OS keyring integration.
//
// These tests drive the pre-built neo4j-cli binary as a subprocess and verify
// that credential storage, migration, and removal interact correctly with the
// OS keyring on each platform. They are gated behind the `keyring_smoke` build
// tag so they are excluded from the default `go test ./...` run.
//
// Prerequisites:
//   - `make build` must have been run to produce bin/neo4j-cli (or bin/neo4j-cli.exe on Windows)
//   - On Linux: a gnome-keyring daemon or similar libsecret provider is required for the
//     with-daemon group; the no-daemon group runs without any keyring daemon.
package keyring_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from this test file to the repo root (directory containing go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine caller file path")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not locate repo root (go.mod) walking up from test file")
	return ""
}

// binaryPath returns the absolute path to the pre-built neo4j-cli binary in bin/.
// It assumes `make build` has already been run before the test suite.
func binaryPath(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	name := "neo4j-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(root, "bin", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("binary not found at %s — run `make build` first: %v", p, err)
	}
	return p
}

// configEnvForDir returns the env-var assignments that point the CLI at
// dir for its config/credentials. On Linux the init() in common/clicfg/linux.go
// reads XDG_CONFIG_HOME at process startup, so we must pass that env var to the
// subprocess to redirect its config directory. HOME is also set for completeness.
func configEnvForDir(dir string) []string {
	switch runtime.GOOS {
	case "linux":
		return []string{
			"HOME=" + dir,
			"XDG_CONFIG_HOME=" + filepath.Join(dir, ".config"),
		}
	case "darwin":
		return []string{"HOME=" + dir}
	case "windows":
		return []string{
			"USERPROFILE=" + dir,
			"LOCALAPPDATA=" + filepath.Join(dir, "AppData", "Local"),
			"APPDATA=" + filepath.Join(dir, "AppData", "Roaming"),
		}
	default:
		return []string{"HOME=" + dir}
	}
}

// configDirForHome returns the absolute on-disk directory under which
// neo4j-cli will look for config.json and credentials.json.
func configDirForHome(dir string) string {
	switch runtime.GOOS {
	case "linux":
		return filepath.Join(dir, ".config", "neo4j", "cli")
	case "darwin":
		return filepath.Join(dir, "Library", "Preferences", "neo4j", "cli")
	case "windows":
		return filepath.Join(dir, "AppData", "Local", "neo4j", "cli")
	default:
		return filepath.Join(dir, ".config", "neo4j", "cli")
	}
}

// baseChildEnv assembles a minimal environment for a subprocess invocation of
// the binary. It includes PATH and optionally SystemRoot (Windows DLL loading),
// and suppresses telemetry / version-check noise. Callers append platform-specific
// config-home overrides and any additional env vars via extra.
//
// DBUS_SESSION_BUS_ADDRESS is forwarded when set in the parent process so that
// the neo4j-cli subprocess can connect to the gnome-keyring daemon on Linux.
// No-daemon tests call stripDBUS(baseChildEnv(...)) to remove it explicitly.
func baseChildEnv(extra ...string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"DO_NOT_TRACK=1",
		"NEO4J_CLI_NO_UPDATE_NAG=1",
	}
	if sr := os.Getenv("SystemRoot"); sr != "" {
		env = append(env, "SystemRoot="+sr)
	}
	if v := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); v != "" {
		env = append(env, "DBUS_SESSION_BUS_ADDRESS="+v)
	}
	env = append(env, extra...)
	return env
}

// runCLI launches the pre-built binary with the supplied args and env.
// Returns exit code, stdout, stderr as strings.
func runCLI(t *testing.T, bin string, args []string, env []string) (exitCode int, stdout, stderr string) {
	t.Helper()

	cmd := exec.Command(bin, args...)
	cmd.Env = env

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()

	code := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("running %s failed without ExitError: %v", bin, runErr)
		}
	}
	return code, outBuf.String(), errBuf.String()
}

// readConfigJSON reads config.json from the CLI config dir rooted at homeDir
// and returns it as a map. Returns nil if the file does not exist.
func readConfigJSON(t *testing.T, homeDir string) map[string]interface{} {
	t.Helper()
	p := filepath.Join(configDirForHome(homeDir), "config.json")
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse config.json: %v\n%s", err, data)
	}
	return m
}

// readCredentialsJSON reads credentials.json from the CLI config dir rooted
// at homeDir and returns it as a map. Returns nil if the file does not exist.
func readCredentialsJSON(t *testing.T, homeDir string) map[string]interface{} {
	t.Helper()
	p := filepath.Join(configDirForHome(homeDir), "credentials.json")
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read credentials.json: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse credentials.json: %v\n%s", err, data)
	}
	return m
}

// writeConfigJSON writes a config.json file under homeDir's config dir.
// Creates the directory if it does not exist.
func writeConfigJSON(t *testing.T, homeDir string, content map[string]interface{}) {
	t.Helper()
	dir := configDirForHome(homeDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal config.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
}

// writeCredentialsJSON writes a credentials.json file under homeDir's config dir.
// Creates the directory if it does not exist.
func writeCredentialsJSON(t *testing.T, homeDir string, content string) {
	t.Helper()
	dir := configDirForHome(homeDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write credentials.json: %v", err)
	}
}

// insecureCredentialsJSON returns a credentials.json body containing one dbms
// credential with the given name and password stored in plaintext (insecure mode).
func insecureCredentialsJSON(name, password string) string {
	return fmt.Sprintf(`{"aura":{"default-credential":"","credentials":[]},"dbms":{"default-credential":%q,"credentials":[{"name":%q,"username":"neo4j","password":%q,"database-name":"neo4j","uri":"neo4j://localhost:7687"}]},"embed":{"default-credential":"","credentials":[]}}`,
		name, name, password)
}
