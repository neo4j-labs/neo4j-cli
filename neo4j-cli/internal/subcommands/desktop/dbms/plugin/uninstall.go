// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package plugin

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

// Idempotent on the relate side: uninstalling an already-removed plugin returns 200
// with `{name}`, surfaced as a normal success render.
func newUninstallCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		pluginValue string
		noRestart   bool
	)
	const (
		pluginFlag    = "plugin"
		noRestartFlag = "no-restart"
	)

	cmd := &cobra.Command{
		Use:   "uninstall <dbms-id>",
		Short: "Uninstall a plugin from a local Desktop-managed DBMS",
		Long: "Uninstall a Neo4j plugin from a DBMS managed by the local Neo4j Desktop 2 install. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"`--plugin` is the plugin name from `neo4j-cli desktop dbms plugin list <dbms-id>` (e.g. `apoc`); relate matches it against the DBMS's installed plugin set. " +
			"Plugin changes take effect only after the DBMS is restarted. By default this command auto-restarts a running DBMS (Stop → Start) so the JVM drops the plugin JAR immediately; pass `--no-restart` to defer the restart explicitly. " +
			"If the DBMS is currently stopped, no restart is issued and the removal will be picked up on the next start. " +
			"The uninstall is idempotent — removing an already-uninstalled plugin still exits 0 with the same confirmation shape. " +
			"The plugin uninstall operation itself may take up to 2 minutes; the auto-restart adds up to a further 60 seconds (30s Stop poll + 30s Start poll).",
		Example: `# Uninstall a plugin (auto-restart if the DBMS is running)
neo4j-cli desktop dbms plugin uninstall my-dbms-id --plugin apoc --rw

# Uninstall a plugin without auto-restarting the DBMS (will deactivate on next restart)
neo4j-cli desktop dbms plugin uninstall my-dbms-id --plugin apoc --no-restart --rw

# Uninstall a plugin and emit a machine-readable confirmation for scripting
neo4j-cli desktop dbms plugin uninstall my-dbms-id --plugin apoc --format json --rw`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			fs := cfg.Aura.Fs()
			port, _ := cmd.Flags().GetInt(portFlag)
			dbmsID := args[0]
			if pluginValue == "" {
				return clierr.NewUsageError("--%s is required", pluginFlag)
			}

			client, err := newDesktopClientFn(ctx, fs, port)
			if err != nil {
				return err
			}

			// Tolerated on failure — the uninstall POST routes through `doPlugin`
			// which surfaces `ErrDbmsNotFound` cleanly if the DBMS is missing.
			pre, preErr := client.GetDbms(ctx, dbmsID)

			uninstalledName, err := client.UninstallPlugin(ctx, dbmsID, pluginValue)
			if err != nil {
				if errors.Is(err, desktopclient.ErrPluginNotFound) {
					return pluginNotFoundError(pluginValue, dbmsID)
				}
				if errors.Is(err, desktopclient.ErrDbmsNotFound) {
					return dbmsNotFoundError(dbmsID)
				}
				// Prefer the pre-op error when the uninstall surfaces something generic.
				if preErr != nil {
					return preErr
				}
				return err
			}

			// Fall back to the user-supplied value if relate didn't echo a name (defensive).
			displayName := uninstalledName
			if displayName == "" {
				displayName = pluginValue
			}

			// Render BEFORE the auto-restart so the success confirmation is visible
			// on stdout even if the restart partially fails (uninstall not rolled back).
			renderUninstallResult(cmd, cfg, displayName)

			preStatus := ""
			preName := ""
			if pre != nil {
				preStatus = pre.Status
				preName = pre.Name
			}

			// Auto-restart decision matrix (identical to install):
			//   started + !noRestart → Stop→Start (plugin gone now)
			//   started +  noRestart → manual-restart hint
			//   stopped              → no restart needed (next-start hint)
			//   "" (GetDbms failed)  → status-unknown hint
			switch {
			case preStatus == dbmsStatusStarted && !noRestart:
				if rerr := autoRestartIfRunning(ctx, client, preName, dbmsID, displayName, "removed", cmd.ErrOrStderr()); rerr != nil {
					var rer *restartErr
					if errors.As(rerr, &rer) {
						// Uninstall op already succeeded — downgrade auto-restart failure to warning + exit 0.
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s. The plugin is uninstalled; restart the DBMS manually with `neo4j-cli desktop dbms start %s --wait --rw` to drop it from the running JVM.\n", rer.Error(), dbmsID)
						return nil
					}
					return rerr
				}
			case preStatus == dbmsStatusStarted && noRestart:
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "DBMS %s is running but --no-restart was passed; plugin %q is removed from neo4j.conf but the running JVM will keep it loaded until the DBMS is restarted (e.g. `neo4j-cli desktop dbms stop %s --rw && neo4j-cli desktop dbms start %s --rw`).\n", dbmsRef(preName, dbmsID), displayName, dbmsID, dbmsID)
			case preStatus == dbmsStatusStopped:
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "DBMS %s is not running; plugin %q removal will be picked up on next start.\n", dbmsRef(preName, dbmsID), displayName)
			default:
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "DBMS %q current status unknown (pre-op fetch failed); plugin %q removal will be picked up after the DBMS is restarted. Run `neo4j-cli desktop dbms list` to check.\n", dbmsID, displayName)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&pluginValue, pluginFlag, "", "(required) Plugin name to uninstall (must match an entry from `neo4j-cli desktop dbms plugin list <dbms-id>`)")
	cmd.Flags().BoolVar(&noRestart, noRestartFlag, false, "Do not auto-restart a running DBMS after the uninstall; the running JVM will keep the plugin loaded until the next manual restart")
	return cmd
}

// renderUninstallResult emits the `{name, uninstalled: true}` confirmation across all formats.
func renderUninstallResult(cmd *cobra.Command, cfg *clicfg.Config, pluginName string) {
	switch commonoutput.ResolveOutput(cmd, cfg) {
	case "json":
		payload := struct {
			Name        string `json:"name"`
			Uninstalled bool   `json:"uninstalled"`
		}{Name: pluginName, Uninstalled: true}
		buf, err := json.MarshalIndent(payload, "", "\t")
		if err != nil {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "{\"name\":%q,\"uninstalled\":true}\n", pluginName)
			return
		}
		cmd.Println(string(buf))
	default:
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Uninstalled plugin %q.\n", pluginName)
	}
}
