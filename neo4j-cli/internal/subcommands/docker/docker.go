// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package docker provides the `neo4j-cli docker` command tree for managing
// local Neo4j containers via the host `docker` CLI. Discovery and state are
// driven by Docker itself (containers carry `org.neo4j.cli.managed=true`
// and a small set of metadata labels) — no separate state file under cli/.
package docker

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewCmd returns the `docker` parent command. Leaves (create, list, get,
// start, stop, delete) are added in subsequent tasks.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	// cfg is threaded through so leaf constructors (added in subsequent
	// tasks) can reach credentials / Fs / output flags via the closure.
	_ = cfg

	cmd := &cobra.Command{
		Use:   "docker",
		Short: "Manage local Neo4j containers via Docker",
		Long: "Manage local Neo4j Docker containers (create, list, get, start, stop, delete). " +
			"Shells out to the host `docker` CLI and discovers managed containers via the " +
			"`org.neo4j.cli.managed=true` label — Docker itself is the source of truth, " +
			"no separate state file is maintained. Use `--ephemeral` on `create` for a " +
			"throwaway container plus an env-file consumable by `query --env <path>`.",
	}

	// Leaves are registered in later tasks via cmd.AddCommand(newXxxCmd(cfg)).
	// Reference newClient here so the default exec-backed client survives
	// dead-code linting until the first leaf wires it.
	_ = newClient

	return cmd
}
