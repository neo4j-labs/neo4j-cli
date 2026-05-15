// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"errors"
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

// newGetCmd builds the `neo4j-cli docker get <name>` leaf. It inspects a
// single Neo4j container managed by neo4j-cli and renders the same metadata
// `list` emits plus `uri` and `image` (REQ-F-031). Containers that exist in
// Docker but lack `org.neo4j.cli.managed=true` are treated as unknown
// (REQ-F-032) so the leaf never leaks data about non-neo4j-cli containers.
//
// Unknown name → clierr.NewUsageError documented in REQ-F-032, pointing at
// `docker list` so the operator can see the actual set of managed containers.
func newGetCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show details of a Neo4j container managed by neo4j-cli",
		Long: "Show details of a single Neo4j Docker container carrying the `org.neo4j.cli.managed=true` label. " +
			"Renders name, status (Docker's human-readable state), edition, version, bolt-port, http-port, ephemeral, " +
			"uri (neo4j://localhost:<bolt-port>), and image. " +
			"Containers that exist in Docker but lack the managed label are treated as unknown; the error message " +
			"points at `neo4j-cli docker list` so the operator can see the actual set of managed containers. " +
			"Daemon-side errors (Docker not running, socket permission denied, etc.) are surfaced verbatim and are " +
			"distinct from the unknown-name error so you can tell a missing container apart from a missing daemon.",
		Example: `# Show details of a managed container by name
neo4j-cli docker get dev

# Emit JSON for scripting (e.g. piping into jq to extract the URI)
neo4j-cli docker get dev --format json

# Emit TOON for token-efficient ingestion by agents
neo4j-cli docker get dev --format toon`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			client := clientFactory()
			ctx := cmd.Context()

			container, err := client.Inspect(ctx, name)
			if err != nil {
				cmd.SilenceUsage = true
				// REQ-F-032: only the "container does not exist" branch maps
				// to the unknown-name usage error. Operational failures
				// (daemon down, permission denied, rootless misconfig, …)
				// propagate verbatim so the operator can fix the real cause
				// instead of chasing a phantom container.
				if errors.Is(err, ErrNotFound) {
					return unknownContainerError(name)
				}
				return err
			}

			// REQ-F-032: containers that exist in Docker but lack the
			// managed label are treated as unknown. parseInspectOutput is
			// responsible for populating Container only when the label is
			// present; the explicit check here guards against fake clients
			// (and any future inspector that doesn't enforce this) so the
			// contract holds end-to-end.
			if !container.Managed {
				cmd.SilenceUsage = true
				return unknownContainerError(name)
			}

			uri := fmt.Sprintf("neo4j://localhost:%s", container.BoltPort)

			row := map[string]any{
				"name":      container.Name,
				"status":    container.Status,
				"edition":   container.Edition,
				"version":   container.Version,
				"bolt-port": container.BoltPort,
				"http-port": container.HTTPPort,
				"ephemeral": container.Ephemeral,
				"uri":       uri,
				"image":     container.Image,
			}
			fields := []string{"name", "status", "edition", "version", "bolt-port", "http-port", "ephemeral", "uri", "image"}
			commonoutput.PrintBodyMap(cmd, cfg, singleRow{row: row}, fields)
			return nil
		},
	}

	flags.RegisterOutputFlag(cmd, cfg)
	return cmd
}

// unknownContainerError returns the REQ-F-032 usage error verbatim so the
// branches in `get` (and any future leaf that needs the same hint) emit
// exactly one phrasing. The %q on name keeps a stray space / quote in user
// input visually obvious.
func unknownContainerError(name string) error {
	return clierr.NewUsageError(
		"no managed container named %q (use 'neo4j-cli docker list' to see managed containers)",
		name,
	)
}
