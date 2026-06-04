// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package adminutil provides shared types and helpers used by all admin
// sub-packages (database, user, role).
package adminutil

import (
	"context"
	"encoding/json"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
)

// ExecFn is the Cypher execution function signature shared by all admin
// sub-packages. It is set by each package's NewCmd from the injected
// admin.RunAdminStatement and replaced by tests to avoid real Bolt connections.
type ExecFn func(ctx context.Context, cfg *clicfg.Config, cred *credentials.DbmsCredential, cypher string, params map[string]any) ([]map[string]any, error)

// Rows adapts a []map[string]any into commonoutput.ResponseData.
type Rows []map[string]any

func (r Rows) AsArray() []map[string]any {
	if r == nil {
		return []map[string]any{}
	}
	return r
}

func (r Rows) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.AsArray())
}

// ResolveCredential looks up the named dbms credential from cfg. If name is
// empty it falls back to the default credential.
func ResolveCredential(cfg *clicfg.Config, name *string) (*credentials.DbmsCredential, error) {
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
