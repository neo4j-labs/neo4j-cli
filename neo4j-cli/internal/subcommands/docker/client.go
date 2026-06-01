// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"github.com/neo4j/cli/common/clierr"
)

// lookPathFn is the injectable seam used by altRuntimeHint to detect
// alternative container runtimes (currently podman) on PATH. Production
// wires exec.LookPath; tests swap a deterministic stub so the docker-
// missing error path can be exercised against both "podman present" and
// "podman missing" branches without touching the host PATH.
var lookPathFn = exec.LookPath

// altRuntimeHint returns a non-empty string describing an alternative
// container runtime that the operator can alias as `docker` when the real
// docker binary is missing from PATH. Currently checks for podman. The
// result is appended to the docker-missing usage error; an empty return
// means "no alternative detected, use the default message".
//
// Backticks around shell tokens are emitted as literal characters so the
// suggestion reads cleanly in plain terminals — the message is rendered
// through fmt-style format, not a markdown renderer.
func altRuntimeHint(lookPath func(string) (string, error)) string {
	if _, err := lookPath("podman"); err == nil {
		return " It looks like you have podman installed; podman is a drop-in for the docker CLI." +
			" Aliasing it as `docker` (e.g. `alias docker=podman` in your shell rc," +
			" or `Set-Alias docker podman` on Windows PowerShell) will let neo4j-cli use it."
	}
	return ""
}

// ErrNotFound signals that `docker inspect` reported the container does not
// exist. All other docker errors (daemon down, permission denied, timeout,
// rootless misconfig, etc.) are returned with their underlying stderr/exit-code
// information preserved so the operator sees what actually broke instead of a
// misleading "no such container" message. Leaf commands compare with
// errors.Is(err, ErrNotFound) and translate to the documented unknown-name
// usage error only on that match.
var ErrNotFound = errors.New("docker: container not found")

// sensitiveAssignmentRe matches `KEY=VALUE` env-style assignments whose LHS
// contains AUTH or PASSWORD (case-insensitive), tolerating whitespace around
// `=`. Group 1 captures the full LHS including the `=` so the replacement
// template can preserve it while masking the value. The pattern is shared by
// argv-echo (redactArgs) and docker stderr (redactString) so a future
// tightening edits one regex, not two algorithms.
var sensitiveAssignmentRe = regexp.MustCompile(`(?i)(\w*(?:AUTH|PASSWORD)\w*\s*=\s*)(\S+)`)

// redactString returns s with every `KEY=VALUE` assignment whose LHS contains
// AUTH or PASSWORD (case-insensitive) rewritten as `KEY=<redacted>`. The
// underlying regex `(?i)(\w*(?:AUTH|PASSWORD)\w*\s*=\s*)(\S+)` captures the
// full LHS (including `=`) in group 1 and the non-whitespace value in group 2;
// the replacement template `${1}<redacted>` keeps the LHS verbatim. Backed by
// ReplaceAllString so multi-match stderr blobs (e.g. multi-line docker errors
// echoing several env vars) are fully covered, not just the first hit.
// Single source of truth for credential redaction across the argv echo and
// docker's own captured stderr.
func redactString(s string) string {
	return sensitiveAssignmentRe.ReplaceAllString(s, "${1}<redacted>")
}

// dockerClient abstracts the host `docker` CLI. The default execClient shells
// out via os/exec; tests inject a fake (see helpers_test.go). Every method
// returns a clierr.UsageError when the docker binary is missing from PATH
// (REQ-F-060) so all leaf commands surface the same install hint.
//
// RunArgs / leaf-specific argument plumbing is deferred to the tasks that
// build the leaves; this interface only fixes the verbs.
type dockerClient interface {
	// Run shells `docker run -d ...args` and returns the container ID (stdout)
	// or a typed error including captured stderr (REQ-F-061).
	Run(ctx context.Context, args []string) (string, error)
	// Start shells `docker start <name>`.
	Start(ctx context.Context, name string) error
	// Stop shells `docker stop <name>`.
	Stop(ctx context.Context, name string) error
	// RemoveForce shells `docker rm -f <name>`.
	RemoveForce(ctx context.Context, name string) error
	// PsAll shells `docker ps -a --format '{{json .}}'` (optionally with
	// extra filters). The returned slice contains one parsed entry per
	// container line on stdout.
	PsAll(ctx context.Context, filters []string) ([]PsEntry, error)
	// Inspect shells `docker inspect <name>` and parses the labels +
	// state needed to populate a Container metadata struct. Returns a
	// NotFound-style clierr when the container does not exist.
	Inspect(ctx context.Context, name string) (Container, error)
	// Exec shells `docker exec <name> <args...>` and returns trimmed stdout.
	// Non-zero exit wraps docker's captured stderr (with AUTH/PASSWORD
	// redacted) in a clierr.UsageError via the shared run path.
	Exec(ctx context.Context, name string, args []string) (string, error)
}

