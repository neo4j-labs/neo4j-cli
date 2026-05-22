// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package connection

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

// newDeleteCmd builds the `desktop connection delete <id>` leaf. Positional
// `<id>` is UUID-only. `--yes` skips the prompt; on a TTY without `--yes` the
// user is prompted with a human-readable name resolved via ListConnections
// (relate has no GET /connections/:id); on a non-TTY without `--yes` the
// command refuses to delete.
func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	var yes bool
	const yesFlag = "yes"

	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a saved remote DB connection registered with Neo4j Desktop 2",
		Long: "Delete a saved remote DB connection profile by id. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"On an interactive terminal the command prompts for confirmation before deleting; pass `--yes` to skip the prompt. " +
			"When stdin is not a TTY (CI / piped input) the command refuses to delete without `--yes` so accidental deletions cannot happen silently. " +
			"Desktop owns the saved connection's credential lifecycle — the `connection:<id>` safeStorage entry is removed by Desktop as part of the DELETE; this leaf does NOT mutate `~/.neo4j/cli/credentials.json`. " +
			"Find connection ids with `neo4j-cli desktop list`.",
		Example: `# Delete a saved connection with an interactive y/N confirmation
neo4j-cli desktop connection delete f4e2f3c0-1111-2222-3333-444455556666 --rw

# Delete a saved connection without prompting (scripts, CI, non-TTY shells)
neo4j-cli desktop connection delete f4e2f3c0-1111-2222-3333-444455556666 --yes --rw

# Delete a saved connection and emit a machine-readable confirmation for scripting
neo4j-cli desktop connection delete f4e2f3c0-1111-2222-3333-444455556666 --yes --format json --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			if !yes {
				if !stdinIsTTYFn() {
					return clierr.NewUsageError(
						"refusing to delete connection %q: stdin is not a terminal and --yes was not provided. "+
							"Re-run with --yes to confirm non-interactively.",
						id)
				}

				// Resolve the connection name for the prompt label by listing
				// connections (relate has no GET /connections/:id). If the id
				// is missing from the list we fall through to a name-less
				// prompt rather than blocking the delete.
				name := ""
				if conns, lerr := client.ListConnections(ctx); lerr == nil {
					for _, c := range conns {
						if c.ID == id {
							name = c.Name
							break
						}
					}
				}

				if name != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Delete connection %q (%s)? [y/N] ", name, id)
				} else {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Delete connection (%s)? [y/N] ", id)
				}
				reader := bufio.NewReader(cmd.InOrStdin())
				line, rerr := reader.ReadString('\n')
				if rerr != nil && line == "" {
					// EOF before any input on a TTY is treated as "abort".
					return clierr.NewUsageError(
						"refusing to delete connection %q: no confirmation received on stdin",
						id)
				}
				answer := strings.ToLower(strings.TrimSpace(line))
				if answer != "y" && answer != "yes" {
					return clierr.NewUsageError("delete aborted: confirmation declined")
				}
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
		},
	}

	cmd.Flags().BoolVar(&yes, yesFlag, false, "Skip the interactive y/N confirmation; required when stdin is not a terminal")
	return cmd
}
