// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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

// ExecFnType is the signature shared by all user leaf commands for their
// Cypher execution seam.
type ExecFnType func(ctx context.Context, cfg *clicfg.Config, cred *credentials.DbmsCredential, cypher string, params map[string]any) ([]map[string]any, error)

// userExecFn is the package-level test seam. It is set by NewCmd in production
// and replaced by tests to inject fake results without a real Bolt connection.
var userExecFn ExecFnType

// userRows adapts a []map[string]any into commonoutput.ResponseData.
type userRows []map[string]any

func (r userRows) AsArray() []map[string]any {
	if r == nil {
		return []map[string]any{}
	}
	return r
}

func (r userRows) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.AsArray())
}

// resolveCredential looks up the named dbms credential from cfg. If the
// credential name is empty it falls back to the default credential. Returns a
// usage error when the credential cannot be found.
func resolveCredential(cfg *clicfg.Config, name *string) (*credentials.DbmsCredential, error) {
	if name != nil && *name != "" {
		cred, err := cfg.Credentials.Dbms.Get(*name)
		if err != nil || cred == nil {
			return nil, clierr.NewUsageError("credential %q not found, run `neo4j-cli credential dbms list` to see available credentials", *name)
		}
		return cred, nil
	}
	cred, err := cfg.Credentials.Dbms.GetDefault()
	if err != nil || cred == nil {
		return nil, clierr.NewUsageError("no --credential specified and no default dbms credential is set; run `neo4j-cli credential dbms list` to see available credentials")
	}
	return cred, nil
}

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
