// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package dataset provides the `neo4j-cli dataset` discovery command tree. It
// is discovery-only: `dataset list` prints a curated suggestion set and the
// parent `--help` signposts the three per-target load verbs (`docker load`,
// `desktop dbms load`, `aura instance load`). There is no `load` leaf here.
package dataset

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewCmd returns the `dataset` parent command. The load verbs live on each
// target's own tree (docker, desktop dbms, aura instance), so this command only
// surfaces suggestions and signposts where to load them.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dataset",
		Short: "Discover example Neo4j datasets",
		Long: "Discover example Neo4j datasets you can load into a database.\n\n" +
			"`dataset list` prints a curated set of suggestions. Datasets are addressed by " +
			"their GitHub `<owner>/<repo>` (e.g. neo4j-graph-examples/movies), and any repo " +
			"carrying a relate.project-install.json manifest works — the suggestions are not a " +
			"constraint.\n\n" +
			"Loading happens on each target's own command tree:\n" +
			"  neo4j-cli docker load <owner/repo>          — load into a local Docker container\n" +
			"  neo4j-cli desktop dbms load <owner/repo>    — load into a Neo4j Desktop DBMS\n" +
			"  neo4j-cli aura instance load <owner/repo>   — load into a new Aura instance",
		Example: `# Load the movies dataset into a local Docker container
neo4j-cli docker load neo4j-graph-examples/movies --name movies --rw

# ... into a Neo4j Desktop DBMS
neo4j-cli desktop dbms load neo4j-graph-examples/movies --name movies --password <pw> --rw

# ... into a new Aura instance
neo4j-cli aura instance load neo4j-graph-examples/movies --name movies --type free-db --rw`,
	}

	cmd.AddCommand(newListCmd(cfg))

	return cmd
}
