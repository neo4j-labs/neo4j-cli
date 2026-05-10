// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package update implements the `neo4j-cli update` self-update command.
//
// The current state is a scaffold: flags are wired and visible in `--help`,
// but `RunE` returns a "not implemented" error. Subsequent tasks will fill in
// release lookup, install-method detection, atomic swap, and the RunE flow
// that ties them together.
package update

import (
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewCmd returns the `update` cobra command. It is mounted as a top-level
// subcommand on the neo4j-cli tree alongside `aura`, `credential`, `config`,
// `query`, and `skill`.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		preReleases bool
		check       bool
		version     string
		force       bool
	)

	const (
		preReleasesFlag = "pre-releases"
		checkFlag       = "check"
		versionFlag     = "version"
		forceFlag       = "force"
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Self-update the neo4j-cli binary",
		Long: "Self-update the neo4j-cli binary by downloading the latest GitHub release and atomically " +
			"swapping it in place. By default only stable semver tags are considered; pass `--pre-releases` " +
			"to opt into alpha/beta/rc tags. When the running binary lives under a known package-manager " +
			"prefix (Homebrew, npm-global, pipx, uv tool), the command refuses to overwrite and prints the " +
			"channel-correct upgrade command instead — pass `--force` to override.",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = cfg
			_ = preReleases
			_ = check
			_ = version
			_ = force
			return fmt.Errorf("not implemented")
		},
	}

	cmd.Flags().BoolVar(&preReleases, preReleasesFlag, false, "Include alpha/beta/rc tags when looking up the latest release")
	cmd.Flags().BoolVar(&check, checkFlag, false, "Report whether a newer version is available without downloading or swapping")
	cmd.Flags().StringVar(&version, versionFlag, "", "Update to the named release tag instead of the latest (must be a valid semver tag, e.g. v0.1.0)")
	cmd.Flags().BoolVar(&force, forceFlag, false, "Bypass the package-manager-managed-binary check and proceed with the in-place swap")

	return cmd
}
