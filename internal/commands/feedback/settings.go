// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/neo4j/cli/internal/clicfg"
)

// Config keys. They are written through GlobalConfig.Set but deliberately are
// not in ValidConfigKeys, so `neo4j-cli config set` cannot write them.
const (
	keyAuthorizedUntil = "feedback.authorized-until"
	keyEndpoint        = "feedback.endpoint"
	keyInstructions    = "feedback.instructions"
)

// Authorization reasons reported by `submit` when it skips.
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

// mixpanelHosts are the only non-loopback destinations a report may be sent
// to: Mixpanel's ingestion hosts for the US, EU and India data residency
// regions, over https and with no path, so the region is the only choice.
var mixpanelHosts = []string{"api.mixpanel.com", "api-eu.mixpanel.com", "api-in.mixpanel.com"}

// isLoopbackEndpoint reports whether raw is plain http to a loopback host (the
// local test destination).
func isLoopbackEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || !plainURL(u) {
		return false
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// plainURL reports whether u has no user, query (not even an empty "?"),
// fragment or escaped path: anything that could change where the request
// goes or what it asks for once "/track?ip=0&verbose=1" is added.
func plainURL(u *url.URL) bool {
	return u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && u.RawFragment == "" &&
		u.RawPath == "" && u.Opaque == ""
}

// endpointAllowed accepts plain http to a loopback host, or https to one of
// Mixpanel's ingestion hosts (no port, no path).
func endpointAllowed(raw string) bool {
	if isLoopbackEndpoint(raw) {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Port() != "" || (u.Path != "" && u.Path != "/") || !plainURL(u) {
		return false
	}
	return slices.Contains(mixpanelHosts, u.Hostname())
}

// sendAllowed is the destination check Send applies; a seam only so the proxy
// tests can send to a host that exists only behind their test proxy.
var sendAllowed = endpointAllowed

// trackURL is the request URL for an allowed endpoint: its path plus /track,
// with IP geolocation off (ip=0) and Mixpanel's detailed reply (verbose=1).
// It is built from parts, never by joining strings.
func trackURL(endpoint string) (string, bool) {
	if !sendAllowed(endpoint) {
		return "", false
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", false
	}
	target := url.URL{Scheme: u.Scheme, Host: u.Host, Path: strings.TrimRight(u.Path, "/") + "/track", RawQuery: "ip=0&verbose=1"}
	return target.String(), true
}

// defaultEndpoint is the destination compiled into this build: Mixpanel, once
// the build carries a project token. A build without a token has none, so
// nothing can be pointed at Mixpanel by accident.
func defaultEndpoint() string {
	if mixpanelToken == "" {
		return ""
	}
	return mixpanelBase
}

// fallbackEndpoint is used when no endpoint was stored: the operator-only
// loopback override, then the compiled-in default.
func fallbackEndpoint() string {
	if ep := lookupEnv(endpointEnv); ep != "" {
		return ep
	}
	return defaultEndpoint()
}

func receiptsPath() string {
	return filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "feedback-receipts.jsonl")
}
