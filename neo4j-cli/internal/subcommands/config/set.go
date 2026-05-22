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
				//
				// For the keyring target we always run MigrateToKeyring()
				// regardless of the current mode (REQ-F-017 repair pass): if any
				// secrets are still resident in credentials.json (e.g. from a
				// partial previous migration), they will be moved to the keyring
				// and scrubbed. This is idempotent — credentials already in the
				// keyring are read back by load() and written back unchanged.
				//
				// For the insecure target we only run MigrateToInsecure() when
				// the mode actually changes to avoid spurious keyring reads.
				credentialStorageModeChanged := false
				if bareKey == "credential-storage" {
					currentMode := cfg.Credentials.StorageMode()
					if value == credentials.StorageModeKeyring {
						// Always run the repair/migration pass for keyring target.
						if migrateErr := cfg.Credentials.MigrateToKeyring(); migrateErr != nil {
							cmd.SilenceUsage = true
							return migrateErr
						}
						credentialStorageModeChanged = (currentMode != value)
					} else if value == credentials.StorageModeInsecure && currentMode != value {
						// Only migrate to insecure when actually switching modes.
						if migrateErr := cfg.Credentials.MigrateToInsecure(); migrateErr != nil {
							cmd.SilenceUsage = true
							return migrateErr
						}
						credentialStorageModeChanged = true
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
