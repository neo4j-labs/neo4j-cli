// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"net/url"
	"strings"
)

// normalizeURI rewrites bolt-family URIs (bolt://, bolt+s://, bolt+ssc://,
// neo4j://, neo4j+s://, neo4j+ssc://) to the equivalent http(s):// URI for the
// Neo4j HTTP Query API. The rewrite covers the scheme and, for the canonical
// bolt port 7687, the port (→ 7473 for HTTPS or 7474 for HTTP). Any other port
// is preserved (the user clearly meant it). Custom paths/query strings/userinfo
// are preserved on the rewritten URI.
//
// Aura special case: hosts ending in `.neo4j.io` (case-insensitive; apex
// `neo4j.io` matches too) are always rewritten to `https://<host>` with NO
// port — Aura always serves HTTPS on 443 regardless of what the user typed.
//
// Returns:
//   - rewritten:   the URI to use for the HTTP request
//   - didRewrite:  true if the scheme/port was changed
//   - displayOrig: redacted form of the input URI suitable for stderr; userinfo
//     password is masked via (*url.URL).Redacted() so no secret leaks
//
// Inputs that fail to parse, or that use an unrecognised scheme (including
// already-correct http/https), pass through with didRewrite=false. The caller
// is expected to feed `rewritten` to the HTTP client either way; downstream
// transport errors surface naturally for genuine garbage.
func normalizeURI(raw string) (rewritten string, didRewrite bool, displayOrig string) {
	u, err := url.Parse(raw)
	if err != nil {
		return raw, false, ""
	}

	scheme := strings.ToLower(u.Scheme)
	var newScheme string
	switch scheme {
	case "bolt":
		newScheme = "http"
	case "bolt+s", "bolt+ssc", "neo4j", "neo4j+s", "neo4j+ssc":
		newScheme = "https"
	default:
		// http, https, empty, or unknown scheme — passthrough.
		return raw, false, ""
	}

	// Capture the redacted original BEFORE mutating u so password masking is
	// applied to the input form the user typed.
	displayOrig = u.Redacted()

	// Aura special case: any *.neo4j.io host (or apex neo4j.io) on a
	// bolt-family scheme is forced to https with no port. Aura always serves
	// HTTPS on 443. Suffix match (case-insensitive) — must NOT match
	// `*.neo4j.io.evil.com`.
	host := strings.ToLower(u.Hostname())
	if host == "neo4j.io" || strings.HasSuffix(host, ".neo4j.io") {
		u.Scheme = "https"
		u.Host = u.Hostname() // strip any port
		return u.String(), true, displayOrig
	}

	u.Scheme = newScheme
	if u.Port() == "7687" {
		newPort := "7474"
		if newScheme == "https" {
			newPort = "7473"
		}
		u.Host = u.Hostname() + ":" + newPort
	}

	return u.String(), true, displayOrig
}
