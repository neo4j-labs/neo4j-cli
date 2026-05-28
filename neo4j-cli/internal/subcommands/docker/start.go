// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/cobra"
)

// waitForBoltTCPFn is the injectable seam start.go uses when --wait is set but
// no dbms credential is available (TCP-only fallback). Production wires
// WaitForBoltTCP; tests substitute a deterministic fake so the fallback path
// can be exercised without a real socket. Held alongside waitForBoltFn so the
// test-swap surface stays adjacent to the consumer.
var waitForBoltTCPFn = WaitForBoltTCP

// newStartCmd builds the `neo4j-cli docker start <name>` leaf (REQ-F-040).
// It shells `docker start <name>` via the dockerClient seam after verifying
// the container exists and carries `org.neo4j.cli.managed=true` so we never
// touch containers managed outside neo4j-cli (REQ-F-043).
//
// REQ-F-042 (operating on an ephemeral container that has already been
// removed) collapses into REQ-F-043 in practice: once an ephemeral container
// exits with `--rm`, the daemon drops it and `Inspect` reports "no such
// container" — there is no on-disk state we could use to distinguish "removed
// ephemeral" from "never existed". Emitting the same unknown-name usage error
// keeps the contract honest; the message already points at
// `neo4j-cli docker list`, which surfaces the still-present managed
// containers and makes the next step obvious.
//
// When `--wait` is set, after `docker start` succeeds we poll Bolt readiness
// using the same WaitForBolt / waitTimeout / waitForBoltFn seams `create`
// uses (REQ-F-041). The strongest signal is an authenticated Bolt handshake
// using the stored dbms credential; if no credential is available (the
// container was created with `--no-store-credential`, or credentials are
// managed externally) we fall back to a TCP-only readiness probe instead of
// hard-erroring. The TCP probe is weaker — Neo4j may bind the port briefly
// before accepting Bolt handshakes — but it's strictly better than no wait
// at all, and the operator is told on stderr that authentication was skipped.
func newStartCmd(cfg *clicfg.Config) *cobra.Command {
	var wait bool

	cmd := &cobra.Command{
		Use:         "start <name>",
		Short:       "Start a stopped Neo4j container managed by neo4j-cli",
		Annotations: map[string]string{"write": "true"},
		Long: "Start a stopped Neo4j Docker container by name. " +
			"Only containers carrying `org.neo4j.cli.managed=true` are eligible; " +
			"unknown or unmanaged names return a usage error pointing at `neo4j-cli docker list`. " +
			"Pass --wait to block until the container's Bolt endpoint is reachable (60s timeout). " +
			"When a stored dbms credential exists for the container, --wait performs an authenticated " +
			"Bolt handshake. When no credential is stored (e.g. created with --no-store-credential, or " +
			"managed externally), --wait falls back to a TCP-only probe — weaker (Neo4j may bind the " +
			"port briefly before Bolt is fully ready) but strictly better than no wait. " +
			"Ephemeral containers (`--rm`) are removed by Docker when they stop, so attempting to " +
			"start one after it has exited surfaces the same unknown-name error. " +
			"Daemon-side errors (Docker not running, socket permission denied, etc.) are surfaced " +
			"verbatim and are distinct from the unknown-name error.",
		Example: `# Start a managed container by name
neo4j-cli docker start dev --rw

# Start and block until Bolt accepts sessions before returning
neo4j-cli docker start dev --wait --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			client := clientFactory()
			ctx := cmd.Context()

			// Inspect first so we can refuse non-managed / missing containers
			// before mutating any daemon state (REQ-F-043). Only the
			// "container does not exist" branch maps to unknown-name; other
			// Inspect errors (daemon down, permission denied, …) propagate
			// verbatim so the operator can fix the real cause.
			container, err := client.Inspect(ctx, name)
			if err != nil {
				cmd.SilenceUsage = true
				if errors.Is(err, ErrNotFound) {
					return unknownContainerError(name)
				}
				return err
			}
			if !container.Managed {
				cmd.SilenceUsage = true
				return unknownContainerError(name)
			}

			if err := client.Start(ctx, name); err != nil {
				// dockerClient.Start wraps captured stderr verbatim in a
				// clierr.UsageError (REQ-F-061); surface as-is.
				cmd.SilenceUsage = true
				return err
			}

			if !wait {
				return nil
			}

			// --wait readiness probe.
			// Strongest signal is an authenticated Bolt handshake using the
			// stored credential. If no credential is available (any
			// credentials-store error — miss, corrupt file, etc.) fall back
			// to a TCP-only probe with an explicit stderr announcement so
			// the operator understands the reduced guarantee.
			boltPort, parseErr := strconv.Atoi(container.BoltPort)
			if parseErr != nil {
				cmd.SilenceUsage = true
				return fmt.Errorf("docker start: container %q has unparseable bolt-port label %q: %w", name, container.BoltPort, parseErr)
			}

			var cred *credentials.DbmsCredential
			if cfg.Credentials != nil && cfg.Credentials.Dbms != nil {
				if got, getErr := cfg.Credentials.Dbms.Get(name); getErr == nil && got != nil {
					cred = got
				}
			}

			if cred != nil {
				uri := fmt.Sprintf("neo4j://localhost:%d", boltPort)
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "info: waiting for Bolt on localhost:%d...\n", boltPort)
				if err := waitForBoltFn(ctx, uri, cred.Username, cred.Password, waitTimeout); err != nil {
					cmd.SilenceUsage = true
					return err
				}
				return nil
			}

			// Fallback: TCP-only probe. Loudly announce the reduced
			// guarantee so operators understand "port open != Bolt ready".
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
				"info: no stored credential for %q; falling back to TCP probe on localhost:%d (not authenticated)\n",
				name, boltPort,
			)
			if err := waitForBoltTCPFn(ctx, "localhost", boltPort, waitTimeout); err != nil {
				cmd.SilenceUsage = true
				return err
			}
			return nil
		},
	}

	flags.RegisterWait(cmd, &wait, "Wait until Bolt is reachable before returning.")
	return cmd
}
