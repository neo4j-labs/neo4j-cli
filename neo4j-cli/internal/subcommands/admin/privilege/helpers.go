// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// privilegeExecFn is the package-level test seam. It is set by NewCmd from the
// injected admin.RunAdminStatement and replaced by tests to avoid real Bolt
// connections.
//
//nolint:unused // consumed by leaf commands added in later tasks
var privilegeExecFn adminutil.ExecFn

// privilegeFields are the canonical output columns for SHOW PRIVILEGES output.
// The immutable column returned by some Neo4j versions is excluded.
//
//nolint:unused // consumed by leaf commands added in later tasks
var privilegeFields = []string{"access", "action", "resource", "segment", "role"}

// outputPrivileges executes SHOW ROLE $name PRIVILEGES and prints the result
// with privilegeFields columns. Called after a successful mutation to confirm
// the role's updated privilege list.
//
//nolint:unused // consumed by leaf commands added in later tasks
func outputPrivileges(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, roleName string) error {
	rows, err := privilegeExecFn(cmd.Context(), cfg, conn, "SHOW ROLE $name PRIVILEGES", map[string]any{"name": roleName})
	if err != nil {
		return err
	}
	commonoutput.PrintBodyMap(cmd, cfg, adminutil.NewRows(rows, privilegeFields), privilegeFields)
	return nil
}
