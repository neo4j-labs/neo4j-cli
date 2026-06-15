// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// outputDatabase fetches the current record for name and prints it. Called by
// write commands (create, start, stop) after a successful mutation.
func outputDatabase(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, name string) error {
	rows, err := dbExecFn(cmd.Context(), cfg, conn, "SHOW DATABASE $name", map[string]any{"name": name})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	commonoutput.PrintBodyMap(cmd, cfg, adminutil.NewRow(rows[0], getFields), getFields)
	return nil
}
