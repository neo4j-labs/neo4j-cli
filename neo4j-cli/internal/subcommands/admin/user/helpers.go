// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"fmt"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"os"
)

// passwordReader is the test seam for the no-echo TTY password prompt.
var passwordReader = func() (string, error) {
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	return string(pw), nil
}

// stdinIsTTY is the test seam for terminal detection on stdin.
var stdinIsTTY = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

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
	if !stdinIsTTY() {
		return "", clierr.NewUsageError(
			"password is required: set --password or run interactively")
	}
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
	pw, err := passwordReader()
	_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("admin user: read password: %w", err)
	}
	return pw, nil
}
