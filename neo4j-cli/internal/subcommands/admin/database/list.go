// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"context"
	"encoding/json"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

var listFields = []string{"name", "type", "currentStatus", "access", "default"}

func newListCmd(cfg *clicfg.Config, credential *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all databases",
		Long: "List all databases visible from the system database. " +
			"Renders name, type, currentStatus, access, and default columns. " +
			"Uses the dbms credential named by --credential on the parent `admin` command.",
		Example: `# List all databases as a table
neo4j-cli admin database list --credential local

# List all databases as JSON for scripting
neo4j-cli admin database list --credential local --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cred, err := resolveCredential(cfg, credential)
			if err != nil {
				return err
			}
			rows, err := dbExecFn(cmd.Context(), cfg, cred, "SHOW DATABASES", nil)
			if err != nil {
				return err
			}
			commonoutput.PrintBodyMap(cmd, cfg, dbRows(rows), listFields)
			return nil
		},
	}
}

// ExecFnType is the signature shared by all database leaf commands for their
// Cypher execution seam.
type ExecFnType func(ctx context.Context, cfg *clicfg.Config, cred *credentials.DbmsCredential, cypher string, params map[string]any) ([]map[string]any, error)

// dbExecFn is the package-level test seam. It must be set to a real
// implementation by the parent command (NewCmd) before any leaf runs.
// Tests replace it to inject fake results without opening a Bolt connection.
var dbExecFn ExecFnType

// dbRows adapts a []map[string]any into commonoutput.ResponseData.
type dbRows []map[string]any

func (r dbRows) AsArray() []map[string]any {
	if r == nil {
		return []map[string]any{}
	}
	return r
}

func (r dbRows) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.AsArray())
}
