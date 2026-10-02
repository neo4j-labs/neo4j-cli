// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import (
	"slices"
	"strconv"
	"strings"

	"github.com/neo4j/cli/internal/clicfg/credentials"
	"github.com/neo4j/cli/internal/clicfg/fileutils"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/spf13/afero"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tidwall/sjson"
)

// GlobalConfig holds configuration that applies globally across all sub-CLIs,
// operating on top-level (non-namespaced) viper keys.
type GlobalConfig struct {
	viper           *viper.Viper
	fs              afero.Fs
	configPath      string
	ValidConfigKeys []string
}

func (config *GlobalConfig) IsValidConfigKey(key string) bool {
	return slices.Contains(config.ValidConfigKeys, key)
}

// booleanGlobalKeys are the global config keys whose stored value is logically
// boolean. config set persists every value as a string and the env bootstrap
// (e.g. NEO4J_CLI_ACCEPT_ENV_VARS=1) surfaces the raw "1"/"0"/"true" literal, so
// Get coerces these via GetBool to render unquoted true/false consistently
// across config get and config list.
var booleanGlobalKeys = map[string]bool{
	"telemetry":          true,
	"skill-auto-refresh": true,
	"history-enabled":    true,
	"tee-enabled":        true,
	"accept-env-vars":    true,
}

func (config *GlobalConfig) Get(key string) interface{} {
	// Preserve a nil (null) result for keys with no value set so unset
	// boolean keys still render as null rather than a defaulted false.
	if booleanGlobalKeys[key] && config.viper.Get(key) != nil {
		return config.viper.GetBool(key)
	}
	return config.viper.Get(key)
}

func (config *GlobalConfig) GetPrintable(key string) PrintableConfigEntry {
	return PrintableConfigEntry{Key: key, Value: config.Get(key)}
}

func (config *GlobalConfig) Set(key string, value string) error {
	if key == "format" {
		valid := false
		for _, v := range ValidFormatValues {
			if v == value {
				valid = true
				break
			}
		}
		if !valid {
			return clierr.NewUsageError("invalid value for 'format': %s (valid values: %s)", value, strings.Join(ValidFormatValues[:], ", "))
		}
	}

	if key == "telemetry" {
		if value != "true" && value != "false" {
			return clierr.NewUsageError("invalid value for 'telemetry': %s (valid values: true, false)", value)
		}
	}

	if key == "skill-auto-refresh" {
		if value != "true" && value != "false" {
			return clierr.NewUsageError("invalid value for 'skill-auto-refresh': %s (valid values: true, false)", value)
		}
	}

	if key == "credential-storage" {
		if value != credentials.StorageModeKeyring && value != credentials.StorageModeInsecure {
			return clierr.NewUsageError("invalid value for 'credential-storage': %s (valid values: %s, %s)", value, credentials.StorageModeKeyring, credentials.StorageModeInsecure)
		}
	}

	if key == "history-enabled" {
		if value != "true" && value != "false" {
			return clierr.NewUsageError("invalid value for 'history-enabled': %s (valid values: true, false)", value)
		}
	}

	if key == "history-limit" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return clierr.NewUsageError("invalid value for 'history-limit': %s (must be a non-negative integer)", value)
		}
	}

	if key == "tee-enabled" {
		if value != "true" && value != "false" {
			return clierr.NewUsageError("invalid value for 'tee-enabled': %s (valid values: true, false)", value)
		}
	}

	if key == "accept-env-vars" {
		if value != "true" && value != "false" {
			return clierr.NewUsageError("invalid value for 'accept-env-vars': %s (valid values: true, false)", value)
		}
	}

	if key == "tee-limit" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return clierr.NewUsageError("invalid value for 'tee-limit': %s (must be a non-negative integer)", value)
		}
	}

	data := fileutils.ReadFileSafe(config.fs, config.configPath)

	updated, err := sjson.Set(string(data), key, value)
	if err != nil {
		panic(err)
	}

	fileutils.WriteFile(config.fs, config.configPath, []byte(updated))

	// Sync the new value into viper's in-memory store so callers that call
	// IsSet or GetString in the same process reflect the written value immediately.
	config.viper.Set(key, value)
	return nil
}

func (config *GlobalConfig) Format() string {
	return config.viper.GetString("format")
}

// HistoryEnabled reports whether command-history logging is enabled. Defaults to true.
func (config *GlobalConfig) HistoryEnabled() bool {
	return config.viper.GetBool("history-enabled")
}

// HistoryLimit returns the maximum number of history entries to retain. Defaults
// to 1000; a value of 0 disables history retention.
func (config *GlobalConfig) HistoryLimit() int {
	return config.viper.GetInt("history-limit")
}

// TeeEnabled reports whether tee-on-failure output capture is enabled. Defaults to true.
func (config *GlobalConfig) TeeEnabled() bool {
	return config.viper.GetBool("tee-enabled")
}

// TeeLimit returns the maximum number of tee files to retain per command type.
// Defaults to 20; a value of 0 disables tee retention.
func (config *GlobalConfig) TeeLimit() int {
	return config.viper.GetInt("tee-limit")
}

// CredentialStorage is a read accessor for the persisted credential-storage
// value. It returns StorageModeInsecure when the key is absent from config,
// matching the NewCredentials() boot default. First-run default logic (which
// may choose "keyring" on capable platforms) lives in initCredentialStorageDefault
// in internal/cli/app.go and writes the chosen value before this is called.
func (config *GlobalConfig) CredentialStorage() string {
	if v := config.viper.GetString("credential-storage"); v != "" {
		return v
	}
	return credentials.StorageModeInsecure
}

// CredentialStorageIsSet reports whether "credential-storage" has been
// explicitly written to config.json *or set in-memory* in this process.
// When false, the first-run default detection logic in PersistentPreRunE
// should write the appropriate default.
func (config *GlobalConfig) CredentialStorageIsSet() bool {
	return config.viper.IsSet("credential-storage")
}

// AcceptEnvVars reports whether reading credentials from well-known environment
// variables is enabled. Defaults to false; activatable via config or the
// NEO4J_CLI_ACCEPT_ENV_VARS env var.
func (config *GlobalConfig) AcceptEnvVars() bool {
	return config.viper.GetBool("accept-env-vars")
}

// AcceptEnvVarsIsSet reports whether "accept-env-vars" has been explicitly
// written to config.json, set in-memory in this process, or supplied via
// NEO4J_CLI_ACCEPT_ENV_VARS.
func (config *GlobalConfig) AcceptEnvVarsIsSet() bool {
	return config.viper.IsSet("accept-env-vars")
}

func (config *GlobalConfig) BindFormat(flag *pflag.Flag) {
	if err := config.viper.BindPFlag("format", flag); err != nil {
		panic(err)
	}
}
