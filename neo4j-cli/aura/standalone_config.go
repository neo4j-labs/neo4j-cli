// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	common_output "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/config/project"
	"github.com/spf13/cobra"
)

// NewStandaloneConfigCmd returns a flat config command for the standalone aura-cli
// binary that covers both global keys (e.g. output) and aura-scoped keys
// (auth-url, base-url, default-tenant) in a single interface.
func NewStandaloneConfigCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage and view configuration values",
	}

	cmd.AddCommand(newStandaloneConfigGetCmd(cfg))
	cmd.AddCommand(newStandaloneConfigSetCmd(cfg))
	cmd.AddCommand(newStandaloneConfigListCmd(cfg))
	if cfg.Aura.AuraBetaEnabled() {
		cmd.AddCommand(project.NewCmd(cfg))
	}

	return cmd
}

// allStandaloneConfigKeys returns all valid config keys: global first, then aura.
func allStandaloneConfigKeys(cfg *clicfg.Config) []string {
	keys := make([]string, 0, len(cfg.Global.ValidConfigKeys)+len(cfg.Aura.ValidConfigKeys))
	keys = append(keys, cfg.Global.ValidConfigKeys...)
	keys = append(keys, cfg.Aura.ValidConfigKeys...)
	return keys
}

func newStandaloneConfigGetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:       "get <key>",
		Short:     "Displays the specified configuration value",
		ValidArgs: allStandaloneConfigKeys(cfg),
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			var value interface{}
			if cfg.Global.IsValidConfigKey(key) {
				value = cfg.Global.Get(key)
			} else {
				value = cfg.Aura.Get(key)
			}

			common_output.PrintBodyMap(cmd, cfg, common_output.ConfigData{{Key: key, Value: value}}, []string{"key", "value"})

			return nil
		},
	}
}

func newStandaloneConfigSetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Sets the specified configuration value to the provided value",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}

			key := args[0]
			if !cfg.Global.IsValidConfigKey(key) && !cfg.Aura.IsValidConfigKey(key) {
				return clierr.NewUsageError("invalid config key specified: %s", key)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]

			if cfg.Global.IsValidConfigKey(key) {
				// Validate value for "output" key
				if key == "output" {
					valid := false
					for _, v := range clicfg.ValidOutputValues {
						if v == value {
							valid = true
							break
						}
					}
					if !valid {
						return clierr.NewUsageError("invalid value for 'output': %s (valid values: %s)", value, strings.Join(clicfg.ValidOutputValues[:], ", "))
					}
				}
				cfg.Global.Set(key, value)
			} else {
				cfg.Aura.Set(key, value)
			}

			return nil
		},
	}
}

func newStandaloneConfigListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lists all current configuration values",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			data := make(common_output.ConfigData, 0, len(cfg.Global.ValidConfigKeys)+len(cfg.Aura.ValidConfigKeys))
			for _, key := range cfg.Global.ValidConfigKeys {
				data = append(data, common_output.ConfigEntry{Key: key, Value: cfg.Global.Get(key)})
			}
			for _, key := range cfg.Aura.ValidConfigKeys {
				data = append(data, common_output.ConfigEntry{Key: key, Value: cfg.Aura.Get(key)})
			}
			common_output.PrintBodyMap(cmd, cfg, data, []string{"key", "value"})
		},
	}
}
