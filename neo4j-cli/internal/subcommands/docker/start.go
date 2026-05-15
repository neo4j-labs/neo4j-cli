// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"errors"
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/cobra"
)

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
// uses (REQ-F-041). Authenticating the probe requires the dbms credential
// stored at create time — if no credential exists for this container the
// leaf returns a usage error rather than silently degrading.
func newStartCmd(cfg *clicfg.Config) *cobra.Command {
	var wait bool

	cmd := &cobra.Command{
		Use:         "start <name>",
		Short:       "Start a stopped Neo4j container managed by neo4j-cli",
		Annotations: map[string]string{"write": "true"},
		Long: "Start a stopped Neo4j Docker container by name. " +
			"Only containers carrying `org.neo4j.cli.managed=true` are eligible; " +
			"unknown or unmanaged names return a usage error pointing at `neo4j-cli docker list`. " +
			"Pass --wait to block until the container's Bolt endpoint accepts sessions (60s timeout); " +
			"--wait requires a stored dbms credential for the container (the credential supplies " +
			"the password used to authenticate the readiness probe). " +
			"Ephemeral containers (`--rm`) are removed by Docker when they stop, so attempting to " +
			"start one after it has exited surfaces the same unknown-name error. " +
			"Daemon-side errors (Docker not running, socket permission denied, etc.) are surfaced " +
			"verbatim and are distinct from the unknown-name error.",
		Example: `# Start a managed container by name
neo4j-cli docker start dev --rw

# Start and block until Bolt accepts sessions before returning
neo4j-cli docker start dev --wait --rw

# Same as above using the deprecated --await alias
neo4j-cli docker start dev --await --rw`,
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

			// --wait: poll Bolt readiness against the inspected bolt port using
			// the stored dbms credential. The probe needs a password we never
			// labeled (REQ-NF-004), so a missing credential is a hard usage
			// error rather than a TCP-only fallback. The operator can either
			// drop --wait, or re-create the container without --no-store-credential.
			if cfg.Credentials == nil || cfg.Credentials.Dbms == nil {
				return clierr.NewUsageError(
					"--wait on 'docker start' requires a stored dbms credential for %q; either re-create with credentials stored, or omit --wait",
					name,
				)
			}
			cred, credErr := cfg.Credentials.Dbms.Get(name)
			if credErr != nil || cred == nil {
				return clierr.NewUsageError(
					"--wait on 'docker start' requires a stored dbms credential for %q; either re-create with credentials stored, or omit --wait",
					name,
				)
			}

			uri := fmt.Sprintf("neo4j://localhost:%s", container.BoltPort)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "info: waiting for Bolt on localhost:%s...\n", container.BoltPort)
			if err := waitForBoltFn(ctx, uri, cred.Username, cred.Password, waitTimeout); err != nil {
				cmd.SilenceUsage = true
				return err
			}
			return nil
		},
	}

	flags.RegisterWait(cmd, &wait, "Wait until Bolt is reachable before returning.")
	return cmd
}
