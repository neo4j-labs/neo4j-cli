// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a DBMS managed by the local Neo4j Desktop 2 install",
		Long: "Delete a DBMS managed by the local Neo4j Desktop 2 install. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"Destructive: requires `--yes --force` (or a `y` answer at the TTY prompt) when invoked non-interactively. " +
			"This leaf does NOT mutate `~/.neo4j/cli/credentials.json` — Desktop owns the credential lifecycle (REQ-F-025); " +
			"any persisted neo4j-cli credential pointing at this DBMS is left intact and must be cleaned up via `credential dbms remove`.",
		Example: `# Delete a DBMS with an interactive y/N confirmation
neo4j-cli desktop dbms delete my-dbms-id --rw

# Delete a DBMS without prompting (scripts, CI, non-TTY shells)
neo4j-cli desktop dbms delete my-dbms-id --yes --force --rw

# Delete a DBMS and emit a machine-readable confirmation for scripting
neo4j-cli desktop dbms delete my-dbms-id --yes --force --format json --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			fs := cfg.Aura.Fs()
			port, _ := cmd.Flags().GetInt(portFlag)
			id := args[0]

			client, err := newDesktopClientFn(ctx, fs, port)
			if err != nil {
				return err
			}

			if err := confirm.Require(cmd, id); err != nil {
				if errors.Is(err, confirm.ErrCancelled) {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "cancelled.")
					return nil
				}
				return err
			}

			deleted, err := client.DeleteDbms(ctx, id)
			if err != nil {
				return err
			}

			// Desktop's DELETE returns a snapshot whose `status` reflects the
			// pre-delete state (e.g. "stopped") — confusing for a deleted row.
			// Emit a confirmation shape: one line on table/toon, `{id, name,
			// deleted: true}` on JSON.
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
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deleted DBMS %q (%s).\n", displayName, id)
			}
			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
