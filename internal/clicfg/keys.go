// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neo4j/cli/internal/clierr"
)

func (c *Config) Printable() PrintableConfigData {
	data := make(PrintableConfigData, 0, len(c.Global.ValidConfigKeys))
	for _, key := range c.Global.ValidConfigKeys {
		data = append(data, PrintableConfigEntry{Key: key, Value: c.Global.Get(key)})
	}

	for _, key := range c.Aura.ValidConfigKeys {
		data = append(data, PrintableConfigEntry{Key: fmt.Sprintf("aura.%s", key), Value: c.Aura.Get(key)})
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
