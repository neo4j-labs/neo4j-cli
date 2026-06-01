// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

// pluginUpgradeModeWire maps the lowercase user-facing values to the uppercase
// wire enum Desktop expects. The keys double as the validation allow-list.
var pluginUpgradeModeWire = map[string]string{
	"all":        "ALL",
	"none":       "NONE",
	"upgradable": "UPGRADABLE",
}

func newUpgradeCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		version           string
		pluginUpgradeMode string
		noMigrate         bool
		backup            bool
		force             bool
	)
	const (
		versionFlag           = "version"
		pluginUpgradeModeFlag = "plugin-upgrade-mode"
		noMigrateFlag         = "no-migrate"
		backupFlag            = "backup"
		forceFlag             = "force"
	)

	cmd := &cobra.Command{
		Use:   "upgrade <id>",
		Short: "Upgrade a DBMS managed by the local Neo4j Desktop 2 install",
		Long: "Upgrade a DBMS managed by the local Neo4j Desktop 2 install to a newer Neo4j version. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"`--version` is optional: when omitted, the CLI queries Desktop's `GET /dbmss/versions` catalog and auto-picks the highest stable enterprise version (preferring already-cached entries on ties), emitting a stderr breadcrumb naming the picked version + origin. " +
			"Desktop upgrades a DBMS only while it is stopped: the command refuses when the target is running unless `--force` is passed, in which case it stops the DBMS (polling until stopped) and then upgrades. " +
			"`--plugin-upgrade-mode` controls how installed plugins are migrated (`all`, `none`, or `upgradable`); `--no-migrate` skips the store-format migration; `--backup` (default true) takes a backup before upgrading. " +
			"The upgrade can take several minutes; the command blocks until Desktop reports it complete and leaves the DBMS stopped — start it again with `neo4j-cli desktop dbms start <id> --rw`.",
		Example: `# Upgrade a DBMS to the latest stable enterprise version Desktop knows about
neo4j-cli desktop dbms upgrade my-dbms-id --rw

# Upgrade a DBMS to a specific version
neo4j-cli desktop dbms upgrade my-dbms-id --version 5.26.1 --rw

# Stop the DBMS first if it is running, then upgrade it
neo4j-cli desktop dbms upgrade my-dbms-id --version 5.26.1 --force --rw

# Upgrade without a pre-upgrade backup and skip plugin migration
neo4j-cli desktop dbms upgrade my-dbms-id --backup=false --plugin-upgrade-mode none --rw

# Upgrade a DBMS and emit the upgraded DbmsInfo as JSON for scripting
neo4j-cli desktop dbms upgrade my-dbms-id --version 5.26.1 --format json --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate plugin-upgrade-mode up front so a bad value fails before
			// any Desktop round-trip.
			wireMode, ok := pluginUpgradeModeWire[strings.ToLower(pluginUpgradeMode)]
			if !ok {
				return clierr.NewUsageError(
					"invalid --%s %q: valid values are all, none, upgradable", pluginUpgradeModeFlag, pluginUpgradeMode)
			}
			cmd.SilenceUsage = true

			ctx := cmd.Context()
			fs := cfg.Aura.Fs()
			port, _ := cmd.Flags().GetInt(portFlag)
			id := args[0]

			client, err := newDesktopClientFn(ctx, fs, port)
			if err != nil {
				return err
			}
			if version == "" {
				// Pick the highest stable enterprise entry from Desktop's catalog
				// and emit a stderr breadcrumb so the user knows what was chosen.
				versions, verr := client.ListDbmsVersions(ctx)
				if verr != nil {
					return verr
				}
				picked, perr := pickLatestStableEnterprise(versions)
				if perr != nil {
					return perr
				}
				version = picked.Version
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Using Neo4j enterprise %s (%s)\n", picked.Version, picked.Origin)
			}

			info, err := client.GetDbms(ctx, id)
			if err != nil {
				return err
			}
			if info.Status == dbmsStatusStarted {
				if !force {
					return clierr.NewFatalError(
						"Cannot upgrade DBMS %s while it is running. "+
							"Desktop upgrades a stopped DBMS only. "+
							"Stop it first with 'neo4j-cli desktop dbms stop %s --rw', "+
							"or pass --force to stop it automatically.",
						formatDbmsRef(info.Name, info.ID), id)
				}
				// --force: stop the running target and wait until it reports
				// stopped before the upgrade can proceed.
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Stopping DBMS %s before upgrade...\n", formatDbmsRef(info.Name, info.ID))
				if err := client.StopDbms(ctx, id); err != nil {
					return err
				}
				if _, perr := pollUntilStatus(ctx, client, id, dbmsStatusStopped); perr != nil {
					return perr
				}
			}

			opts := desktopclient.UpgradeDbmsOptions{
				Migrate:           ptr(!noMigrate),
				Backup:            ptr(backup),
				PluginUpgradeMode: wireMode,
			}
			upgraded, err := client.UpgradeDbms(ctx, id, version, opts)
			if err != nil {
				return err
			}

			output.PrintBodyMap(cmd, cfg, dbmsCreateResult{Item: upgraded}, dbmsCreateFields)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
				"DBMS is stopped; start it with 'neo4j-cli desktop dbms start %s --rw'\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&version, versionFlag, "", "Neo4j version to upgrade to (e.g. 2026.04.0 or 5.26.1). When omitted, picks the latest stable enterprise version Desktop knows about.")
	cmd.Flags().StringVar(&pluginUpgradeMode, pluginUpgradeModeFlag, "upgradable", "How to migrate installed plugins during the upgrade: all, none, or upgradable.")
	cmd.Flags().BoolVar(&noMigrate, noMigrateFlag, false, "Skip the store-format migration step during the upgrade.")
	cmd.Flags().BoolVar(&backup, backupFlag, true, "Take a backup of the DBMS before upgrading.")
	cmd.Flags().BoolVar(&force, forceFlag, false, "Stop the DBMS first if it is running, then upgrade. Without --force, the command refuses when the DBMS is running.")

	return cmd
}

// ptr returns a pointer to v; used to set the optional *bool upgrade options.
func ptr[T any](v T) *T { return &v }
