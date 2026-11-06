// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package relate provides the `neo4j-cli relate` command tree for managing
// local embedded Relate graph stores (create, list, info, delete). Querying
// a store is the sibling `query --relate` flag, not a leaf here.
//
// Store arguments are names or paths: a bare name resolves under the CLI's
// managed store directory (see `relate list`); anything path-shaped is taken
// verbatim. The engine itself is compiled in behind the `relate` build tag —
// engine-touching leaves (create, info) fail closed with a typed
// not-compiled usage error in a build without it, while list and delete
// (pure filesystem operations) work in every build.

package relate

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

// NewCmd returns the `relate` parent command with all leaves mounted.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relate",
		Short: "Manage local embedded Relate graph stores",
		Long: "Manage local Relate graph stores — the embedded openCypher engine (formerly CypherLite). " +
			"A store is a directory (or .rl bundle) holding one graph database; no server, ports or credentials are involved. " +
			"Stores are created under the CLI's managed directory when you pass a bare name, or at any path you give. " +
			"Query a store with `neo4j-cli query \"...\" --relate <name-or-path>` (writes need --rw). " +
			"`create` and `info` need the engine compiled in (build with -tags relate); `list` and `delete` work in every build.",
		Example: `# Create a store under the managed directory, then query it
neo4j-cli relate create movies
neo4j-cli query "CREATE (m:Movie {title: 'The Matrix'}) RETURN m" --relate movies --rw

# Create a store at an explicit path
neo4j-cli relate create ./mydb

# List managed stores; inspect one (engine, counts, labels)
neo4j-cli relate list
neo4j-cli relate info movies

# Delete a store (destructive: --yes --force required when scripted)
neo4j-cli relate delete movies --yes --force`,
	}

	cmd.AddCommand(newCreateCmd(cfg))
	cmd.AddCommand(newDeleteCmd(cfg))
	cmd.AddCommand(newInfoCmd(cfg))
	cmd.AddCommand(newListCmd(cfg))

	return cmd
}
