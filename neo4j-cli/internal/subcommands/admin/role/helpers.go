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

// roleExecFn is the package-level test seam. It is set by NewCmd from the
// injected admin.RunAdminStatement and replaced by tests to avoid real Bolt
// connections.
var roleExecFn adminutil.ExecFn

// roleFields are the canonical output columns for SHOW ROLES WITH USERS output.
var roleFields = []string{"role", "member"}

// normalizeRoleRow ensures the "member" key is never nil in a SHOW ROLES WITH
// USERS result row. Neo4j returns nil for roles that have no members; this
// converts that to an empty string for consistent JSON output.
func normalizeRoleRow(m map[string]any) map[string]any {
	if m["member"] == nil {
		m["member"] = ""
	}
	return m
}

// outputRoleMembers executes SHOW ROLES WITH USERS WHERE role = $name, normalizes
// each result row, and prints with roleFields columns.
func outputRoleMembers(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, roleName string) error {
	rows, err := roleExecFn(cmd.Context(), cfg, conn, "SHOW ROLES WITH USERS WHERE role = $name", map[string]any{"name": roleName})
	if err != nil {
		return err
	}
	for i, row := range rows {
		rows[i] = normalizeRoleRow(row)
	}
	commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(rows), roleFields)
	return nil
}

// outputUserAfterRoleChange executes SHOW USERS WHERE user = $name and prints
// the matching user record. Called after a successful grant or revoke to confirm
// the updated role membership. Returns nil without printing if the user is not
// found (unlikely in practice, but safe).
//
//nolint:unused
func outputUserAfterRoleChange(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, userName string) error {
	userFields := []string{"user", "roles", "password_change_required", "suspended"}
	rows, err := roleExecFn(cmd.Context(), cfg, conn, "SHOW USERS WHERE user = $name", map[string]any{"name": userName})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	row := rows[0]
	if row["roles"] == nil {
		row["roles"] = []any{}
	}
	if row["suspended"] == nil {
		row["suspended"] = false
	}
	commonoutput.PrintBodyMap(cmd, cfg, adminutil.NewRow(row, userFields), userFields)
	return nil
}
