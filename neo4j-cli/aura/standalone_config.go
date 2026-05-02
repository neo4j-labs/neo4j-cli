// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
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

			switch cfg.Global.Output() {
			case "json":
				standaloneConfigPrintKeyValueAsJSON(cmd, key, value)
			case "table":
				standaloneConfigPrintKeyValueAsTable(cmd, key, value)
			default:
				cmd.Println(value)
			}

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
			if cfg.Global.Output() == "table" {
				standaloneConfigPrintAllTable(cmd, cfg)
			} else {
				standaloneConfigPrintAllJSON(cmd, cfg)
			}
		},
	}
}

func standaloneConfigPrintAllJSON(cmd *cobra.Command, cfg *clicfg.Config) {
	m := make(map[string]interface{})
	for _, key := range cfg.Global.ValidConfigKeys {
		m[key] = cfg.Global.Get(key)
	}
	for _, key := range cfg.Aura.ValidConfigKeys {
		m[key] = cfg.Aura.Get(key)
	}

	bytes, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		panic(err)
	}
	cmd.Println(string(bytes))
}

func standaloneConfigPrintAllTable(cmd *cobra.Command, cfg *clicfg.Config) {
	t := table.NewWriter()
	t.AppendHeader(table.Row{"key", "value"})

	for _, key := range cfg.Global.ValidConfigKeys {
		value := cfg.Global.Get(key)
		var displayValue string
		if value == nil {
			displayValue = ""
		} else {
			displayValue = fmt.Sprintf("%v", value)
		}
		t.AppendRow(table.Row{key, displayValue})
	}

	for _, key := range cfg.Aura.ValidConfigKeys {
		value := cfg.Aura.Get(key)
		var displayValue string
		if value == nil {
			displayValue = ""
		} else {
			displayValue = fmt.Sprintf("%v", value)
		}
		t.AppendRow(table.Row{key, displayValue})
	}

	t.SetStyle(table.StyleLight)
	cmd.Println(t.Render())
}

func standaloneConfigPrintKeyValueAsJSON(cmd *cobra.Command, key string, value interface{}) {
	m := map[string]interface{}{key: value}
	bytes, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		panic(err)
	}
	cmd.Println(string(bytes))
}

func standaloneConfigPrintKeyValueAsTable(cmd *cobra.Command, key string, value interface{}) {
	t := table.NewWriter()
	t.AppendHeader(table.Row{"key", "value"})
	var displayValue string
	if value == nil {
		displayValue = ""
	} else {
		displayValue = fmt.Sprintf("%v", value)
	}
	t.AppendRow(table.Row{key, displayValue})
	t.SetStyle(table.StyleLight)
	cmd.Println(t.Render())
}
