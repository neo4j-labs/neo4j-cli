// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import (
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/neo4j/cli/common/clicfg/projects"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/afero"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tidwall/sjson"
)

var ConfigPrefix string

const (
	DefaultAuraBaseUrl     = "https://api.neo4j.io"
	DefaultAuraAuthUrl     = "https://api.neo4j.io/oauth/token"
	DefaultAuraBetaEnabled = false
)

var ValidOutputValues = [3]string{"default", "json", "table"}

type ConfigScope string

const (
	GlobalScope ConfigScope = "global"
	AuraScope   ConfigScope = "aura"
)

type Config struct {
	Version     string
	Aura        *AuraConfig
	Global      *GlobalConfig
	Credentials *credentials.Credentials
	scope       ConfigScope
}

func NewConfig(fs afero.Fs, version string, scope ConfigScope) *Config {
	configPath := filepath.Join(ConfigPrefix, "neo4j", "cli")
	fullConfigPath := filepath.Join(configPath, "config.json")

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

	// NOTE: The migration block below is intentionally commented out.
	// This experimental release has never shipped to users, so the migration
	// has never run in the field. Users may switch between the stable CLI
	// (which still uses "aura.output") and this experimental version; running
	// the migration would corrupt stable-version config files.
	// This code is preserved as reference for a future stable-release upgrade path.
	//
	// {
	// 	data := fileutils.ReadFileSafe(fs, fullConfigPath)
	// 	if gjson.GetBytes(data, "aura.output").Exists() && !gjson.GetBytes(data, "output").Exists() {
	// 		oldValue := gjson.GetBytes(data, "aura.output").String()
	// 		updated, err := sjson.Set(string(data), "output", oldValue)
	// 		if err == nil {
	// 			updated, err = sjson.Delete(updated, "aura.output")
	// 			if err == nil {
	// 				fileutils.WriteFile(fs, fullConfigPath, []byte(updated))
	// 				if err := Viper.ReadInConfig(); err != nil {
	// 					fmt.Println("Cannot re-read config file after migration.")
	// 					panic(err)
	// 				}
	// 			}
	// 		}
	// 	}
	// }

	credentials := credentials.NewCredentials(fs, ConfigPrefix)
	projects := projects.NewAuraConfigProjects(fs, fullConfigPath)

	globalConfig := &GlobalConfig{
		fs:              fs,
		viper:           Viper,
		configPath:      fullConfigPath,
		ValidConfigKeys: []string{"output"},
	}

	return &Config{
		Version: version,
		Aura: &AuraConfig{
			fs:    fs,
			viper: Viper, pollingOverride: PollingConfig{
				MaxRetries: 60,
				Interval:   20,
			},
			// TODO: just append global here if just aura scope?
			ValidConfigKeys: []string{"auth-url", "base-url", "default-tenant"},
			Projects:        projects,
		},
		Global:      globalConfig,
		Credentials: credentials,
		scope:       scope,
	}
}

func (c *Config) Printable() PrintableConfigData {
	data := make(PrintableConfigData, 0, len(c.Global.ValidConfigKeys))
	for _, key := range c.Global.ValidConfigKeys {
		data = append(data, PrintableConfigEntry{Key: key, Value: c.Global.Get(key)})
	}

	if c.scope == AuraScope {
		auraData := make(PrintableConfigData, 0, len(c.Aura.ValidConfigKeys))
		for _, key := range c.Aura.ValidConfigKeys {
			auraData = append(auraData, PrintableConfigEntry{Key: key, Value: c.Aura.Get(key)})
		}
		data = append(data, auraData...)
	}

	if c.scope == GlobalScope {
		auraData := make(PrintableConfigData, 0, len(c.Aura.ValidConfigKeys))
		for _, key := range c.Aura.ValidConfigKeys {
			auraData = append(auraData, PrintableConfigEntry{Key: fmt.Sprintf("aura.%s", key), Value: c.Aura.Get(key)})
		}
		data = append(data, auraData...)
	}

	return data
}

// PrintableConfigEntry represents a single configuration key-value pair.
type PrintableConfigEntry struct {
	Key   string
	Value interface{}
}

func (e PrintableConfigEntry) AsArray() []map[string]any {
	return []map[string]any{
		{"key": e.Key, "value": e.Value},
	}
}

// PrintableConfigData is a slice of ConfigEntry that satisfies the ResponseData interface,
// enabling config commands to use PrintBodyMap for consistent rendering.
type PrintableConfigData []PrintableConfigEntry

// AsArray returns each entry as a {"key": k, "value": v} map for table rendering.
func (d PrintableConfigData) AsArray() []map[string]any {
	result := make([]map[string]any, len(d))
	for i, e := range d {
		result[i] = map[string]any{
			"key":   e.Key,
			"value": e.Value,
		}
	}
	return result
}

// // MarshalJSON renders ConfigData as a flat map {key: value, ...} so that
// // PrintBodyMap JSON output is {"output": "json", ...} rather than an array.
// func (d PrintableConfigData) MarshalJSON() ([]byte, error) {
// 	m := make(map[string]interface{}, len(d))
// 	for _, e := range d {
// 		m[e.Key] = e.Value
// 	}
// 	return json.Marshal(m)
// }

func bindEnvironmentVariables(Viper *viper.Viper) {
	Viper.BindEnv("aura.base-url", "AURA_BASE_URL") //nolint:errcheck // BindEnv only errors on zero key args, which cannot happen here
	Viper.BindEnv("aura.auth-url", "AURA_AUTH_URL") //nolint:errcheck // BindEnv only errors on zero key args, which cannot happen here
}

func setDefaultValues(Viper *viper.Viper) {
	Viper.SetDefault("aura.base-url", DefaultAuraBaseUrl)
	Viper.SetDefault("aura.auth-url", DefaultAuraAuthUrl)
	Viper.SetDefault("output", "default")
	// TODO: should this become aura.projects?
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
	// Bit of a hack for a global config key
	// TODO: refactor this for global config keys to be properly namespaced (e.g. "output" vs "aura.output") and remove this special case
	if key == "output" {
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

func (config *GlobalConfig) Get(key string) interface{} {
	return config.viper.Get(key)
}

func (config *GlobalConfig) GetPrintable(key string) PrintableConfigEntry {
	return PrintableConfigEntry{Key: key, Value: config.Get(key)}
}

func (config *GlobalConfig) Set(key string, value string) error {
	if key == "output" {
		valid := false
		for _, v := range ValidOutputValues {
			if v == value {
				valid = true
				break
			}
		}
		if !valid {
			return clierr.NewUsageError("invalid value for 'output': %s (valid values: %s)", value, strings.Join(ValidOutputValues[:], ", "))
		}
	}

	data := fileutils.ReadFileSafe(config.fs, config.configPath)

	updated, err := sjson.Set(string(data), key, value)
	if err != nil {
		panic(err)
	}

	fileutils.WriteFile(config.fs, config.configPath, []byte(updated))
	return nil
}

func (config *GlobalConfig) Output() string {
	return config.viper.GetString("output")
}

func (config *GlobalConfig) BindOutput(flag *pflag.Flag) {
	if err := config.viper.BindPFlag("output", flag); err != nil {
		panic(err)
	}
}
