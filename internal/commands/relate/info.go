// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"path/filepath"

	engine "github.com/neo4j/cli/internal/relate"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

// newInfoCmd builds the `neo4j-cli relate info <name-or-path>` leaf: it
// opens the store read-only and reports its identity and contents — engine
// name and version, node and relationship counts, labels, relationship
// types, index and constraint counts, plus the store's size and mtime from
// the filesystem. Read-only: a missing store is a usage error, nothing is
// ever created. Requires a build with the Relate engine (-tags relate).
func newInfoCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <name-or-path>",
		Short: "Show a Relate graph store's engine, counts and labels",
		Long: "Inspect a local Relate graph store: engine name and version (CALL dbms.components), " +
			"node and relationship counts, labels, relationship types, index and constraint counts, " +
			"and the store's on-disk size and last modification. The store is opened read-only — " +
			"a missing store is an error, never created. Works with .rl bundles and directories alike. " +
			"Requires a build with the Relate engine (-tags relate).",
		Example: `# Inspect a managed store
neo4j-cli relate info movies

# Inspect a store at an explicit path, JSON for scripting
neo4j-cli relate info ./mydb --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			if engine.OpenStoreReadOnly == nil {
				return engine.NotCompiledError("`relate info`")
			}

			path := engine.ResolveStorePath(args[0])
			if !engine.IsStore(cfg.Fs(), path) {
				return unknownStoreError(path)
			}

			store, err := engine.OpenStoreReadOnly(path, "")
			if err != nil {
				return clierr.NewFatalError("cannot open the Relate store at %s read-only: %w", path, err)
			}
			defer store.Close()

			row, err := inspectStore(cmd, cfg, store, path)
			if err != nil {
				return err
			}
			commonoutput.PrintBodyMap(cmd, cfg, singleRow{row: row}, infoFields)
			return nil
		},
	}

	return cmd
}

// infoFields is the render order of the info row.
var infoFields = []string{
	"name", "path", "engine", "version",
	"nodes", "relationships", "labels", "relationship_types",
	"indexes", "constraints", "size_bytes", "modified",
}

// inspectStore gathers the store's metadata through read-only queries and
// the filesystem, returning the rendered row.
func inspectStore(cmd *cobra.Command, cfg *clicfg.Config, store engine.Store, path string) (map[string]any, error) {
	// Engine identity: dbms.components reports name and versions.
	engineName, engineVersion := "", ""
	if res, err := store.Run([]string{"CALL dbms.components()"}, nil, false); err == nil && len(res) == 1 && len(res[0].Rows) > 0 {
		engineName, engineVersion = componentFields(res[0])
	}

	row := map[string]any{
		"name":    filepath.Base(path),
		"path":    path,
		"engine":  engineName,
		"version": engineVersion,
	}

	// Each probe is optional: a stripped engine must not fail the whole
	// info, so failures leave the field at its zero value.
	for _, probe := range []struct {
		stmt  string
		apply func(res engine.StatementResult)
	}{
		{"MATCH (n) RETURN count(n) AS nodes", func(res engine.StatementResult) {
			if v, ok := firstCell(res, "nodes"); ok {
				row["nodes"] = v
			}
		}},
		{"MATCH ()-[r]->() RETURN count(r) AS relationships", func(res engine.StatementResult) {
			if v, ok := firstCell(res, "relationships"); ok {
				row["relationships"] = v
			}
		}},
		{"CALL db.labels() YIELD label WITH label ORDER BY label RETURN collect(label) AS labels", func(res engine.StatementResult) {
			if v, ok := firstCell(res, "labels"); ok {
				row["labels"] = v
			}
		}},
		{"CALL db.relationshipTypes() YIELD relationshipType WITH relationshipType ORDER BY relationshipType RETURN collect(relationshipType) AS relationship_types", func(res engine.StatementResult) {
			if v, ok := firstCell(res, "relationship_types"); ok {
				row["relationship_types"] = v
			}
		}},
		{"SHOW INDEXES", func(res engine.StatementResult) {
			row["indexes"] = len(res.Rows)
		}},
		{"SHOW CONSTRAINTS", func(res engine.StatementResult) {
			row["constraints"] = len(res.Rows)
		}},
	} {
		if res, err := store.Run([]string{probe.stmt}, nil, false); err == nil && len(res) == 1 {
			probe.apply(res[0])
		}
	}

	size, modified, err := engine.TreeSize(cfg.Fs(), path)
	if err == nil {
		row["size_bytes"] = size
		row["modified"] = modified.Format("2006-01-02T15:04:05Z07:00")
	}

	return row, nil
}

// componentFields extracts the engine name and version from a
// dbms.components result, locating the columns by name defensively.
func componentFields(res engine.StatementResult) (name, version string) {
	nameIdx, versionIdx := columnIndexOf(res, "name"), columnIndexOf(res, "versions")
	if nameIdx >= 0 && len(res.Rows[0]) > nameIdx {
		if s, ok := res.Rows[0][nameIdx].(string); ok {
			name = s
		}
	}
	if versionIdx >= 0 && len(res.Rows[0]) > versionIdx {
		switch v := res.Rows[0][versionIdx].(type) {
		case string:
			version = v
		case []any:
			if len(v) > 0 {
				if s, ok := v[0].(string); ok {
					version = s
				}
			}
		}
	}
	return name, version
}

// firstCell returns the named column's value from the result's first row.
func firstCell(res engine.StatementResult, column string) (any, bool) {
	idx := columnIndexOf(res, column)
	if idx < 0 || len(res.Rows) == 0 || len(res.Rows[0]) <= idx {
		return nil, false
	}
	return res.Rows[0][idx], true
}

// columnIndexOf locates a column by name; -1 when absent.
func columnIndexOf(res engine.StatementResult, column string) int {
	for i, c := range res.Columns {
		if c == column {
			return i
		}
	}
	return -1
}

// unknownStoreError is the shared "no store here" usage error naming the
// resolved path and pointing at create.
func unknownStoreError(path string) error {
	return clierr.NewUsageError(
		"no Relate store at %s; create one with `neo4j-cli relate create` or check the name with `neo4j-cli relate list`", path)
}
