// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
)

func NewSetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Sets the specified configuration value to the provided value",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}

			key := args[0]
			if !cfg.Aura.IsValidConfigKey(key) {
				return clierr.NewUsageError("invalid config key specified: %s", key)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]

			if cfg.Global.IsValidConfigKey(key) {
				return cfg.Global.Set(key, value)
			}
			if cfg.Aura.IsValidConfigKey(key) {
				cfg.Aura.Set(key, value)
				return nil
			}

			// Should never get here due to validation in Args, but adding a safeguard just in case
			return clierr.NewUsageError("invalid config key specified: %s", key)
		},
	}
}
