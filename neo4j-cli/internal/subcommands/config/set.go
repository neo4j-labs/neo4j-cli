// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
)

func NewSetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Sets the specified global configuration value to the provided value",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}

			if !cfg.Global.IsValidConfigKey(args[0]) {
				return clierr.NewUsageError("invalid config key specified: %s", args[0])
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]

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

			return nil
		},
	}
}
