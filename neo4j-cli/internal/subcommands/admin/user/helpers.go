// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package user implements the `neo4j-cli admin user` subcommand tree.
package user

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
)

// userExecFn is the package-level test seam. It must be set to a real
// implementation by the parent command (NewCmd) before any leaf runs.
// Tests replace it to inject fake results without opening a Bolt connection.
var userExecFn adminutil.ExecFn

// userFields is the canonical ordered list of output columns for user records.
var userFields = []string{"user", "roles", "password_change_required", "suspended"}

// normalizeUserRow fills in null values from Community edition with their
// canonical defaults: null roles → []any{}, null suspended → false.
func normalizeUserRow(m map[string]any) map[string]any {
	if m["roles"] == nil {
		m["roles"] = []any{}
	}
	if m["suspended"] == nil {
		m["suspended"] = false
	}
	return m
}

// outputUser fetches the current record for userName and prints it. Called by
// write commands (create, rename, set-password, suspend, activate) after a
// successful mutation.
func outputUser(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, userName string) error {
	rows, err := userExecFn(cmd.Context(), cfg, conn, "SHOW USERS WHERE user = $name", map[string]any{"name": userName})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	commonoutput.PrintBodyMap(cmd, cfg, adminutil.NewRow(normalizeUserRow(rows[0]), userFields), userFields)
	return nil
}

// promptUserPassword reads the operation password from the named flag.
// If the flag value is non-empty it is returned immediately.
// If the flag is empty and stdin is a TTY it prompts "Password: " to stderr
// and reads without echo via dbconn.PasswordReader.
// If the flag is empty and stdin is not a TTY it returns a usage error.
// Calls dbconn.StdinIsTTY and dbconn.PasswordReader (both test-overridable)
// so no new seam vars are declared here.
func promptUserPassword(cmd *cobra.Command, flagName string) (string, error) {
	pw, _ := cmd.Flags().GetString(flagName)
	if pw != "" {
		return pw, nil
	}
	if !dbconn.StdinIsTTY() {
		return "", clierr.NewUsageError("--%s is required or run interactively", flagName)
	}
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
	pw, err := dbconn.PasswordReader()
	_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return pw, nil
}
