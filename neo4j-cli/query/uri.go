// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
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
// Returns:
//   - rewritten:   the URI to feed to neo4j.NewDriver
//   - didRewrite:  true if the scheme/port was changed
//   - displayOrig: redacted form of the input URI suitable for stderr
//
// Inputs that fail to parse, or that use a scheme other than http/https
// (including bolt-family schemes that are already valid for the driver),
// pass through with didRewrite=false.
func normalizeURI(raw string) (rewritten string, didRewrite bool, displayOrig string) {
	u, err := url.Parse(raw)
	if err != nil {
		return raw, false, ""
	}

	scheme := strings.ToLower(u.Scheme)
	var newScheme string
	switch scheme {
	case "http":
		newScheme = "neo4j"
	case "https":
		newScheme = "neo4j+s"
	default:
		// bolt, bolt+s, bolt+ssc, neo4j, neo4j+s, neo4j+ssc, empty,
		// or anything else — passthrough.
		return raw, false, ""
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

	return u.String(), true, displayOrig
}