// PsEntry is the subset of `docker ps --format '{{json .}}'` fields we use.
// Field tags use the Title-Case shape Docker emits (Names, Status, …).
type PsEntry struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Status string `json:"Status"`
	State  string `json:"State"`
	Image  string `json:"Image"`
	Labels string `json:"Labels"`
}

// execClient is the default dockerClient that shells out via os/exec.
// dockerPath is resolved lazily on first use (REQ-F-060) so other neo4j-cli
// subtrees (aura, query, credential, …) stay usable on hosts without docker
// installed.
type execClient struct {
	once       sync.Once
	dockerPath string
	lookupErr  error
}

// newClient returns the default exec-backed client. Wired by the docker
// parent and each leaf in later tasks.
func newClient() dockerClient {
	return &execClient{}
}

// NewDeployClient returns the default exec-backed dockerClient for callers
// outside the docker package (e.g. the aura `instance deploy` leaf) that need
// to pass a client into PushToAura. The concrete client type stays unexported;
// only this constructor and the dockerClient methods are reachable, preserving
// the package's existing test seam.
func NewDeployClient() dockerClient {
	return newClient()
}

// resolve performs the cached exec.LookPath("docker") and converts a miss
// into the documented usage error. All execClient methods funnel through
// this so the hint appears exactly once per process invocation.
func (c *execClient) resolve() (string, error) {
	c.once.Do(func() {
		path, err := exec.LookPath("docker")
		if err != nil {
			c.lookupErr = err
			return
		}
		c.dockerPath = path
	})
	if c.lookupErr != nil {
		return "", clierr.NewUsageError(
			"docker not found in PATH — install Docker Desktop (https://www.docker.com/products/docker-desktop/) or the docker CLI.%s",
			altRuntimeHint(lookPathFn),
		)
	}
	return c.dockerPath, nil
}

// run invokes `docker <args...>` and returns stdout. On non-zero exit it
// wraps the captured stderr (REQ-F-061) in a clierr.UsageError so the user
// sees Docker's own error verbatim.
func (c *execClient) run(ctx context.Context, args ...string) (string, error) {
	path, err := c.resolve()
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		// Redact AUTH/PASSWORD env values before echoing argv (REQ-NF-004) —
		// `docker run -e NEO4J_AUTH=neo4j/<secret>` would otherwise leak the
		// generated password to the terminal and any captured shell/CI logs
		// on non-zero exit. The argv passed to exec is untouched; redaction
		// is only applied to the user-facing error string. The same
		// redaction is applied to docker's own captured stderr (CLI-162) so
		// third-party wrappers (alias docker=podman, lazydocker) and any
		// future docker release that echoes argv-with-env on failure cannot
		// leak the value through the stderr surface.
		return "", clierr.NewUsageError("docker %s: %s", strings.Join(redactArgs(args), " "), redactString(msg))
	}
	return stdout.String(), nil
}

// redactArgs returns a copy of args with any element shaped like a sensitive
// env-var assignment (LHS contains AUTH or PASSWORD) replaced by
// `<LHS>=<redacted>`. Non-env elements are preserved unchanged and the input
// slice is never mutated. Per-element redaction is delegated to redactString,
// so the LHS match is regex-driven and case-insensitive (the previous
// strings.ToUpper LHS-fold is now folded into the (?i) flag on the shared
// regex). The nil/empty short-circuit is preserved: nil input returns nil,
// empty input returns an empty slice.
func redactArgs(args []string) []string {
	if args == nil {
		return nil
	}
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = redactString(a)
	}
	return out
}

