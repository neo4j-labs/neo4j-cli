// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"fmt"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/cobra"
)

// dbWaitTimeout is the fixed polling budget for --wait. Exposed as a package
// var so tests can shrink it to keep the timeout path fast.
var dbWaitTimeout = 60 * time.Second

// dbWaitInterval is the pause between SHOW DATABASE polls. Exposed as a
// package var so tests can set it to 0 to avoid real sleeps.
var dbWaitInterval = 1 * time.Second

func newCreateCmd(cfg *clicfg.Config, credential *string) *cobra.Command {
	var wait bool

	cmd := &cobra.Command{
		Use:         "create <name>",
		Short:       "Create a database",
		Annotations: map[string]string{"write": "true"},
		Long: "Create a database via CREATE DATABASE <name> IF NOT EXISTS against the system database. " +
			"Uses the dbms credential named by --credential on the parent `admin` command. " +
			"Pass --wait to block until the database status is online (polls every 1 second, 60-second timeout).",
		Example: `# Create a database
neo4j-cli admin database create mydb --credential local --rw

# Create a database and wait until it is online before returning
neo4j-cli admin database create mydb --credential local --wait --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			cred, err := resolveCredential(cfg, credential)
			if err != nil {
				return err
			}
			if _, err := dbExecFn(cmd.Context(), cfg, cred, "CREATE DATABASE $name IF NOT EXISTS", map[string]any{"name": name}); err != nil {
				return err
			}
			if wait {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Waiting for database to come online...\n")
				if err := pollDatabaseStatus(cmd, cfg, cred, name, "online"); err != nil {
					return err
				}
			}
			return nil
		},
	}

	flags.RegisterWait(cmd, &wait, "Wait until the database is online before returning (polls every 1 second, 60-second timeout).")
	return cmd
}

// pollDatabaseStatus polls SHOW DATABASE $name YIELD currentStatus at
// dbWaitInterval intervals until currentStatus equals wantStatus or
// dbWaitTimeout elapses. Returns a UsageError on timeout.
func pollDatabaseStatus(cmd *cobra.Command, cfg *clicfg.Config, cred *credentials.DbmsCredential, name, wantStatus string) error {
	deadline := time.Now().Add(dbWaitTimeout)
	for {
		rows, err := dbExecFn(cmd.Context(), cfg, cred, "SHOW DATABASE $name YIELD currentStatus", map[string]any{"name": name})
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			if status, ok := rows[0]["currentStatus"].(string); ok && status == wantStatus {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return clierr.NewUsageError("timed out waiting for database %q to reach status %q", name, wantStatus)
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-time.After(dbWaitInterval):
		}
	}
}
