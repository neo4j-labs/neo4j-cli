// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/neo4j/cli/common/analytics"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/configmigrate"
	"github.com/spf13/afero"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tidwall/sjson"
)

var ConfigPrefix string

const (
	DefaultAuraBaseUrl      = "https://api.neo4j.io"
	DefaultAuraAuthUrl      = "https://api.neo4j.io/oauth/token"
	DefaultmixpanelEndpoint = "https://api.mixpanel.com"
	DefaultmixpanelToken    = "4bfb2414ab973c741b6f067bf06d5575"
)

var ValidFormatValues = [4]string{"default", "json", "table", "toon"}

type ConfigScope string

const (
	GlobalScope ConfigScope = "global"
	AuraScope   ConfigScope = "aura"
	SkillsScope ConfigScope = "skills"
	QueryScope  ConfigScope = "query"
	FlagScope   ConfigScope = "flag"
)

type Config struct {
	Version     string
	Aura        *AuraConfig
	Global      *GlobalConfig
	Flags       *FlagSet
	Credentials *credentials.Credentials
	Events      analytics.Service // Look to refactor this in the future , pull this into an application struct
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
		if err := fs.MkdirAll(configPath, 0o700); err != nil {
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

	// Apply any pending forward-only config migrations, then re-read so Viper
	// sees migrated values. Run never returns a non-nil error in this design,
	// but we always re-read because Run may have rewritten the file on disk.
	_, _ = configmigrate.Run(fs, fullConfigPath, os.Stderr)
	if err := Viper.ReadInConfig(); err != nil {
		fmt.Println("Cannot re-read config file after migration.")
		panic(err)
	}

	creds := credentials.NewCredentials(fs, ConfigPrefix)

	logger := slog.Default()

	events := analytics.NewAnalytics(DefaultmixpanelToken, DefaultmixpanelEndpoint, "NEO4J-CLI", version, logger)
	if shouldDisableTelemetry(Viper, os.Getenv) {
		events.Disable()
	}
	globalConfig := &GlobalConfig{
		fs:              fs,
		viper:           Viper,
		configPath:      fullConfigPath,
		ValidConfigKeys: []string{"format", "telemetry", "skill-auto-refresh", "credential-storage", "history-enabled", "history-limit", "tee-enabled", "tee-limit", "accept-env-vars"},
	}

	// Wire the storage mode only when credential-storage is explicitly set in
	// config. When absent the credentials default to insecure mode (backwards
	// compatible). initCredentialStorageDefault writes the key on first run;
	// subsequent invocations see an explicit value and reach this branch.
	if Viper.IsSet("credential-storage") {
		creds.SetStorageMode(globalConfig.CredentialStorage(), os.Stderr)
	}

	validAuraConfigKeys := []string{"auth-url", "base-url", "default-workspace"}
	if scope == AuraScope {
		validAuraConfigKeys = append(validAuraConfigKeys, globalConfig.ValidConfigKeys...)
	}

	return &Config{
		Version: version,
		Aura: &AuraConfig{
			fs:    fs,
			viper: Viper, pollingOverride: PollingConfig{
				MaxRetries: 60,
				Interval:   20,
			},
			ValidConfigKeys: validAuraConfigKeys,
		},
		Global: globalConfig,
		Flags: &FlagSet{
			viper:      Viper,
			fs:         fs,
			configPath: fullConfigPath,
		},
		Credentials: creds,
		Events:      events,
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
			// Skip keys that are already included as global keys to avoid duplication
			if c.Global.IsValidConfigKey(key) {
				continue
			}
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

func (e PrintableConfigEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		e.Key: e.Value,
	})
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

// MarshalJSON renders ConfigData as a flat map {key: value, ...} so that
// PrintBodyMap JSON output is {"format": "json", ...} rather than an array.
func (d PrintableConfigData) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{}, len(d))
	for _, e := range d {
		m[e.Key] = e.Value
	}
	return json.Marshal(m)
}

func bindEnvironmentVariables(Viper *viper.Viper) {
	Viper.BindEnv("aura.base-url", "AURA_BASE_URL")               //nolint:errcheck // BindEnv only errors on zero key args, which cannot happen here
	Viper.BindEnv("aura.auth-url", "AURA_AUTH_URL")               //nolint:errcheck // BindEnv only errors on zero key args, which cannot happen here
	Viper.BindEnv("accept-env-vars", "NEO4J_CLI_ACCEPT_ENV_VARS") //nolint:errcheck // BindEnv only errors on zero key args, which cannot happen here

	// Bind one env var per registered feature flag. Names are derived
	// purely from the flag name via FlagNameToEnv (e.g. "flag.aura-beta"
	// -> "NEO4J_CLI_FLAG_AURA_BETA").
	for name := range Registry {
		Viper.BindEnv(name, FlagNameToEnv(name)) //nolint:errcheck // BindEnv only errors on zero key args, which cannot happen here
	}
}

