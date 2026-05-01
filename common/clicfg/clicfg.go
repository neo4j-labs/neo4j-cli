// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"slices"

	"github.com/neo4j/cli/common/analytics"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/neo4j/cli/common/clicfg/projects"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tidwall/sjson"
)

var ConfigPrefix string

const (
	DefaultAuraBaseUrl      = "https://api.neo4j.io"
	DefaultAuraAuthUrl      = "https://api.neo4j.io/oauth/token"
	DefaultAuraBetaEnabled  = false
	DefaultmixpanelEndpoint = "https://api.mixpanel.com"
	DefaultmixpanelToken    = "4bfb2414ab973c741b6f067bf06d5575"
)

var ValidOutputValues = [3]string{"default", "json", "table"}

type Config struct {
	Version     string
	Aura        *AuraConfig
	Credentials *credentials.Credentials
	Events      *analytics.Analytics // Look to refactor this in the future
}

func NewConfig(fs afero.Fs, version string) *Config {
	configPath := filepath.Join(ConfigPrefix, "neo4j", "cli")
	fullConfigPath := filepath.Join(configPath, "config.json")

	logger := slog.Default()

	events := analytics.NewAnalytics(DefaultmixpanelToken, DefaultmixpanelEndpoint, "NEO4J-CLI", version, logger)

	Viper := viper.New()

	Viper.SetFs(fs)
	Viper.SetConfigName("config")
	Viper.SetConfigType("json")
	Viper.AddConfigPath(configPath)
	Viper.SetConfigPermissions(0600)

	bindEnvironmentVariables(Viper)
	setDefaultValues(Viper)

	if !fileutils.FileExists(fs, fullConfigPath) {
		if err := fs.MkdirAll(configPath, 0755); err != nil {
			panic(err)
		}
		if err := Viper.SafeWriteConfig(); err != nil {
			panic(err)
		}
	}

	if err := Viper.ReadInConfig(); err != nil {
		fmt.Println("Cannot read config file.")
		panic(err)
	}

	credentials := credentials.NewCredentials(fs, ConfigPrefix)
	projects := projects.NewAuraConfigProjects(fs, fullConfigPath)

	return &Config{
		Version: version,
		Aura: &AuraConfig{
			fs:    fs,
			viper: Viper, pollingOverride: PollingConfig{
				MaxRetries: 60,
				Interval:   20,
			},
			ValidConfigKeys: []string{"auth-url", "base-url", "default-tenant", "output"},
			Projects:        projects,
		},
		Credentials: credentials,
		Events:      events,
	}
}

func bindEnvironmentVariables(Viper *viper.Viper) {
	Viper.BindEnv("aura.base-url", "AURA_BASE_URL") //nolint:errcheck // BindEnv only errors on zero key args, which cannot happen here
	Viper.BindEnv("aura.auth-url", "AURA_AUTH_URL") //nolint:errcheck // BindEnv only errors on zero key args, which cannot happen here
}

func setDefaultValues(Viper *viper.Viper) {
	Viper.SetDefault("aura.base-url", DefaultAuraBaseUrl)
	Viper.SetDefault("aura.auth-url", DefaultAuraAuthUrl)
	Viper.SetDefault("aura.output", "default")
	Viper.SetDefault("aura-projects", projects.AuraProjects{Default: "", Projects: map[string]*projects.AuraProject{}})
}

type AuraConfig struct {
	viper           *viper.Viper
	fs              afero.Fs
	pollingOverride PollingConfig
	ValidConfigKeys []string
	Projects        *projects.AuraConfigProjects
	betaEnabled     bool
}

type PollingConfig struct {
	Interval   int
	MaxRetries int
}

func (config *AuraConfig) IsValidConfigKey(key string) bool {
	return slices.Contains(config.ValidConfigKeys, key)
}

func (config *AuraConfig) Get(key string) interface{} {
	return config.viper.Get(fmt.Sprintf("aura.%s", key))
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

func (config *AuraConfig) PrintAuraConfig(cmd *cobra.Command) {
	filtered := make(map[string]interface{}, len(config.ValidConfigKeys))
	for _, key := range config.ValidConfigKeys {
		filtered[key] = config.viper.Get(fmt.Sprintf("aura.%s", key))
	}

	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "\t")

	if err := encoder.Encode(filtered); err != nil {
		panic(err)
	}
}

func (config *AuraConfig) PrintAuraProjects(cmd *cobra.Command) {
	config.print(cmd, "aura-projects")
}

func (config *AuraConfig) print(cmd *cobra.Command, path string) {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "\t")

	if err := encoder.Encode(config.viper.Get(path)); err != nil {
		panic(err)
	}
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

func (config *AuraConfig) BetaPathV1() string {
	return "v1beta5"
}

func (config *AuraConfig) BetaPathV2() string {
	return "v2beta1"
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

func (config *AuraConfig) Output() string {
	return config.viper.GetString("aura.output")
}

func (config *AuraConfig) BindOutput(flag *pflag.Flag) {
	if err := config.viper.BindPFlag("aura.output", flag); err != nil {
		panic(err)
	}
}

func (config *AuraConfig) SetBetaEnabled(enabled bool) {
	config.betaEnabled = enabled
}

func (config *AuraConfig) AuraBetaEnabled() bool {
	return config.betaEnabled
}

func (config *AuraConfig) DefaultTenant() string {
	return config.viper.GetString("aura.default-tenant")
}

func (config *AuraConfig) Fs() afero.Fs {
	return config.fs
}

func (config *AuraConfig) PollingConfig() PollingConfig {
	return config.pollingOverride
}

func (config *AuraConfig) SetPollingConfig(maxRetries int, interval int) {
	config.pollingOverride = PollingConfig{
		MaxRetries: maxRetries,
		Interval:   interval,
	}
}

func (config *AuraConfig) auraBaseUrlOnConfigChange(url string) string {
	if url == "" {
		return DefaultAuraBaseUrl
	}
	return removePathParametersFromUrl(url)
}
