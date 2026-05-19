// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/spf13/cobra"
)

func NewSetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Sets the specified configuration value to the provided value",
		Example: `# Set the default output format to JSON
neo4j-cli config set format json --rw

# Disable telemetry
neo4j-cli config set telemetry false --rw

# Set the default Aura workspace via dot-notation
neo4j-cli config set aura.default-workspace my-org-id/my-project-id --rw`,
		Annotations: map[string]string{"write": "true"},
		ValidArgs:   validSetArgs(cfg),
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
				cmd.SilenceUsage = true
				return err
			}

			switch scope {
			case clicfg.AuraScope:
				cfg.Aura.Set(bareKey, value)
				return nil
			case clicfg.FlagScope:
				if err := cfg.Flags.SetFromConfigCmd(bareKey, value); err != nil {
					cmd.SilenceUsage = true
					return err
				}
				return nil
			default:
				// When changing credential-storage, migrate secrets before
				// persisting the new config value. Migration must succeed before
				// the config key is written; on failure the config is unchanged.
				credentialStorageModeChanged := false
				if bareKey == "credential-storage" {
					currentMode := cfg.Credentials.StorageMode()
					if currentMode != value {
						credentialStorageModeChanged = true
						var migrateErr error
						switch value {
						case credentials.StorageModeKeyring:
							migrateErr = cfg.Credentials.MigrateToKeyring()
						case credentials.StorageModeInsecure:
							migrateErr = cfg.Credentials.MigrateToInsecure()
						}
						if migrateErr != nil {
							cmd.SilenceUsage = true
							return migrateErr
						}
					}
				}

				if err := cfg.Global.Set(bareKey, value); err != nil {
					cmd.SilenceUsage = true
					return err
				}

				// Update in-memory storage mode after successfully persisting the
				// config key so subsequent operations in this process use the new mode.
				// Only update when the mode actually changed to avoid a redundant
				// keyring reload.
				if credentialStorageModeChanged {
					if err := cfg.Credentials.SetStorageMode(value); err != nil {
						cmd.SilenceUsage = true
						return err
					}
				}

				return nil
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
