// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// stdinIsTerminal is the test seam for TTY detection on stdin. Production
// calls `term.IsTerminal` on os.Stdin's file descriptor; tests override the
// var so the prompt / non-TTY branches can be exercised deterministically
// without standing up a real PTY.
var stdinIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// missingCredentialErrorPrefix mirrors credentials.DbmsCredentials.Remove's
// "could not find credential with name <n> to remove" wording. The leaf
// swallows that exact shape per REQ-F-050 (missing credential during delete
// is not an error) while still surfacing any other failure verbatim.
const missingCredentialErrorPrefix = "could not find credential with name"

// newDeleteCmd builds the `neo4j-cli docker delete <name>` leaf (REQ-F-050..F-054).
// It refuses non-managed containers (same unknown-name hint as get/start/stop),
// optionally prompts for confirmation when stdin is a TTY, and then shells
// `docker rm -f <name>` via the dockerClient seam followed by a best-effort
// removal of the stored dbms credential.
//
// Behaviour matrix:
//   - TTY caller, no --force → "Delete container <name> and its dbms credential? [y/N]"
//     prompt; only `y` / `Y` / `yes` confirms (default N → cancelled, exit 0).
//   - non-TTY caller, no --force → clierr.NewUsageError per REQ-F-052; nothing
//     is touched. Scripts MUST pass --force to confirm deletion.
//   - --force (TTY or non-TTY) → skip the prompt, proceed straight to delete.
//
// Per REQ-F-050, a missing dbms credential is NOT an error — the container
// removal still succeeded, the credential just wasn't stored. Any OTHER
// credential-removal error is surfaced verbatim.
func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:         "delete <name>",
		Short:       "Remove a Neo4j container and its dbms credential",
		Annotations: map[string]string{"write": "true"},
		Long: "Remove a Neo4j Docker container by name and best-effort delete its stored dbms credential. " +
			"Only containers carrying `org.neo4j.cli.managed=true` are eligible; unknown or unmanaged names " +
			"return a usage error pointing at `neo4j-cli docker list`. " +
			"On a TTY, you are prompted to confirm before deletion; non-TTY callers MUST pass --force to " +
			"confirm. A missing dbms credential is NOT an error — the container is still removed. " +
			"Daemon-side errors (Docker not running, socket permission denied, etc.) are surfaced verbatim " +
			"and are distinct from the unknown-name error.",
		Example: `# Delete a managed container; prompts on a TTY
neo4j-cli docker delete dev --rw

# Skip the prompt (required for scripts / non-TTY callers)
neo4j-cli docker delete dev --force --rw

# Delete and confirm by listing remaining managed containers
neo4j-cli docker delete dev --force --rw && neo4j-cli docker list --format json`,
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

			if !force {
				if !stdinIsTerminal() {
					// REQ-F-052: scripts / piped callers MUST opt in explicitly.
					cmd.SilenceUsage = true
					return clierr.NewUsageError("non-TTY caller must pass --force to confirm deletion")
				}
				confirmed, err := promptForDelete(cmd.InOrStdin(), cmd.ErrOrStderr(), name)
				if err != nil {
					cmd.SilenceUsage = true
					return err
				}
				if !confirmed {
					// Cancelled by the user — exit 0 cleanly, neither the
					// container nor the credential is touched.
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "cancelled.")
					return nil
				}
			}

			if err := client.RemoveForce(ctx, name); err != nil {
				// dockerClient.RemoveForce wraps captured stderr verbatim in
				// a clierr.UsageError (REQ-F-061); surface as-is.
				cmd.SilenceUsage = true
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
						cmd.SilenceUsage = true
						return err
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip the TTY confirmation prompt. Required for non-TTY callers.")
	return cmd
}

// promptForDelete writes the REQ-F-051 confirmation prompt to stderr and
// reads a single line from stdin. The default answer is N: only an exact
// `y`, `Y`, or `yes` (case-insensitive) confirms; an empty line or anything
// else cancels. A read error before any input bytes is treated as cancel.
func promptForDelete(in io.Reader, errOut io.Writer, name string) (bool, error) {
	_, _ = fmt.Fprintf(errOut, "Delete container %s and its dbms credential? [y/N] ", name)
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		// EOF or read error with no bytes — treat as cancel rather than as
		// an error so a stdin that closes mid-prompt doesn't look like a
		// docker failure.
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
