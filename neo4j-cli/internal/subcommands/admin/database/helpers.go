// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"os"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"golang.org/x/term"
)

// stdinIsTTY is the test seam for terminal detection on stdin.
var stdinIsTTY = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
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
