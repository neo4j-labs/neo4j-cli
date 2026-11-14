// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"fmt"

	engine "github.com/neo4j/cli/internal/relate"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/spf13/cobra"
)

// newDeleteCmd builds the `neo4j-cli relate delete <name-or-path>` leaf: it
// refuses anything that is not a Relate store, gates the destructive removal
// via the shared confirm helper (both --yes and --force are required for
// non-TTY callers), then removes the whole store directory (or .rl bundle).
// Works in every build — a pure filesystem operation, no engine needed.
func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "delete <name-or-path>",
		Short:       "Delete a local Relate graph store and all its data",
		Annotations: map[string]string{"write": "true"},
		Long: "Delete a local Relate graph store — the whole store directory (or .rl bundle) and every graph " +
			"it holds; this cannot be undone. Only paths that are Relate stores can be deleted; anything else " +
			"is refused. Destructive: requires `--yes --force` (or a `y` answer at the TTY prompt) when " +
			"invoked non-interactively. A store currently open by another process (e.g. a running " +
			"`relate serve`) keeps its lock files; removal still proceeds on platforms that allow it.",
		Example: `# Delete a managed store; prompts on a TTY
neo4j-cli relate delete movies

# Skip the prompt (required for scripts / non-TTY callers)
neo4j-cli relate delete movies --yes --force

# Delete a store at an explicit path
neo4j-cli relate delete ./mydb --yes --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			path := engine.ResolveStorePath(args[0])
			if path == engine.StoreDir() {
				return clierr.NewUsageError("refusing to delete the managed store directory itself; delete individual stores")
			}
			if !engine.IsStore(cfg.Fs(), path) {
				return unknownStoreError(path)
			}

			if err := confirm.RequireTyped(cmd, "Relate store", path); err != nil {
				return err
			}

			if err := engine.RemoveStore(cfg.Fs(), path); err != nil {
				return clierr.NewFatalError("cannot delete the Relate store at %s: %w", path, err)
			}

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Deleted Relate store at %s\n", path)
			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
