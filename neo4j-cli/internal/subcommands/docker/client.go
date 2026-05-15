// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"context"
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
		return "", clierr.NewUsageError("docker %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
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

// parsePsOutput is split out so unit tests can exercise the JSON-per-line
// shape without invoking the real `docker` binary. The concrete parse lands
// with the `list` leaf in task-008; the scaffold only needs the symbol so
// leaf tasks can import it.
func parsePsOutput(stdout string) ([]PsEntry, error) {
	_ = stdout
	return nil, nil
}

// parseInspectOutput is similarly split out for hermetic testing. The
// concrete parse lands with the `get` leaf in task-009.
func parseInspectOutput(name, stdout string) (Container, error) {
	return Container{Name: name}, nil
}
