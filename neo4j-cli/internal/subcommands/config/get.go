// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

// validGetArgs returns the list of valid tab-completion arguments for the get command.
// It includes all global keys plus "aura.<key>" for each aura key, excluding any
// aura key that would shadow a global key (e.g. "aura.output" is excluded because
// "output" already exists as a global key).
func validGetArgs(cfg *clicfg.Config) []string {
	args := make([]string, 0, len(cfg.Global.ValidConfigKeys)+len(cfg.Aura.ValidConfigKeys))
	args = append(args, cfg.Global.ValidConfigKeys...)
	for _, k := range cfg.Aura.ValidConfigKeys {
		if !cfg.Global.IsValidConfigKey(k) {
			args = append(args, fmt.Sprintf("aura.%s", k))
		}
	}
	return args
}

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Displays the specified configuration value",
		Example: `# Get the active output format
neo4j-cli config get format

# Get the active output format as JSON
neo4j-cli config get format --format json

# Get an aura-scoped key via dot-notation
neo4j-cli config get aura.default-workspace --format json`,
		ValidArgs: validGetArgs(cfg),
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}

			// Validate the key via the resolver — rejects unrecognised or shadowed keys.
			_, _, err := clicfg.ResolveConfigKey(args[0], cfg)
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			scope, bareKey, err := clicfg.ResolveConfigKey(key, cfg)
			if err != nil {
				cmd.SilenceUsage = true
				return err
			}

			switch scope {
			case clicfg.AuraScope:
				entry := cfg.Aura.GetPrintable(bareKey)
				// Display with the "aura." prefix so the user sees the full dot-notation key
				entry.Key = fmt.Sprintf("aura.%s", bareKey)
				output.PrintBodyMap(cmd, cfg, entry, configPrintFields)
			case clicfg.FlagScope:
				entry := clicfg.PrintableConfigEntry{Key: bareKey, Value: cfg.Flags.Enabled(bareKey)}
				output.PrintBodyMap(cmd, cfg, entry, configPrintFields)
			default:
				output.PrintBodyMap(cmd, cfg, cfg.Global.GetPrintable(bareKey), configPrintFields)
			}
			return nil
		},
	}
}
