// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// userExecFn is the package-level test seam. It is set by NewCmd in production
// and replaced by tests to inject fake results without a real Bolt connection.
var userExecFn adminutil.ExecFn

// promptPassword reads a password from the controlling terminal with no echo,
// or returns a clear usage error when stdin is not a TTY (so scripted use
// must supply the password via flag).
func promptPassword(cmd *cobra.Command) (string, error) {
	pwFlag := cmd.Flags().Lookup("password")
	if pwFlag != nil && pwFlag.Changed {
		return pwFlag.Value.String(), nil
	}
	if !dbconn.StdinIsTTY() {
		return "", clierr.NewUsageError(
			"password is required: set --password or run interactively")
	}
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
	pw, err := dbconn.PasswordReader()
	_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("admin user: read password: %w", err)
	}
	return pw, nil
}

// outputUser fetches the current record for name and prints it using the same
// field list as the read commands (list/get). Called by write commands after
// a successful mutation.
func outputUser(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, name string) error {
	rows, err := userExecFn(cmd.Context(), cfg, conn, "SHOW USERS WHERE user = $name", map[string]any{"name": name})
	if err != nil {
		return err
	}
	commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(rows), listFields)
	return nil
}
