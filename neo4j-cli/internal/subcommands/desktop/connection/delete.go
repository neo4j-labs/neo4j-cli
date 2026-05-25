// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package connection

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

// newDeleteCmd builds the `desktop connection delete <id>` leaf. Positional
// `<id>` is UUID-only. The destructive action is gated by the shared confirm
// helper: both --yes and --force are required for non-TTY callers; TTY
// callers get a y/N prompt unless they pre-confirm with both flags. Desktop
// owns the saved connection's credential lifecycle.
func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a saved remote DB connection registered with Neo4j Desktop 2",
		Long: "Delete a saved remote DB connection profile by id. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"Destructive: requires `--yes --force` (or a `y` answer at the TTY prompt) when invoked non-interactively. " +
			"Desktop owns the saved connection's credential lifecycle — the `connection:<id>` safeStorage entry is removed by Desktop as part of the DELETE; this leaf does NOT mutate `~/.neo4j/cli/credentials.json`. " +
			"Find connection ids with `neo4j-cli desktop list`.",
		Example: `# Delete a saved connection with an interactive y/N confirmation
neo4j-cli desktop connection delete f4e2f3c0-1111-2222-3333-444455556666 --rw

# Delete a saved connection without prompting (scripts, CI, non-TTY shells)
neo4j-cli desktop connection delete f4e2f3c0-1111-2222-3333-444455556666 --yes --force --rw

# Delete a saved connection and emit a machine-readable confirmation for scripting
neo4j-cli desktop connection delete f4e2f3c0-1111-2222-3333-444455556666 --yes --force --format json --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE:        nil,
	}

	confirmFlags := confirm.Register(cmd)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		id := args[0]
		if _, err := uuid.Parse(id); err != nil {
			return clierr.NewUsageError(
				"connection id must be a UUID; got %q. "+
					"Run 'neo4j-cli desktop list' to see connection ids.", id)
		}

		ctx := cmd.Context()
		fs := cfg.Aura.Fs()
		port, _ := cmd.Flags().GetInt("port")

		client, err := newDesktopClientFn(ctx, fs, port)
		if err != nil {
			return err
		}

		if err := confirmFlags.Require(cmd, id); err != nil {
			if errors.Is(err, confirm.ErrCancelled) {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "cancelled.")
				return nil
			}
			return err
		}

		deleted, err := client.DeleteConnection(ctx, id)
		if err != nil {
			return err
		}

		name := ""
		if deleted != nil {
			name = deleted.Name
		}
		displayName := name
		if displayName == "" {
			displayName = id
		}

		switch output.ResolveOutput(cmd, cfg) {
		case "json":
			payload := struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Deleted bool   `json:"deleted"`
			}{ID: id, Name: name, Deleted: true}
			buf, jerr := json.MarshalIndent(payload, "", "\t")
			if jerr != nil {
				return jerr
			}
			cmd.Println(string(buf))
		default:
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deleted connection %q (%s).\n", displayName, id)
		}
		return nil
	}

	return cmd
}
