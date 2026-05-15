// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/neo4j/cli/common/clierr"
)

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
			"docker not found in PATH — install Docker Desktop (https://www.docker.com/products/docker-desktop/) or the docker CLI",
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
		// is only applied to the user-facing error string.
		return "", clierr.NewUsageError("docker %s: %s", strings.Join(redactArgs(args), " "), msg)
	}
	return stdout.String(), nil
}

// redactArgs returns a copy of args with any element shaped like a sensitive
// env-var assignment (LHS contains AUTH or PASSWORD) replaced by `<LHS>=<redacted>`.
// Non-env elements are preserved unchanged and the input slice is never mutated.
// Match is performed on the uppercase-folded LHS so case variants are caught,
// even though Neo4j's own env-var names are uppercase by convention.
func redactArgs(args []string) []string {
	if args == nil {
		return nil
	}
	out := make([]string, len(args))
	for i, a := range args {
		eq := strings.IndexByte(a, '=')
		if eq <= 0 {
			out[i] = a
			continue
		}
		lhs := strings.ToUpper(a[:eq])
		if strings.Contains(lhs, "AUTH") || strings.Contains(lhs, "PASSWORD") {
			out[i] = a[:eq] + "=<redacted>"
			continue
		}
		out[i] = a
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

func (c *execClient) Inspect(ctx context.Context, name string) (Container, error) {
	out, err := c.run(ctx, "inspect", name)
	if err != nil {
		// docker exits non-zero on missing containers; surface as the
		// caller's chosen NotFound shape. Leaves wrap this further with
		// the REQ-F-032 hint pointing at `docker list`.
		return Container{}, err
	}
	return parseInspectOutput(name, out)
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
