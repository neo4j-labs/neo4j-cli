// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewSetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:       "set <key> <value>",
		Short:     "Sets the specified configuration value to the provided value",
		ValidArgs: validSetArgs(cfg),
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}

			// Validate the key via the resolver — rejects unrecognised or shadowed keys.
			_, _, err := clicfg.ResolveConfigKey(args[0], cfg)
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]

			scope, bareKey, err := clicfg.ResolveConfigKey(key, cfg)
			if err != nil {
				return err
			}

			switch scope {
			case clicfg.AuraScope:
				cfg.Aura.Set(bareKey, value)
				return nil
			default:
				return cfg.Global.Set(bareKey, value)
			}
		},
	}
}

// validSetArgs returns the list of valid tab-completion arguments for the set command.
// It reuses the same logic as validGetArgs: global keys plus "aura.<key>" for each
// aura key that is not already a global key.
func validSetArgs(cfg *clicfg.Config) []string {
	return validGetArgs(cfg)
}
