// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import "github.com/neo4j/cli/internal/clicfg/credentials"

// PollingConfig is how an async Aura operation is polled: how many attempts, and
// the delay in seconds between them.
type PollingConfig struct {
	Interval   int
	MaxRetries int
}

// AuraRuntime is Aura state that lives for one invocation and is never
// persisted: it is set once while the command starts and read by the Aura
// transport, which receives *Config rather than *cobra.Command. Persisted Aura
// settings are in AuraConfig.
type AuraRuntime struct {
	debug            bool
	activeCredential *credentials.AuraCredential
	polling          PollingConfig
}

func newAuraRuntime() *AuraRuntime {
	return &AuraRuntime{polling: PollingConfig{MaxRetries: 60, Interval: 20}}
}

// SetDebug stores the resolved --debug state for this invocation.
func (r *AuraRuntime) SetDebug(enabled bool) { r.debug = enabled }

// Debug reports whether Aura debug tracing is enabled for this invocation.
func (r *AuraRuntime) Debug() bool { return r.debug }

// SetActiveCredential overrides the credential used for Aura requests (for
// example one synthesised from environment variables). It is held in memory
// only and never written to credentials.json or the keyring.
func (r *AuraRuntime) SetActiveCredential(cred *credentials.AuraCredential) {
	r.activeCredential = cred
}

// ActiveCredential returns the credential stored by SetActiveCredential, or nil
// when no override has been set.
func (r *AuraRuntime) ActiveCredential() *credentials.AuraCredential { return r.activeCredential }

// PollingConfig returns how async operations are polled.
func (r *AuraRuntime) PollingConfig() PollingConfig { return r.polling }

// SetPollingConfig overrides how async operations are polled.
func (r *AuraRuntime) SetPollingConfig(maxRetries int, interval int) {
	r.polling = PollingConfig{MaxRetries: maxRetries, Interval: interval}
}
