// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package plugin

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

// JSON output bypasses this list so `--format json` consumers see every wire field.
var pluginInstallFields = []string{"name", "version", "pendingRestart", "filePath"}

// pluginInstallResult adapts a single `*DbmsPlugin` to the output.ResponseData contract.
type pluginInstallResult struct {
	Item *desktopclient.DbmsPlugin
}

func (r pluginInstallResult) AsArray() []map[string]any {
	if r.Item == nil {
		return nil
	}
	return []map[string]any{
		{
			"name":           r.Item.Name,
			"version":        r.Item.Version,
			"pendingRestart": r.Item.PendingRestart,
			"filePath":       r.Item.FilePath,
		},
	}
}

// MarshalJSON emits the full DbmsPlugin so `--format json` matches `plugin list` shape.
func (r pluginInstallResult) MarshalJSON() ([]byte, error) {
	if r.Item == nil {
		return []byte("null"), nil
	}
	return json.Marshal(r.Item)
}

func newInstallCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		pluginValue string
		noRestart   bool
	)
	const (
		pluginFlag    = "plugin"
		noRestartFlag = "no-restart"
	)

	cmd := &cobra.Command{
		Use:   "install <dbms-id>",
		Short: "Install a plugin on a local Desktop-managed DBMS",
		Long: "Install a Neo4j plugin on a DBMS managed by the local Neo4j Desktop 2 install. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"`--plugin` is the plugin name from `neo4j-cli desktop dbms plugin available <dbms-id>` (e.g. `apoc`) or an absolute path to a local plugin JAR; Desktop dispatches name-vs-path server-side. " +
			"Plugin changes take effect only after the DBMS is restarted. By default this command auto-restarts a running DBMS (Stop → Start) so the new plugin becomes active immediately; pass `--no-restart` to defer the restart explicitly. " +
			"If the DBMS is currently stopped, no restart is issued and the plugin will activate on the next start. " +
			"The plugin install operation itself may take up to 2 minutes; the auto-restart adds up to a further 60 seconds (30s Stop poll + 30s Start poll).",
		Example: `# Install a named plugin from Desktop's catalog (auto-restart if running)
neo4j-cli desktop dbms plugin install my-dbms-id --plugin apoc --rw

# Install a plugin from a local JAR path (auto-restart if running)
neo4j-cli desktop dbms plugin install my-dbms-id --plugin /tmp/custom-plugin.jar --rw

# Install a plugin without auto-restarting the DBMS (will activate on next start)
neo4j-cli desktop dbms plugin install my-dbms-id --plugin apoc --no-restart --rw`,
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

			// `GetDbms` surfaces 404 as a generic 4xx (no `ErrDbmsNotFound` — that
			// sentinel is plugin-endpoint-scoped). Tolerate failure: install routes
			// through `doPlugin` which will surface `ErrDbmsNotFound` cleanly.
			pre, preErr := client.GetDbms(ctx, dbmsID)

			installed, err := client.InstallPlugin(ctx, dbmsID, pluginValue)
			if err != nil {
				if errors.Is(err, desktopclient.ErrPluginNotFound) {
					return pluginNotFoundError(pluginValue, dbmsID)
				}
				if errors.Is(err, desktopclient.ErrDbmsNotFound) {
					return dbmsNotFoundError(dbmsID)
				}
				// Prefer the pre-op error when the install surfaces something generic.
				if preErr != nil {
					return preErr
				}
				return err
			}

			pluginName := pluginValue
			if installed != nil && installed.Name != "" {
				pluginName = installed.Name
			}

			// Render BEFORE the auto-restart so the install result is visible on
			// stdout even if the restart partially fails (plugin op is not rolled back).
			output.PrintBodyMap(cmd, cfg, pluginInstallResult{Item: installed}, pluginInstallFields)

			preStatus := ""
			preName := ""
			if pre != nil {
				preStatus = pre.Status
				preName = pre.Name
			}

			// Auto-restart decision matrix:
			//   started + !noRestart → Stop→Start (plugin live now)
			//   started +  noRestart → manual-restart hint
			//   stopped              → no restart needed (next-start hint)
			//   "" (GetDbms failed)  → status-unknown hint; NOT speculative Stop→Start
			switch {
			case preStatus == dbmsStatusStarted && !noRestart:
				if rerr := autoRestartIfRunning(ctx, client, preName, dbmsID, pluginName, "active", cmd.ErrOrStderr()); rerr != nil {
					var rer *restartErr
					if errors.As(rerr, &rer) {
						// Plugin op already succeeded — downgrade auto-restart failure to warning + exit 0.
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s. The plugin is installed; restart the DBMS manually with `neo4j-cli desktop dbms start %s --wait --rw` to activate it.\n", rer.Error(), dbmsID)
						return nil
					}
					return rerr
				}
			case preStatus == dbmsStatusStarted && noRestart:
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "DBMS %s is running but --no-restart was passed; plugin %q will activate only after you restart the DBMS (e.g. `neo4j-cli desktop dbms stop %s --rw && neo4j-cli desktop dbms start %s --rw`).\n", dbmsRef(preName, dbmsID), pluginName, dbmsID, dbmsID)
			case preStatus == dbmsStatusStopped:
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "DBMS %s is not running; plugin %q will activate on next start.\n", dbmsRef(preName, dbmsID), pluginName)
			default:
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "DBMS %q current status unknown (pre-op fetch failed); plugin %q will activate after the DBMS is restarted. Run `neo4j-cli desktop dbms list` to check.\n", dbmsID, pluginName)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&pluginValue, pluginFlag, "", "(required) Plugin name from the Desktop catalog (e.g. `apoc`) OR an absolute path to a local plugin JAR; Desktop dispatches name-vs-path server-side")
	cmd.Flags().BoolVar(&noRestart, noRestartFlag, false, "Do not auto-restart a running DBMS after the install; the plugin will activate only on the next manual start")
	return cmd
}

// pluginNotFoundError renders the canonical hint for a missing plugin.
func pluginNotFoundError(pluginName, dbmsID string) error {
	return clierr.NewFatalError(
		"Plugin %q not found on DBMS %q. Run `neo4j-cli desktop dbms plugin available %s` to see the installable catalog.",
		pluginName, dbmsID, dbmsID)
}

// dbmsNotFoundError renders the canonical hint for a missing DBMS.
func dbmsNotFoundError(dbmsID string) error {
	return clierr.NewFatalError(
		"DBMS %q not found. Run 'neo4j-cli desktop dbms list' to see the catalog of local DBMSes.",
		dbmsID)
}
