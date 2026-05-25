// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"errors"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/spf13/cobra"
)

// missingCredentialErrorPrefix mirrors credentials.DbmsCredentials.Remove's
// "could not find credential with name <n> to remove" wording. The leaf
// swallows that exact shape per REQ-F-050 (missing credential during delete
// is not an error) while still surfacing any other failure verbatim.
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
	cmd := &cobra.Command{
		Use:         "delete <name>",
		Short:       "Remove a Neo4j container and its dbms credential",
		Annotations: map[string]string{"write": "true"},
		Long: "Remove a Neo4j Docker container by name and best-effort delete its stored dbms credential. " +
			"Only containers carrying `org.neo4j.cli.managed=true` are eligible; unknown or unmanaged names " +
			"return a usage error pointing at `neo4j-cli docker list`. " +
			"Destructive: requires `--yes --force` (or a `y` answer at the TTY prompt) when invoked non-interactively. " +
			"A missing dbms credential is NOT an error — the container is still removed. " +
			"Daemon-side errors (Docker not running, socket permission denied, etc.) are surfaced verbatim " +
			"and are distinct from the unknown-name error.",
		Example: `# Delete a managed container; prompts on a TTY
neo4j-cli docker delete dev --rw

# Skip the prompt (required for scripts / non-TTY callers)
neo4j-cli docker delete dev --yes --force --rw

# Delete and confirm by listing remaining managed containers
neo4j-cli docker delete dev --yes --force --rw && neo4j-cli docker list --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			client := clientFactory()
			ctx := cmd.Context()

			// Inspect first so we can refuse non-managed / missing containers
			// before mutating any daemon state (REQ-F-053). Only the
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

			cmd.SilenceUsage = true
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
			if cfg.Credentials != nil && cfg.Credentials.Dbms != nil {
				if err := cfg.Credentials.Dbms.Remove(name); err != nil {
					if !strings.HasPrefix(err.Error(), missingCredentialErrorPrefix) {
						return err
					}
				}
			}

			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
