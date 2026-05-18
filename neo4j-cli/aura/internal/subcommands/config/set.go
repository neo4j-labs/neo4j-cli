// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	auraw "github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/workspace"
	"github.com/spf13/cobra"
)

func NewSetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:         "set <key> <value>",
		Short:       "Sets the specified configuration value to the provided value",
		Annotations: map[string]string{"write": "true"},
		Example: `# Set the default workspace used by aura commands
neo4j-cli aura config set default-workspace 00000000-0000-0000-0000-000000000000/11111111-1111-1111-1111-111111111111 --rw

# Override the Aura API base URL (for staging environments)
neo4j-cli aura config set base-url https://api.neo4j.io/v1 --rw

# Switch the output format default to JSON
neo4j-cli aura config set format json --rw`,
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
				if key == "default-workspace" {
					cmd.SilenceUsage = true
					return auraw.ValidateAndSetDefaultWorkspace(cfg, value)
				}
				cfg.Aura.Set(key, value)
				return nil
			}

			// Should never get here due to validation in Args, but adding a safeguard just in case
			return clierr.NewUsageError("invalid config key specified: %s", key)
		},
	}
}
