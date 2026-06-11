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
	commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(rows), listFields)
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
