// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"errors"
	"fmt"
	"github.com/neo4j/cli/internal/clierr"
	engine "github.com/neo4j/cli/internal/docker"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/neo4j/cli/internal/debug"
	"github.com/spf13/cobra"
)

// missingCredentialErrorPrefix mirrors credentials.DbmsCredentials.Get's
// "could not find credential with name" wording. The leaf swallows that exact
// shape per REQ-F-050 (missing credential during delete is not an error) while
// still surfacing any other failure verbatim.
const missingCredentialErrorPrefix = "could not find credential with name"

// newDeleteCmd builds the `neo4j-cli docker delete <name>` leaf (REQ-F-050..F-054).
// It refuses non-managed containers (same unknown-name hint as get/start/stop),
// gates the destructive action via the shared confirm helper (both --yes and
// --force are required for non-TTY callers), then shells `docker rm -f <name>`
// via the dockerClient seam followed by a best-effort removal of the stored
// dbms credential.
//
// Per REQ-F-050, a missing dbms credential is NOT an error — the container
// removal still succeeded, the credential just wasn't stored. Any OTHER
// credential-removal error is surfaced verbatim.
func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	var removeVolume bool

	cmd := &cobra.Command{
		Use:         "delete <name>",
		Short:       "Remove a Neo4j container and its dbms credential",
		Annotations: map[string]string{"write": "true"},
		Long: "Remove a Neo4j Docker container by name and best-effort delete its stored dbms credential. " +
			"Only containers carrying `org.neo4j.cli.managed=true` are eligible; unknown or unmanaged names " +
			"return a usage error pointing at `neo4j-cli docker list`. " +
			"Destructive: requires `--yes --force` (or a `y` answer at the TTY prompt) when invoked non-interactively. " +
			"A missing dbms credential is NOT an error — the container is still removed. " +
			"A container made by `docker load` keeps its loaded database in a named data volume (`neo4j-cli-<name>-data`) that outlives the container. " +
			"Pass --remove-volume to remove that volume as well; on a TTY you are asked (default no); with `--yes --force` and no --remove-volume it is kept and the command to remove it later is printed. " +
			"Only that CLI-created volume is ever removed — never a volume you attached yourself. " +
			"Daemon-side errors (Docker not running, socket permission denied, etc.) are surfaced verbatim " +
			"and are distinct from the unknown-name error.",
		Example: `# Delete a managed container; prompts on a TTY
neo4j-cli docker delete dev --rw

# Skip the prompt (required for scripts / non-TTY callers)
neo4j-cli docker delete dev --yes --force --rw

# Also remove the data volume a loaded container left behind
neo4j-cli docker delete movies --remove-volume --yes --force --rw

# Delete and confirm by listing remaining managed containers
neo4j-cli docker delete dev --yes --force --rw && neo4j-cli docker list --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			name := args[0]
			client := clientFactory(debug.Resolve(cmd))
			ctx := cmd.Context()

			// Inspect first so we can refuse non-managed / missing containers
			// before mutating any daemon state (REQ-F-053). Only the
			// "container does not exist" branch maps to unknown-name; other
			// Inspect errors (daemon down, permission denied, …) propagate
			// verbatim so the operator can fix the real cause.
			container, err := client.Inspect(ctx, name)
			if err != nil {
				if errors.Is(err, engine.ErrNotFound) {
					return unknownContainerError(name)
				}
				return err
			}
			if !container.Managed {
				return unknownContainerError(name)
			}

			if err := confirm.Require(cmd, name); err != nil {
				return err
			}

			if err := client.RemoveForce(ctx, name); err != nil {
				// dockerClient.RemoveForce wraps captured stderr verbatim in
				// a clierr.UsageError (REQ-F-061); surface as-is.
				return err
			}

			// REQ-F-050: best-effort credential removal. A missing credential
			// is NOT an error — the container went away successfully, the
			// credential just wasn't stored (e.g. --no-store-credential at
			// create time, or it was already removed manually). Any other
			// failure shape is surfaced verbatim.
			if cfg.Credentials != nil {
				if err := cfg.Credentials.RemoveDbms(name, cmd.ErrOrStderr()); err != nil {
					if !strings.HasPrefix(err.Error(), missingCredentialErrorPrefix) {
						return err
					}
				}
			}

			// The data volume `docker load` created outlives the container. It
			// holds the loaded database, so removing it is a separate, opt-in
			// step: --remove-volume, or a y/N offer (default no) on a TTY.
			if volume, ok := engine.ManagedDataVolume(container); ok {
				return settleDataVolume(cmd, client, name, volume, removeVolume)
			}

			return nil
		},
	}

	confirm.Register(cmd)
	cmd.Flags().BoolVar(&removeVolume, "remove-volume", false, "Also remove the container's CLI-created data volume (neo4j-cli-<name>-data), which holds a loaded database. Without it the volume is kept (a TTY is asked; the removal command is printed otherwise).")

	return cmd
}

// settleDataVolume removes the container's data volume when asked to — by flag,
// or by a TTY answering the offer — and otherwise keeps it and says how to
// remove it later. The container is already gone by now, so a failure here is
// reported as a partial success, not a failed delete.
func settleDataVolume(cmd *cobra.Command, client engine.Client, container, volume string, flagged bool) error {
	remove := flagged
	if !remove && !confirm.Scripted(cmd) {
		remove = confirm.Ask(cmd, fmt.Sprintf("Also remove the data volume %q (it holds the loaded database)?", volume))
	}

	if !remove {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"info: kept data volume %q (it holds the loaded database); remove it with `docker volume rm %s` or delete with --remove-volume next time\n", volume, volume)
		return nil
	}

	if err := client.RemoveVolume(cmd.Context(), volume); err != nil {
		cmd.SilenceUsage = true
		return clierr.NewFatalError("container %q was removed but its data volume %q could not be: %w. Remove it with `docker volume rm %s`", container, volume, err, volume)
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "info: removed data volume %q\n", volume)
	return nil
}
