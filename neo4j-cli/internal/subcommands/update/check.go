// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package update

import (
	"io/fs"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// newCheckCmd returns the `update check` cobra subcommand. It reports whether
// a newer release is available without downloading or swapping the running
// binary, mirroring the `skill check` shape: read-only, exits non-zero on
// drift so CI/scripts can branch on it.
//
// The subcommand registers `--pre-releases` and `--version`. It deliberately
// does NOT register `--force` — a check has no swap path to bypass, so the
// flag would be meaningless. RunE delegates to the same runUpdate flow as the
// parent with check=true so the JSON shape (REQ-F-018) and the
// install-method passthrough hint stay identical.
func newCheckCmd(cfg *clicfg.Config, bundle fs.FS, skillName string) *cobra.Command {
	var (
		preReleases bool
		version     string
	)

	const (
		preReleasesFlag = "pre-releases"
		versionFlag     = "version"
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Report whether a newer neo4j-cli release is available without swapping",
		Long: "Compares the running binary's version against the latest GitHub release at " +
			"neo4j-labs/neo4j-cli and reports the result without downloading or swapping. " +
			"By default only stable semver tags are considered; pass `--pre-releases` to " +
			"opt into alpha/beta/rc tags. Exits non-zero when a newer version is available " +
			"so CI/scripts can branch on it.",
		// Silence the cobra Usage block on RunE error — `update check`
		// returns clierr.NewUsageError when a newer release is available
		// (REQ-F-011) to set a non-zero exit code, but that "newer
		// version is available" outcome is NOT a misuse and shouldn't
		// dump a Usage block over the structured-output stream. The
		// parent `update` command leaves SilenceUsage at its cobra
		// default (false) so genuine misuse (`update --bogus`) still
		// gets the help shown.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), cmd, cfg, runOpts{
				preReleases: preReleases,
				check:       true,
				version:     version,
				bundle:      bundle,
				skillName:   skillName,
			})
		},
	}

	cmd.Flags().BoolVar(&preReleases, preReleasesFlag, false, "Include alpha/beta/rc tags when looking up the latest release")
	cmd.Flags().StringVar(&version, versionFlag, "", "Compare against the named release tag instead of the latest (must be a valid semver tag, e.g. v0.1.0)")

	return cmd
}
