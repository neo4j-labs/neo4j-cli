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

var dbmsStartFields = []string{"id", "name", "version", "status", "connection_uri"}

// dbmsStartResult adapts an optional `*DbmsInfo` to output.ResponseData.
// Without `--wait` the start endpoint reply is opaque to the CLI, so the row
// carries only `id`; `--wait` upgrades to the full DbmsInfo.
type dbmsStartResult struct {
	ID   string
	Item *desktopclient.DbmsInfo
}

func (r dbmsStartResult) AsArray() []map[string]any {
	if r.Item != nil {
		return []map[string]any{
			{
				"id":             r.Item.ID,
				"name":           r.Item.Name,
				"version":        r.Item.Version,
				"status":         r.Item.Status,
				"connection_uri": r.Item.ConnectionURI,
			},
		}
	}
	return []map[string]any{{"id": r.ID}}
}

// MarshalJSON emits the snake_case DbmsInfo projection on the --wait path or a
// minimal `{"id": "..."}` envelope otherwise (stable shape for scripts keying off id).
func (r dbmsStartResult) MarshalJSON() ([]byte, error) {
	if r.Item != nil {
		return json.Marshal(r.Item.ToOutput())
	}
	return json.Marshal(map[string]string{"id": r.ID})
}

func newStartCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		wait  bool
		force bool
	)
	const (
		waitFlag  = "wait"
		forceFlag = "force"
	)

	cmd := &cobra.Command{
		Use:   "start <id>",
		Short: "Start a DBMS managed by the local Neo4j Desktop 2 install",
		Long: "Start a DBMS managed by the local Neo4j Desktop 2 install. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"By default the command returns as soon as Desktop accepts the start request; the DBMS may still be booting. " +
			"Pass `--wait` to poll every 1s for up to 30s until `status=started`; on timeout the command exits non-zero with the last-seen status. " +
			"A pre-flight check refuses to start a second DBMS when another is already running, since Neo4j Desktop 2 runs one DBMS at a time on port 7687; pass `--force` to stop the conflicting DBMS first and then proceed. " +
			"This leaf does NOT mutate `~/.neo4j/cli/credentials.json` — Desktop owns the credential lifecycle.",
		Example: `# Start a DBMS and return immediately (do not wait for boot)
neo4j-cli desktop dbms start my-dbms-id --rw

# Start a DBMS and wait until it reports status=started (30s ceiling)
neo4j-cli desktop dbms start my-dbms-id --wait --rw

# Stop any other running DBMS first to free port 7687, then start this one
neo4j-cli desktop dbms start my-dbms-id --force --rw

# Start a DBMS and emit the resolved DbmsInfo as JSON for scripting
neo4j-cli desktop dbms start my-dbms-id --wait --format json --rw`,
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
			if !force {
				// Desktop's relate API silently no-ops a second start, so
				// without this check `--wait` would time out 30s later with a
				// confusing "last status: stopped". Passing `id` as selfID
				// keeps restart of an already-running DBMS idempotent.
				if err := assertNoOtherRunning(ctx, client, id, "", "start", cmd.ErrOrStderr()); err != nil {
					return err
				}
			} else {
				// Stop the conflicting DBMS first so the subsequent start
				// actually succeeds (otherwise Desktop silently no-ops it).
				// selfID=id avoids stopping the target itself.
				if _, err := resolveConflicting(ctx, client, id, cmd.ErrOrStderr()); err != nil {
					return err
				}
			}
			if err := client.StartDbms(ctx, id); err != nil {
				return err
			}

			result := dbmsStartResult{ID: id}
			if wait {
				info, perr := pollUntilStatus(ctx, client, id, dbmsStatusStarted)
				if perr != nil {
					return perr
				}
				result.Item = info
			} else {
				// Desktop's start returns stringified shell output, not a
				// DbmsInfo. One follow-up GET enriches the row; on failure
				// fall back to the slim envelope so exit stays 0.
				if info, ok := fetchForRender(ctx, client, id, cmd.ErrOrStderr()); ok {
					result.Item = info
				}
			}

			output.PrintBodyMap(cmd, cfg, result, dbmsStartFields)
			return nil
		},
	}

	cmd.Flags().BoolVar(&wait, waitFlag, false, "Poll every 1s for up to 30s until the DBMS reports status=started; exits non-zero on timeout")
	cmd.Flags().BoolVar(&force, forceFlag, false, "Stop any other running Desktop DBMS first to free port 7687, then proceed. Without --force, the command refuses when another DBMS is running.")
	return cmd
}
