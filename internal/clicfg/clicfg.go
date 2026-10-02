// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/neo4j/cli/internal/analytics"
	"github.com/neo4j/cli/internal/clicfg/credentials"
	"github.com/neo4j/cli/internal/clicfg/fileutils"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/configmigrate"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
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
	FlagScope   ConfigScope = "flag"
)

type Config struct {
	Version     string
	Aura        *AuraConfig
	AuraRuntime *AuraRuntime
	Global      *GlobalConfig
	Flags       *FlagSet
	Credentials *credentials.Credentials
	Events      analytics.Service // Look to refactor this in the future , pull this into an application struct
	fs          afero.Fs
}

// Fs returns the filesystem the config, credentials and every other piece of
// persisted CLI state is read from and written to. Production passes the OS
// filesystem; tests pass an in-memory one.
func (c *Config) Fs() afero.Fs {
	return c.fs
}

// DbmsCredentials returns the local DBMS credential store, or nil when
// credential storage is unavailable. Callers that can carry on without one pass
// the result straight on instead of checking the Credentials chain themselves.
func (c *Config) DbmsCredentials() *credentials.DbmsCredentials {
	if c == nil || c.Credentials == nil {
		return nil
	}
	return c.Credentials.Dbms
}

// NewConfig is Load for callers that cannot continue without a config (tests,
// helpers): it panics on failure. Production code uses Load and reports the
// error.
func NewConfig(fs afero.Fs, version string) *Config {
	cfg, err := Load(fs, version)
	if err != nil {
		panic(err)
	}
	return cfg
}

// Load reads the config file under ConfigPrefix (creating it with defaults on
// first run), applies any pending config migrations, and wires the credential
// store and analytics. A config file that cannot be created or parsed is
// returned as an error naming the file, rather than panicking, so a corrupt
// config.json is reported cleanly.
func Load(fs afero.Fs, version string) (*Config, error) {
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
			return nil, clierr.NewFatalError("cannot create the config directory %s: %w", configPath, err)
		}
		if err := Viper.SafeWriteConfig(); err != nil {
			return nil, clierr.NewFatalError("cannot create the config file %s: %w", fullConfigPath, err)
		}
	}

	if err := Viper.ReadInConfig(); err != nil {
		return nil, clierr.NewFatalError("cannot read the config file %s: %w. Fix or remove the file to continue", fullConfigPath, err)
	}

	// Apply any pending forward-only config migrations, then re-read so Viper
	// sees migrated values. Run never returns a non-nil error in this design,
	// but we always re-read because Run may have rewritten the file on disk.
	_, _ = configmigrate.Run(fs, fullConfigPath, os.Stderr)
	if err := Viper.ReadInConfig(); err != nil {
		return nil, clierr.NewFatalError("cannot re-read the config file %s after migration: %w", fullConfigPath, err)
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

	return &Config{
		Version: version,
		Aura: &AuraConfig{
			fs:              fs,
			viper:           Viper,
			ValidConfigKeys: validAuraConfigKeys,
		},
		AuraRuntime: newAuraRuntime(),
		Global:      globalConfig,
		Flags: &FlagSet{
			viper:      Viper,
			fs:         fs,
			configPath: fullConfigPath,
		},
		Credentials: creds,
		Events:      events,
		fs:          fs,
	}, nil
}

func bindEnvironmentVariables(Viper *viper.Viper) {
	Viper.BindEnv("aura.base-url", "AURA_BASE_URL")               //nolint:errcheck // BindEnv only errors on zero key args, which cannot happen here
	Viper.BindEnv("aura.auth-url", "AURA_AUTH_URL")               //nolint:errcheck // BindEnv only errors on zero key args, which cannot happen here
	Viper.BindEnv("accept-env-vars", "NEO4J_CLI_ACCEPT_ENV_VARS") //nolint:errcheck // BindEnv only errors on zero key args, which cannot happen here

	// Bind one env var per registered feature flag. Names are derived
	// purely from the flag name via FlagNameToEnv (e.g. "flag.docker-command"
	// -> "NEO4J_CLI_FLAG_DOCKER_COMMAND").
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