func setDefaultValues(Viper *viper.Viper) {
	Viper.SetDefault("aura.base-url", DefaultAuraBaseUrl)
	Viper.SetDefault("aura.auth-url", DefaultAuraAuthUrl)
	Viper.SetDefault("format", "default")
	Viper.SetDefault("telemetry", true)
	Viper.SetDefault("skill-auto-refresh", true)
	Viper.SetDefault("history-enabled", true)
	Viper.SetDefault("history-limit", 1000)
	Viper.SetDefault("tee-enabled", true)
	Viper.SetDefault("tee-limit", 20)

	// Feature-flag defaults are intentionally NOT seeded into viper:
	// viper.IsSet returns true whenever a default is registered, which
	// would defeat both the primary "explicitly set" detection and the
	// legacy-fallback gate in FlagSet.Enabled. The default lives in the
	// Registry and is the final precedence layer in FlagSet.Enabled.
}

type AuraConfig struct {
	viper            *viper.Viper
	fs               afero.Fs
	pollingOverride  PollingConfig
	ValidConfigKeys  []string
	activeCredential *credentials.AuraCredential
	debug            bool
}

type PollingConfig struct {
	Interval   int
	MaxRetries int
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

// SetActiveCredential stores a per-invocation credential override. The value is
// never persisted to disk or viper; it is cleared when the process exits.
func (config *AuraConfig) SetActiveCredential(cred *credentials.AuraCredential) {
	config.activeCredential = cred
}

// ActiveCredential returns the credential previously stored by SetActiveCredential,
// or nil when no override has been set.
func (config *AuraConfig) ActiveCredential() *credentials.AuraCredential {
	return config.activeCredential
}

// SetDebug stores the resolved --debug state for this invocation. The value is
// never persisted; it is set once by the Aura root PersistentPreRunE and read
// by the api package (MakeRequest/getToken/Poll), which receive *Config rather
// than *cobra.Command.
func (config *AuraConfig) SetDebug(enabled bool) {
	config.debug = enabled
}

// Debug reports whether debug output is enabled for this invocation.
func (config *AuraConfig) Debug() bool {
	return config.debug
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
// in neo4j-cli/app/app.go and writes the chosen value before this is called.
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

// GatedGetenv returns os.Getenv(name) only when accept-env-vars is enabled;
// otherwise it returns "" so credential env vars are ignored. It is the single
// gate shared by the DBMS connection and embed resolvers; the dotenv (--env
// walk-up) mechanism is intentionally NOT routed through it. The receiver and
// its Global are nil-checked so callers can pass a partially constructed Config
// (the embed :embed leaf relies on this).
func (c *Config) GatedGetenv(name string) string {
	if c == nil || c.Global == nil || !c.Global.AcceptEnvVars() {
		return ""
	}
	return os.Getenv(name)
}

func (config *GlobalConfig) BindFormat(flag *pflag.Flag) {
	if err := config.viper.BindPFlag("format", flag); err != nil {
		panic(err)
	}
}

// ResolveConfigKey resolves a dot-notation key string against the provided Config
// and returns which namespace it belongs to (GlobalScope, AuraScope, or FlagScope)
// and the resolved key name.
//
// Rules:
//   - Keys prefixed with "flag." resolve to FlagScope if registered, with the
//     full dotted name preserved; unknown flag.* keys are rejected.
//   - Keys prefixed with "aura." resolve to AuraScope; the prefix is stripped.
//   - All other keys resolve to GlobalScope.
//   - Keys that exist in GlobalScope (e.g. "format") can never be addressed via
//     the "aura." prefix — "aura.format" is always rejected as invalid.
//   - Unrecognised keys in any namespace return an error.
func ResolveConfigKey(key string, cfg *Config) (ConfigScope, string, error) {
	const (
		auraPrefix = "aura."
		flagPrefix = "flag."
	)

	if strings.HasPrefix(key, flagPrefix) {
		if _, ok := Registry[key]; !ok {
			return "", "", clierr.NewUsageError("invalid config key: %q", key)
		}
		return FlagScope, key, nil
	}

	if strings.HasPrefix(key, auraPrefix) {
		bareKey := strings.TrimPrefix(key, auraPrefix)

		// Reject if the bare key is a global-only key (e.g. "aura.output" is invalid)
		if cfg.Global.IsValidConfigKey(bareKey) {
			return "", "", clierr.NewUsageError("invalid config key: %q is a global key and cannot be addressed with the \"aura.\" prefix", key)
		}

		if !cfg.Aura.IsValidConfigKey(bareKey) {
			return "", "", clierr.NewUsageError("invalid config key: %q", key)
		}

		return AuraScope, bareKey, nil
	}

	// No prefix — must be a global key
	if !cfg.Global.IsValidConfigKey(key) {
		return "", "", clierr.NewUsageError("invalid config key: %q", key)
	}

	return GlobalScope, key, nil
}
