// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var deleteStdinIsTTYFn = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// SetDeleteStdinIsTTYFnForTest overrides the TTY detector for tests.
func SetDeleteStdinIsTTYFnForTest(fn func() bool) func() {
	prev := deleteStdinIsTTYFn
	deleteStdinIsTTYFn = fn
	return func() { deleteStdinIsTTYFn = prev }
}

func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	var yes bool
	const yesFlag = "yes"

	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a DBMS managed by the local Neo4j Desktop 2 install",
		Long: "Delete a DBMS managed by the local Neo4j Desktop 2 install. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"On an interactive terminal the command prompts for confirmation before deleting; pass `--yes` to skip the prompt. " +
			"When stdin is not a TTY (CI / piped input) the command refuses to delete without `--yes` so accidental deletions cannot happen silently. " +
			"This leaf does NOT mutate `~/.neo4j/cli/credentials.json` — Desktop owns the credential lifecycle (REQ-F-025); " +
			"any persisted neo4j-cli credential pointing at this DBMS is left intact and must be cleaned up via `credential dbms remove`.",
		Example: `# Delete a DBMS with an interactive y/N confirmation
neo4j-cli desktop dbms delete my-dbms-id --rw

# Delete a DBMS without prompting (scripts, CI, non-TTY shells)
neo4j-cli desktop dbms delete my-dbms-id --yes --rw

# Delete a DBMS and emit a machine-readable confirmation for scripting
neo4j-cli desktop dbms delete my-dbms-id --yes --format json --rw`,
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

			if !yes {
				// Non-TTY without --yes is a hard usage error; silent deletes
				// in scripts are a worse failure mode than a noisy reminder.
				if !deleteStdinIsTTYFn() {
					return clierr.NewUsageError(
						"refusing to delete DBMS %q: stdin is not a terminal and --yes was not provided. "+
							"Re-run with --yes to confirm non-interactively.",
						id)
				}

				// Fetch first so the prompt can name the DBMS; a 404 here
				// surfaces with the canonical error mapping.
				info, gerr := client.GetDbms(ctx, id)
				if gerr != nil {
					return gerr
				}

				name := info.Name
				if name == "" {
					name = id
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Delete DBMS %q (%s)? [y/N] ", name, id)
				reader := bufio.NewReader(cmd.InOrStdin())
				line, rerr := reader.ReadString('\n')
				if rerr != nil && line == "" {
					// EOF before any input ⇒ abort (no silent delete when
					// `</dev/null` is piped into a TTY-detected shell).
					return clierr.NewUsageError(
						"refusing to delete DBMS %q: no confirmation received on stdin",
						id)
				}
				answer := strings.ToLower(strings.TrimSpace(line))
				if answer != "y" && answer != "yes" {
					return clierr.NewUsageError("delete aborted: confirmation declined")
				}
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

	cmd.Flags().BoolVar(&yes, yesFlag, false, "Skip the interactive y/N confirmation; required when stdin is not a terminal")
	return cmd
}
