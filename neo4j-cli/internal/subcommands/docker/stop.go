// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/cobra"
)

// newStopCmd builds the `neo4j-cli docker stop <name>` leaf (REQ-F-040).
// It shells `docker stop <name>` via the dockerClient seam after verifying
// the container exists and carries `org.neo4j.cli.managed=true` so we never
// touch containers managed outside neo4j-cli (REQ-F-043).
//
// When `--wait` is set, after `docker stop` succeeds we poll Inspect every
// pollInterval until either `State.Running == false` or waitTimeout elapses
// (REQ-F-041). For ephemeral containers (`--rm`) the daemon removes the
// container the moment it exits, so Inspect returning "no such container"
// is treated as a successful exit rather than an error — it's the natural
// terminal state for the ephemeral case.
//
// Unlike `start --wait` (which polls Bolt readiness) `stop --wait` does NOT
// need a stored dbms credential — the running-state check is a metadata-only
// query against the daemon, not an authenticated Bolt session.
func newStopCmd(cfg *clicfg.Config) *cobra.Command {
	_ = cfg // reserved for future use (credential cleanup is delete's job, not stop's)
	var wait bool

	cmd := &cobra.Command{
		Use:         "stop <name>",
		Short:       "Stop a running Neo4j container managed by neo4j-cli",
		Annotations: map[string]string{"write": "true"},
		Long: "Stop a running Neo4j Docker container by name. " +
			"Only containers carrying `org.neo4j.cli.managed=true` are eligible; " +
			"unknown or unmanaged names return a usage error pointing at `neo4j-cli docker list`. " +
			"Pass --wait to block until the container has actually exited (60s timeout). " +
			"Ephemeral containers (`--rm`) are removed by Docker the moment they exit, so a " +
			"subsequent `neo4j-cli docker get` will return the same unknown-name error.",
		Example: `# Stop a managed container by name
neo4j-cli docker stop dev --rw

# Stop and block until the container has fully exited before returning
neo4j-cli docker stop dev --wait --rw

# Same as above using the deprecated --await alias
neo4j-cli docker stop dev --await --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			client := clientFactory()
			ctx := cmd.Context()

			// Inspect first so we can refuse non-managed / missing containers
			// before mutating any daemon state (REQ-F-043). Any Inspect error
			// — missing container, removed ephemeral, daemon unreachable — is
			// funneled into the documented unknown-name message; mirrors the
			// pattern used by `get` and `start`.
			container, err := client.Inspect(ctx, name)
			if err != nil {
				cmd.SilenceUsage = true
				return unknownContainerError(name)
			}
			if !container.Managed {
				cmd.SilenceUsage = true
				return unknownContainerError(name)
			}

			if err := client.Stop(ctx, name); err != nil {
				// dockerClient.Stop wraps captured stderr verbatim in a
				// clierr.UsageError (REQ-F-061); surface as-is.
				cmd.SilenceUsage = true
				return err
			}

			if !wait {
				return nil
			}

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "info: waiting for container %q to exit...\n", name)
			if err := waitForExit(ctx, client, name, waitTimeout); err != nil {
				cmd.SilenceUsage = true
				return err
			}
			return nil
		},
	}

	flags.RegisterWait(cmd, &wait, "Wait until the container has exited before returning.")
	return cmd
}

// waitForExit polls dockerClient.Inspect every pollInterval until the named
// container is no longer running or timeout elapses. Inspect returning an
// error whose message indicates the container is gone (the natural terminal
// state for an ephemeral `--rm` container) counts as a successful exit;
// other Inspect errors are treated as transient and retried until the
// deadline so a flaky daemon doesn't surface as an immediate failure.
//
// The first probe fires immediately so a container that has already exited
// (e.g. `docker stop` finished synchronously) returns without sleeping for
// pollInterval first.
func waitForExit(ctx context.Context, client dockerClient, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	probeCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	if exited := inspectExited(probeCtx, client, name); exited {
		return nil
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-probeCtx.Done():
			return clierr.NewUsageError(
				"container %q did not exit within %s; check 'docker logs %s'",
				name, timeout, name,
			)
		case <-ticker.C:
			if exited := inspectExited(probeCtx, client, name); exited {
				return nil
			}
		}
	}
}

// inspectExited returns true when the container is observably gone:
// Inspect reports State.Running == false, OR Inspect errors with a
// missing-container shape (ephemeral `--rm` cleaned up after exit).
// Any other Inspect error is treated as transient — the poll loop will
// keep retrying within the deadline.
func inspectExited(ctx context.Context, client dockerClient, name string) bool {
	c, err := client.Inspect(ctx, name)
	if err != nil {
		// "no such container" / "No such object" is the daemon's wording when
		// an ephemeral container has been removed after exit. Treat it as
		// the natural terminal state for stop --wait. Other errors (daemon
		// unreachable, etc.) are transient — fall through to the poll loop.
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "no such container") || strings.Contains(msg, "no such object") {
			return true
		}
		return false
	}
	return !c.Running
}
