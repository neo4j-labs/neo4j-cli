// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"net/url"
	"slices"
	"strings"
)

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

// EndpointAllowed accepts plain http to a loopback host, or https to one of
// Mixpanel's ingestion hosts (no port, no path).
func EndpointAllowed(raw string) bool {
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
var sendAllowed = EndpointAllowed

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

// DefaultEndpoint is the destination compiled into this build: Mixpanel, once
// the build carries a project token. A build without a token has none, so
// nothing can be pointed at Mixpanel by accident.
func DefaultEndpoint() string {
	if mixpanelToken == "" {
		return ""
	}
	return mixpanelBase
}
