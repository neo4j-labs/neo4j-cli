// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	engine "github.com/neo4j/cli/internal/feedback"
	"path/filepath"
	"time"

	"github.com/neo4j/cli/internal/clicfg"
)

// Config keys. They are written through GlobalConfig.Set but deliberately are
// not in ValidConfigKeys, so `neo4j-cli config set` cannot write them.
const (
	keyAuthorizedUntil = "feedback.authorized-until"
	keyEndpoint        = "feedback.endpoint"
)

// Authorization reasons reported by `submit` when it skips.
const (
	// endpointEnv overrides the feedback destination (dev/test).
	endpointEnv = "NEO4J_CLI_FEEDBACK_ENDPOINT"

	hintDoNotTrack   = "DO_NOT_TRACK=1 is set, so feedback is off. Do not try to work around it; continue the task."
	hintTelemetryOff = "Telemetry is turned off in neo4j-cli's settings, so feedback is off. Do not try to work around it; continue the task."
)

const (
	reasonNotEnabled   = "not_enabled"
	reasonExpired      = "authorization_expired"
	reasonDoNotTrack   = "do_not_track"
	reasonTelemetryOff = "telemetry_off"
)

// getString reads key from the config file on every call rather than from the
// values loaded at startup, so a long-running `feedback serve` sees a
// `feedback disable`, an expiry or a decline written by another process. An
// unreadable file reads as unset, which fails closed: not authorized.
func getString(cfg *clicfg.Config, key string) string {
	v, err := cfg.Global.ReadFresh(key)
	if err != nil {
		return ""
	}
	return v
}

// setValues stores feedback state in one write and returns an error, never
// panicking, when the config file is not writable.
func setValues(cfg *clicfg.Config, kv ...[2]string) error {
	return cfg.Global.SetValues(kv...)
}

// doNotTrack reports whether DO_NOT_TRACK=1 is set. It switches feedback off
// everywhere it could send or ask; it can never switch anything on.
func doNotTrack() bool { return lookupEnv("DO_NOT_TRACK") == "1" }

// telemetryOff reports whether the person turned the CLI's telemetry off
// (`neo4j-cli config set telemetry false`). Feedback follows that setting.
func telemetryOff(cfg *clicfg.Config) bool { return getString(cfg, "telemetry") == "false" }

// optOut returns why the person's privacy settings switch feedback off, or ""
// when they don't: DO_NOT_TRACK=1, or telemetry turned off. Either wins over an
// authorization, and neither can ever switch anything on.
func optOut(cfg *clicfg.Config) string {
	switch {
	case doNotTrack():
		return reasonDoNotTrack
	case telemetryOff(cfg):
		return reasonTelemetryOff
	}
	return ""
}

// authorizedUntil returns the stored expiry. An expiry further ahead than
// `enable` can set (maxDays, plus a day of slack) was not written by it, so it
// counts as no authorization at all.
func authorizedUntil(cfg *clicfg.Config) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, getString(cfg, keyAuthorizedUntil))
	if err != nil || t.After(now().Add((maxDays+1)*24*time.Hour)) {
		return time.Time{}, false
	}
	return t, true
}

// authState returns "" when feedback is authorized at now, otherwise the skip
// reason: not_enabled (unset or unparsable) or authorization_expired.
func authState(cfg *clicfg.Config, now time.Time) string {
	until, ok := authorizedUntil(cfg)
	if !ok {
		return reasonNotEnabled
	}
	if !until.After(now) {
		return reasonExpired
	}
	return ""
}

// Authorized reports whether authorized-until parses as RFC3339 and is after now.
func Authorized(cfg *clicfg.Config, now time.Time) bool {
	return authState(cfg, now) == ""
}

func endpoint(cfg *clicfg.Config) string { return getString(cfg, keyEndpoint) }

// fallbackEndpoint is used when no endpoint was stored: the operator-only
// loopback override, then the compiled-in default.
func fallbackEndpoint() string {
	if ep := lookupEnv(endpointEnv); ep != "" {
		return ep
	}
	return engine.DefaultEndpoint()
}

func receiptsPath() string {
	return filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "feedback-receipts.jsonl")
}
