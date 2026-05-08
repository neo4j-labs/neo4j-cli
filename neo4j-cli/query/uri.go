// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// normalizeURI rewrites HTTP-family URIs to their Bolt equivalent so the
// neo4j-go-driver can connect. The query command speaks the Bolt protocol
// natively, but users (and the previous CLI default) frequently still pass
// `http://localhost:7474` or `https://...:7473`. The rewrite turns:
//
//   - http://<host>[:<port>][/...]   → neo4j://<host>:7687
//   - https://<host>[:<port>][/...]  → neo4j+s://<host>:7687
//
// Path and query are stripped (the driver does not use them). Userinfo is
// preserved on the rewritten URI; the displayOrig form is run through
// (*url.URL).Redacted() so any password is masked before it hits stderr.
//
// After the (optional) rewrite, the resolved scheme is inspected: if it is
// the cleartext bolt-family (bolt:// or neo4j://) AND the host is not a
// loopback address, a warning string is returned. The caller is expected to
// print the warning to stderr (e.g. via cmd.PrintErrln). The warning text
// uses (*url.URL).Redacted() so any userinfo password is masked.
//
// Returns:
//   - rewritten:   the URI to feed to neo4j.NewDriver
//   - didRewrite:  true if the scheme/port was changed
//   - displayOrig: redacted form of the input URI suitable for stderr
//   - warning:     non-empty when the resolved URI is cleartext bolt/neo4j to
//     a non-loopback host; empty otherwise
//
// Inputs that fail to parse pass through with didRewrite=false and warning="".
// Bolt-family inputs pass through with didRewrite=false but may still produce
// a warning when cleartext-to-non-loopback.
func normalizeURI(raw string) (rewritten string, didRewrite bool, displayOrig, warning string) {
	u, err := url.Parse(raw)
	if err != nil {
		return raw, false, "", ""
	}

	scheme := strings.ToLower(u.Scheme)
	var newScheme string
	switch scheme {
	case "http":
		newScheme = "neo4j"
	case "https":
		newScheme = "neo4j+s"
	case "bolt", "neo4j", "bolt+s", "bolt+ssc", "neo4j+s", "neo4j+ssc":
		// Already a bolt-family scheme — passthrough, but still check whether
		// the resolved (cleartext) form warrants a warning.
		return raw, false, "", cleartextWarning(u, scheme)
	default:
		// Empty scheme, gibberish, or any other unsupported scheme — passthrough.
		return raw, false, "", ""
	}

	// Capture the redacted original BEFORE mutating u so password masking is
	// applied to the input form the user typed.
	displayOrig = u.Redacted()

	// Rewrite scheme, force port 7687, strip path/query/fragment.
	u.Scheme = newScheme
	u.Host = u.Hostname() + ":7687"
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	u.RawFragment = ""

	return u.String(), true, displayOrig, cleartextWarning(u, newScheme)
}

// cleartextWarning returns a non-empty warning when the URI's scheme is the
// cleartext bolt-family (bolt:// or neo4j://) and the host is NOT a loopback
// address. Loopback hosts are silent: bolt://localhost, bolt://127.0.0.1,
// and bolt://[::1] all return "". Encrypted schemes (neo4j+s://, bolt+s://,
// neo4j+ssc://, bolt+ssc://) always return "". The returned text uses
// (*url.URL).Redacted() so any userinfo password is masked.
func cleartextWarning(u *url.URL, scheme string) string {
	if scheme != "bolt" && scheme != "neo4j" {
		return ""
	}
	host := u.Hostname()
	if host == "" {
		return ""
	}
	if isLoopbackHost(host) {
		return ""
	}
	return fmt.Sprintf(
		"warning: connecting to '%s' over cleartext (use %s+s:// for verified TLS or %s+ssc:// for self-signed)",
		u.Redacted(), scheme, scheme)
}

// isLoopbackHost reports whether host is loopback. Accepts IP literals
// (127.0.0.0/8, ::1) via net.ParseIP and the conventional hostname
// "localhost" (with or without trailing dot, case-insensitive).
func isLoopbackHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	return h == "localhost"
}
