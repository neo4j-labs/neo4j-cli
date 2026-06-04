// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"context"
	"encoding/json"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
)

// ExecFnType is the signature shared by all role leaf commands for their
// Cypher execution seam.
type ExecFnType func(ctx context.Context, cfg *clicfg.Config, cred *credentials.DbmsCredential, cypher string, params map[string]any) ([]map[string]any, error)

// roleExecFn is the package-level test seam. It is set by NewCmd in production
// and replaced by tests to inject fake results without a real Bolt connection.
var roleExecFn ExecFnType

// roleRows adapts a []map[string]any into commonoutput.ResponseData.
type roleRows []map[string]any

func (r roleRows) AsArray() []map[string]any {
	if r == nil {
		return []map[string]any{}
	}
	return r
}

func (r roleRows) MarshalJSON() ([]byte, error) {
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
