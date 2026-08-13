// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"github.com/tidwall/sjson"
)

// Flag describes a single registered feature flag. The dotted Name
// (e.g. "flag.<area>-<feature>") is the canonical config-file key and the
// source of truth for the derived env-var (see FlagNameToEnv).
type Flag struct {
	// Name is the full dotted key, e.g. "flag.<area>-<feature>".
	Name string
	// Default is the value returned when no override, env var, config-file
	// value, or legacy alias is present.
	Default bool
	// Owner is the team/area responsible for the flag.
	Owner string
	// Gates is a free-form description of what the flag gates.
	Gates string
	// IntroducedIn is the semver version in which the flag was introduced.
	IntroducedIn string
	// RemovalCondition describes when the flag is expected to be removed.
	RemovalCondition string
	// LegacyKey, when non-empty, is read as a fallback config-file key for
	// backwards compatibility. Its presence triggers a one-shot debug log.
	LegacyKey string
}

// Registry is the source of truth for all registered feature flags. Add a new
// entry here to introduce a flag; callers gate behaviour via
// (*Config).Flags.Enabled("flag.<area>-<feature>").
var Registry = map[string]Flag{
	"flag.mcp-server": {
		Name:             "flag.mcp-server",
		Default:          false,
		Owner:            "neo4j-cli-workinggroup",
		Gates:            "the `neo4j-cli mcp` command group — the stdio MCP server plus its Claude Desktop bundle/install leaves. When disabled the group is not registered on the root command at all.",
		IntroducedIn:     "1.12.0",
		RemovalCondition: "GA of `neo4j-cli mcp`: this entry and the gated branch in app.go are deleted in the same PR. Never flipped to default true.",
	},
}

// FlagSet wraps the runtime override surface (env vars + config file via viper)
// with an in-process override map for tests and a per-legacy-key sync.Once that
// guarantees the deprecated-key debug log fires at most once per process.
type FlagSet struct {
	viper         *viper.Viper
	fs            afero.Fs
	configPath    string
	mu            sync.Mutex
	overrides     map[string]bool
	legacyLogOnce map[string]*sync.Once
}

// Enabled returns the resolved boolean value for the named flag. Resolution
// order, highest to lowest:
//  1. In-process override set via SetForTest.
//  2. viper.IsSet(name) — covers env vars bound by FlagNameToEnv plus
//     explicit config-file values.
//  3. spec.LegacyKey when present and explicitly set (fires a one-shot
//     slog.Debug per legacy key per process).
//  4. spec.Default.
//
// An unknown name (not in Registry) emits a debug log and returns false.
func (f *FlagSet) Enabled(name string) bool {
	spec, ok := Registry[name]
	if !ok {
		slog.Debug("feature-flag lookup for unregistered key", "key", name)
		return false
	}

	f.mu.Lock()
	if v, exists := f.overrides[name]; exists {
		f.mu.Unlock()
		return v
	}
	f.mu.Unlock()

	if f.viper != nil && f.viper.IsSet(name) {
		return f.viper.GetBool(name)
	}

	if spec.LegacyKey != "" && f.viper != nil && f.viper.IsSet(spec.LegacyKey) {
		f.logLegacyOnce(spec)
		return f.viper.GetBool(spec.LegacyKey)
	}

	return spec.Default
}

func (f *FlagSet) logLegacyOnce(spec Flag) {
	f.mu.Lock()
	if f.legacyLogOnce == nil {
		f.legacyLogOnce = map[string]*sync.Once{}
	}
	once, ok := f.legacyLogOnce[spec.LegacyKey]
	if !ok {
		once = &sync.Once{}
		f.legacyLogOnce[spec.LegacyKey] = once
	}
	f.mu.Unlock()

	once.Do(func() {
		slog.Debug("feature flag read from deprecated key", "deprecated", spec.LegacyKey, "new", spec.Name)
	})
}

// SetForTest installs an in-process override for the named flag. The value is
// never persisted to disk or viper; it is cleared when the process exits.
// Panics if name is not in Registry — an unregistered override is always
// ignored by Enabled.
func (f *FlagSet) SetForTest(name string, value bool) {
	if _, ok := Registry[name]; !ok {
		panic(fmt.Sprintf("clicfg.SetForTest: %q is not in Registry — the override would be ignored by Enabled", name))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.overrides == nil {
		f.overrides = map[string]bool{}
	}
	f.overrides[name] = value
}

// SetFromConfigCmd is used by `neo4j-cli config set flag.<area>-<feature>
// <value>`. It validates the key against the Registry and the value as a
// boolean ("true"/"false"), then writes the parsed bool into the config file
// via sjson + fileutils.WriteFile. Unknown keys or invalid values are returned
// as clierr.UsageError so the top-level main exits 2.
func (f *FlagSet) SetFromConfigCmd(name, value string) error {
	if _, ok := Registry[name]; !ok {
		return clierr.NewUsageError("invalid config key: %q", name)
	}

	var parsed bool
	switch value {
	case "true":
		parsed = true
	case "false":
		parsed = false
	default:
		return clierr.NewUsageError("invalid value for %q: %s (valid values: true, false)", name, value)
	}

	filename := f.configPath
	if filename == "" && f.viper != nil {
		filename = f.viper.ConfigFileUsed()
	}

	data := fileutils.ReadFileSafe(f.fs, filename)
	// sjson treats unescaped "." as a path separator. The canonical config
	// key for a flag IS a literal dotted string ("flag.<area>-<feature>"),
	// so escape the dots before passing to sjson.
	sjsonPath := strings.ReplaceAll(name, ".", `\.`)
	updated, err := sjson.Set(string(data), sjsonPath, parsed)
	if err != nil {
		return err
	}
	fileutils.WriteFile(f.fs, filename, []byte(updated))
	return nil
}

// FlagNameToEnv derives the environment-variable name from a flag name. It
// strips the leading "flag." prefix, uppercases the remainder, replaces "-"
// with "_", and prepends "NEO4J_CLI_FLAG_". Pure / dependency-free so it can
// be called at package-init time during viper env binding.
func FlagNameToEnv(name string) string {
	trimmed := strings.TrimPrefix(name, "flag.")
	upper := strings.ToUpper(trimmed)
	return "NEO4J_CLI_FLAG_" + strings.ReplaceAll(upper, "-", "_")
}