func (c *execClient) Run(ctx context.Context, args []string) (string, error) {
	out, err := c.run(ctx, append([]string{"run", "-d"}, args...)...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *execClient) Start(ctx context.Context, name string) error {
	_, err := c.run(ctx, "start", name)
	return err
}

func (c *execClient) Stop(ctx context.Context, name string) error {
	_, err := c.run(ctx, "stop", name)
	return err
}

func (c *execClient) RemoveForce(ctx context.Context, name string) error {
	_, err := c.run(ctx, "rm", "-f", name)
	return err
}

func (c *execClient) PsAll(ctx context.Context, filters []string) ([]PsEntry, error) {
	args := []string{"ps", "-a", "--format", "{{json .}}"}
	for _, f := range filters {
		args = append(args, "--filter", f)
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parsePsOutput(out)
}

func (c *execClient) Exec(ctx context.Context, name string, args []string) (string, error) {
	out, err := c.run(ctx, append([]string{"exec", name}, args...)...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *execClient) Inspect(ctx context.Context, name string) (Container, error) {
	out, err := c.run(ctx, "inspect", name)
	if err != nil {
		// Classify the error so leaves can distinguish "container missing"
		// (REQ-F-032 unknown-name message) from operational failures (daemon
		// down, permission denied, rootless misconfig). Operational errors
		// propagate with docker's stderr verbatim so the operator can act on
		// the real cause instead of chasing a phantom container.
		return Container{}, classifyInspectError(err, name)
	}
	return parseInspectOutput(name, out)
}

// classifyInspectError converts the wrapped error returned by c.run when
// invoking `docker inspect` into either an ErrNotFound wrap (when docker's
// stderr reported the container is missing) or the original error (for any
// other failure shape). Docker's stderr wording for missing containers is
// stable across versions: "Error: No such object: <name>" (modern, since
// ~2018) or "Error: No such container: <name>" (older). Matching the shared
// "No such " substring covers both variants.
//
// Note: c.run already wraps the captured docker stderr inside a
// clierr.UsageError whose message string contains the original stderr text,
// so we classify by substring on err.Error() rather than re-running exec or
// peeking at the buffer. Pulled out into its own function so client_test.go
// can drive the classification without needing a real docker binary.
func classifyInspectError(runErr error, name string) error {
	if runErr == nil {
		return nil
	}
	if strings.Contains(runErr.Error(), "No such ") {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return runErr
}

// parsePsOutput decodes the output of
// `docker ps -a --format '{{json .}}'` — one JSON object per line, separated
// by newlines. Empty stdout yields an empty slice and no error. Malformed
// JSON on any line returns a typed error naming the 1-indexed line number so
// version drift surfaces immediately rather than silently emptying the list
// (user-locked fail-loud decision).
//
// The scanner buffer is bumped to 4 MiB because rich label payloads
// (multi-line org.opencontainers.image.* etc.) can push a single emitted line
// well past bufio's default 64 KB ceiling.
func parsePsOutput(stdout string) ([]PsEntry, error) {
	entries := []PsEntry{}
	if stdout == "" {
		return entries, nil
	}
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var entry PsEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			return nil, fmt.Errorf("docker: parse ps output: line %d: %w", lineNo, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("docker: parse ps output: scan: %w", err)
	}
	return entries, nil
}

// parseInspectOutput decodes the output of `docker inspect <name>` — a JSON
// array with exactly one element for a single-name inspect. Only the fields
// we render are extracted (Name, State.Status/Running, Config.Image,
// Config.Labels); unknown fields in Docker's payload are ignored.
//
// Fail loud on malformed JSON or unexpected element count (0 or >1) so
// daemon/version drift surfaces immediately. Missing labels are NOT an error
// — the resulting Container simply has empty strings and Managed=false; the
// leaf's managed-gate handles refusing unmanaged containers.
func parseInspectOutput(name, stdout string) (Container, error) {
	type inspectShape struct {
		Name  string `json:"Name"`
		State struct {
			Status  string `json:"Status"`
			Running bool   `json:"Running"`
		} `json:"State"`
		Config struct {
			Image  string            `json:"Image"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	var shapes []inspectShape
	if err := json.Unmarshal([]byte(stdout), &shapes); err != nil {
		return Container{}, fmt.Errorf("docker: parse inspect output for %q: %w", name, err)
	}
	if len(shapes) != 1 {
		return Container{}, fmt.Errorf("docker: parse inspect output for %q: expected 1 container, got %d", name, len(shapes))
	}
	s := shapes[0]
	labels := s.Config.Labels
	return Container{
		Name:      strings.TrimPrefix(s.Name, "/"),
		Status:    s.State.Status,
		Running:   s.State.Running,
		Image:     s.Config.Image,
		Edition:   labels[LabelEdition],
		Version:   labels[LabelVersion],
		BoltPort:  labels[LabelBoltPort],
		HTTPPort:  labels[LabelHTTPPort],
		Ephemeral: labels[LabelEphemeral] == "true",
		Managed:   labels[LabelManaged] == "true",
	}, nil
}
