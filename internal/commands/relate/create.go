// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"encoding/json"
	"path/filepath"

	engine "github.com/neo4j/cli/internal/relate"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

// newCreateCmd builds the `neo4j-cli relate create <name-or-path>` leaf: it
// opens (and thereby creates) an empty store, checkpoints it so the store
// directory is fully materialized, and closes it again. An existing store is
// refused unless --force (which keeps it untouched and reports it).
func newCreateCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "create <name-or-path>",
		Short:       "Create a local Relate graph store",
		Annotations: map[string]string{"write": "true"},
		Long: "Create an empty local Relate graph store. A bare name is created under the CLI's managed " +
			"store directory (see `relate list`); a path is used verbatim. The store is created empty — " +
			"populate and query it with `neo4j-cli query --relate <name-or-path>` (writes need --rw). " +
			"An existing store is refused unless --force, which keeps the existing data untouched. " +
			"A path that exists but is neither a store nor an empty directory is always refused. " +
			"Requires a build with the Relate engine (-tags relate).",
		Example: `# Create a store under the managed directory
neo4j-cli relate create movies

# Create a store at an explicit path
neo4j-cli relate create ./mydb

# Accept an existing store instead of refusing
neo4j-cli relate create movies --force

# Then populate it
neo4j-cli query "CREATE (:Movie {title: 'The Matrix'})" --relate movies --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			if engine.OpenStore == nil {
				return engine.NotCompiledError("`relate create`")
			}

			path := engine.ResolveStorePath(args[0])
			force, _ := cmd.Flags().GetBool("force")

			if engine.IsStore(cfg.Fs(), path) {
				if !force {
					return clierr.NewUsageError(
						"a Relate store already exists at %s; pass --force to keep it, or choose another name",
						path)
				}
				// --force: keep the existing store untouched, report it.
				commonoutput.PrintBodyMap(cmd, cfg, singleRow{row: storeRow(args[0], path)}, storeFields)
				return nil
			}
			if engine.Exists(cfg.Fs(), path) && !engine.IsEmptyDir(cfg.Fs(), path) {
				return clierr.NewUsageError(
					"path %s exists and is neither a Relate store nor an empty directory", path)
			}

			store, err := engine.OpenStore(path, "")
			if err != nil {
				return clierr.NewFatalError("cannot create the Relate store at %s: %w", path, err)
			}
			if err := store.Checkpoint(); err != nil {
				store.Close()
				return clierr.NewFatalError("cannot checkpoint the new store at %s: %w", path, err)
			}
			store.Close()

			commonoutput.PrintBodyMap(cmd, cfg, singleRow{row: storeRow(args[0], path)}, storeFields)
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Keep an existing store instead of refusing (its data is untouched)")

	return cmd
}

// storeFields is the field order every relate leaf renders stores in.
var storeFields = []string{"name", "path"}

// storeRow shapes one store's identity row. name echoes the argument as
// given (the name the user can address the store by); path is the resolved
// filesystem path.
func storeRow(arg, path string) map[string]any {
	return map[string]any{
		"name": filepath.Base(path),
		"path": path,
	}
}

// singleRow adapts one map into a single-entity response: AsArray feeds the
// table renderer a one-element table, and MarshalJSON makes the JSON form
// the row object itself (not the wrapper struct, whose field is unexported
// and would marshal as {}).
type singleRow struct {
	row map[string]any
}

// AsArray implements commonoutput.ResponseData.
func (r singleRow) AsArray() []map[string]any {
	return []map[string]any{r.row}
}

// MarshalJSON returns the row's JSON object form.
func (r singleRow) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.row)
}
