// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/neo4j/cli/internal/clicfg/fileutils"
	"github.com/spf13/afero"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tidwall/sjson"
)

// AuraConfig is the Aura settings persisted in config.json under "aura.*"
// (base-url, auth-url, default-workspace). State that only lives for one
// invocation is in AuraRuntime.
type AuraConfig struct {
	viper           *viper.Viper
	fs              afero.Fs
	ValidConfigKeys []string
}

func (config *AuraConfig) IsValidConfigKey(key string) bool {
	return slices.Contains(config.ValidConfigKeys, key)
}

func (config *AuraConfig) Get(key string) interface{} {
	// Bit of a hack for a global config key - it's fine with just the one value but if we're adding more we should refactor
	// TODO: refactor this for global config keys to be properly namespaced (i.e. "format" vs "aura.format") and remove this special case
	if key == "format" {
		return config.viper.Get(key)
	}
	return config.viper.Get(fmt.Sprintf("aura.%s", key))
}

func (config *AuraConfig) GetPrintable(key string) PrintableConfigEntry {
	return PrintableConfigEntry{Key: key, Value: config.Get(key)}
}

func (config *AuraConfig) Set(key string, value string) {
	filename := config.viper.ConfigFileUsed()
	data := fileutils.ReadFileSafe(config.fs, filename)

	updateConfig, err := sjson.Set(string(data), fmt.Sprintf("aura.%s", key), value)
	if err != nil {
		panic(err)
	}

	if key == "base-url" {
		updatedAuraBaseUrl := config.auraBaseUrlOnConfigChange(value)
		intermediateUpdateConfig, err := sjson.Set(string(updateConfig), "aura.base-url", updatedAuraBaseUrl)
		if err != nil {
			panic(err)
		}
		updateConfig = intermediateUpdateConfig
	}

	fileutils.WriteFile(config.fs, filename, []byte(updateConfig))
}

func (config *AuraConfig) BaseUrl() string {
	originalUrl := config.viper.GetString("aura.base-url")
	//Existing users have base url configs with trailing path /v1.
	//To make it backward compatible, we allow old config and clear up by removing trailing path /v1 in the url
	return removePathParametersFromUrl(originalUrl)
}

func removePathParametersFromUrl(originalUrl string) string {
	parsedUrl, err := url.Parse(originalUrl)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s://%s", parsedUrl.Scheme, parsedUrl.Host)
}

func (config *AuraConfig) BindBaseUrl(flag *pflag.Flag) {
	if err := config.viper.BindPFlag("aura.base-url", flag); err != nil {
		panic(err)
	}
}

func (config *AuraConfig) AuthUrl() string {
	return config.viper.GetString("aura.auth-url")
}

func (config *AuraConfig) BindAuthUrl(flag *pflag.Flag) {
	if err := config.viper.BindPFlag("aura.auth-url", flag); err != nil {
		panic(err)
	}
}

// DefaultWorkspace returns the raw value of aura.default-workspace (e.g. "{orgId}/{projectId}").
// Returns an empty string when not set.
func (config *AuraConfig) DefaultWorkspace() string {
	return config.viper.GetString("aura.default-workspace")
}

// DefaultTenant resolves the default tenant/project ID for Aura commands.
// Resolution order:
//  1. Project portion of aura.default-workspace (the part after the '/' in "{orgId}/{projectId}").
//  2. Legacy aura.default-tenant config key as a fallback.
//
// Returns an empty string when neither is set.
func (config *AuraConfig) DefaultTenant() string {
	if ctx := config.viper.GetString("aura.default-workspace"); ctx != "" {
		if idx := strings.LastIndex(ctx, "/"); idx >= 0 {
			return ctx[idx+1:]
		}
	}
	return config.viper.GetString("aura.default-tenant")
}

func (config *AuraConfig) auraBaseUrlOnConfigChange(url string) string {
	if url == "" {
		return DefaultAuraBaseUrl
	}
	return removePathParametersFromUrl(url)
}
