// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// roleExecFn is the package-level test seam. It is set by NewCmd in production
// and replaced by tests to inject fake results without a real Bolt connection.
var roleExecFn adminutil.ExecFn

// normalizeRoleRow normalizes a role member record row for rendering:
//   - nil member → "" (renders cleanly instead of null in JSON/table)
func normalizeRoleRow(row map[string]any) map[string]any {
	out := make(map[string]any, len(row))
	for k, v := range row {
		out[k] = v
	}
	if out["member"] == nil {
		out["member"] = ""
	}
	return out
}

// outputRoleMembers fetches the SHOW ROLES WITH USERS WHERE role=$name result
// and prints it. Zero rows is valid (role exists but has no members). Called
// by write commands (create) after a successful mutation.
func outputRoleMembers(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, roleName string) error {
	rows, err := roleExecFn(cmd.Context(), cfg, conn,
		"SHOW ROLES WITH USERS WHERE role = $name",
		map[string]any{"name": roleName},
	)
	if err != nil {
		return err
	}
	normalized := make([]map[string]any, len(rows))
	for i, row := range rows {
		normalized[i] = normalizeRoleRow(row)
	}
	commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(normalized), listFields)
	return nil
}

// outputUserRoles fetches SHOW USERS WHERE user=$user and prints it using the
// user package's field list. Called by grant/revoke after a successful mutation.
func outputUserRoles(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, userName string) error {
	rows, err := roleExecFn(cmd.Context(), cfg, conn,
		"SHOW USERS WHERE user = $name",
		map[string]any{"name": userName},
	)
	if err != nil {
		return err
	}
	userFields := []string{"user", "roles", "passwordChangeRequired", "suspended"}
	commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(rows), userFields)
	return nil
}
