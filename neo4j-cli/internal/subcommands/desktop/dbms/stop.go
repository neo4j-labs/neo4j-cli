// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"encoding/json"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

var dbmsStopFields = []string{"id", "name", "version", "status", "connectionUri"}

// dbmsStopResult adapts an optional `*DbmsInfo` to output.ResponseData.
// Without `--wait` the stop reply is opaque to the CLI, so the row carries
// only `id`; `--wait` upgrades to the full DbmsInfo.
type dbmsStopResult struct {
	ID   string
	Item *desktopclient.DbmsInfo
}

func (r dbmsStopResult) AsArray() []map[string]any {
	if r.Item != nil {
		return []map[string]any{
			{
				"id":            r.Item.ID,
				"name":          r.Item.Name,
				"version":       r.Item.Version,
				"status":        r.Item.Status,
				"connectionUri": r.Item.ConnectionURI,
			},
		}
	}
	return []map[string]any{{"id": r.ID}}
}

// MarshalJSON emits the full DbmsInfo on the --wait path or a minimal
// `{"id": "..."}` envelope otherwise (stable shape: `id` is always present).
func (r dbmsStopResult) MarshalJSON() ([]byte, error) {
	if r.Item != nil {
		return json.Marshal(r.Item)
	}
	return json.Marshal(map[string]string{"id": r.ID})
}

func newStopCmd(cfg *clicfg.Config) *cobra.Command {
	var wait bool
	const waitFlag = "wait"

	cmd := &cobra.Command{
		Use:   "stop <id>",
		Short: "Stop a DBMS managed by the local Neo4j Desktop 2 install",
		Long: "Stop a DBMS managed by the local Neo4j Desktop 2 install. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"By default the command returns as soon as Desktop accepts the stop request; the DBMS may still be draining. " +
			"Pass `--wait` to poll every 1s for up to 30s until `status=stopped`; on timeout the command exits non-zero with the last-seen status. " +
			"This leaf does NOT mutate `~/.neo4j/cli/credentials.json` — Desktop owns the credential lifecycle.",
		Example: `# Stop a DBMS and return immediately (do not wait for shutdown)
neo4j-cli desktop dbms stop my-dbms-id --rw

# Stop a DBMS and wait until it reports status=stopped (30s ceiling)
neo4j-cli desktop dbms stop my-dbms-id --wait --rw

# Stop a DBMS and emit the resolved DbmsInfo as JSON for scripting
neo4j-cli desktop dbms stop my-dbms-id --wait --format json --rw`,
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
			if err := client.StopDbms(ctx, id); err != nil {
				return err
			}

			result := dbmsStopResult{ID: id}
			if wait {
				info, perr := pollUntilStatus(ctx, client, id, dbmsStatusStopped)
				if perr != nil {
					return perr
				}
				result.Item = info
			} else {
				// Desktop's stop returns stringified shell output, not a
				// DbmsInfo. One follow-up GET enriches the row; on failure
				// fall back to the slim envelope so exit stays 0.
				if info, ok := fetchForRender(ctx, client, id, cmd.ErrOrStderr()); ok {
					result.Item = info
				}
			}

			output.PrintBodyMap(cmd, cfg, result, dbmsStopFields)
			return nil
		},
	}

	cmd.Flags().BoolVar(&wait, waitFlag, false, "Poll every 1s for up to 30s until the DBMS reports status=stopped; exits non-zero on timeout")
	return cmd
}
