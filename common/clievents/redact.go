// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clievents

import (
	"net/url"
	"regexp"
	"strings"
)

// secretFlags is the canonical list of flag names whose values must never be
// echoed in telemetry, panic templates, or error output. The list is matched
// against both the long form (--flag) and the defensive single-dash form
// (-flag), and both space-separated (--flag value) and equals-separated
// (--flag=value) shapes.
//
// "p" is the `query` password shorthand (StringP("password","p")); it is the
// only -p in the tree, so including it here fails closed — over-redaction in a
// redaction context is acceptable.
var secretFlags = []string{
	"password",
	"p",
	"client-secret",
	"api-key",
	"instance-password",
}

// redactedPlaceholder is what the secret value is replaced with in output.
const redactedPlaceholder = "***"

// secretParamKeyParts are case-insensitive substrings that mark a `--param`
// bind-parameter key as secret-bearing. A query parameter can legitimately
// carry a token/password/api-key, so when the key matches we scrub only the
// value and keep the key visible. The list is intentionally conservative to
// avoid over-redacting common non-secret params (e.g. `limit`, `name`).
var secretParamKeyParts = []string{
	"password",
	"passwd",
	"pwd",
	"secret",
	"token",
	"apikey",
	"api-key",
	"api_key",
	"access-key",
	"accesskey",
	"key",
}

// RedactArgs renders an argv slice as a single space-separated string with
// the values of secret-bearing flags replaced by ***. It handles three argv
// shapes:
//
//	--flag value          -> --flag ***
//	--flag=value          -> --flag=***
//	-flag value           -> -flag ***   (defensive single-dash form)
//
// A trailing secret flag with no following argument is left as-is — there is
// no value to scrub. Positional arguments and non-secret flags are emitted
// unchanged.
//
// For `--param key=value` (and `-param`, both shapes), a best-effort heuristic
// scrubs only the value when the base key (the part before any `:embed`
// modifier and the first `=`) looks secret-bearing — see secretParamKeyParts.
// Non-secret params (e.g. `--param limit=10`) pass through unchanged so the
// history stays useful. This is a name-based heuristic: a secret stored under
// an innocuous key name (e.g. `--param x=sk-live-...`) is NOT caught.
//
// This helper is the single source of truth for "what flag is sensitive" in
// the CLI; telemetry, panic templates, and error formatting all funnel
// through it.
func RedactArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}

	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Equals form (--flag=value / -flag=value).
		if name, value, ok := splitFlagEq(arg); ok {
			if red, sensitive := redactByFlag(name, value); sensitive {
				out = append(out, dashPrefix(arg)+name+"="+red)
				continue
			}
		}

		// Space form (--flag value / -flag value); the value is the next token.
		if name, ok := flagName(arg); ok {
			if _, sensitive := redactByFlag(name, ""); sensitive {
				out = append(out, arg)
				// A value-taking flag with no value must not swallow a following
				// flag: for uri/param a flag-looking next token is not a value, so
				// leaving it unconsumed lets the loop redact it (e.g.
				// `--uri --password hunter2` -> `--uri --password ***`). Generic
				// secret flags still consume unconditionally — the next token
				// becomes ***, so a secret value starting with '-' stays redacted.
				if i+1 < len(args) && (isSecretFlag(name) || !looksLikeFlag(args[i+1])) {
					red, _ := redactByFlag(name, args[i+1])
					out = append(out, red)
					i++
				}
				continue
			}
		}

		out = append(out, arg)
	}
	return strings.Join(out, " ")
}

// textURIUserinfoRe matches a connection URI's userinfo password embedded in
// free text: scheme://user:password@ with the password rewritten to ***. Only
// the password segment (group 1 is scheme://user:) is replaced; host/port/path
// are preserved.
var textURIUserinfoRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://[^\s:/@]+:)[^\s@]+@`)

// textAssignmentRe matches password/secret/token/api-key/auth key-value
// assignments (= or :) in free text and rewrites the value to ***. Group 1
// captures the key and separator so the replacement keeps them verbatim.
// The `auth` alternative deliberately ends at a `[=:]`-bounded word rather than
// using `auth\w*`, so it does NOT swallow the HTTP header name "Authorization"
// (handled by the bearer pass) while still matching auth/auth_token/authkey.
var textAssignmentRe = regexp.MustCompile(`(?i)((?:(?:password|passwd|pwd|secret|token|api[-_]?key)\w*|auth(?:[-_]?token|[-_]?key)?)\s*[=:]\s*)(\S+)`)

// textJSONFieldRe matches a quoted JSON key containing a secret-bearing
// substring followed by a quoted value, rewriting only the value to "***". It
// exists because textAssignmentRe cannot match the JSON shape: the `"` between
// the key and the `:` breaks its `key[=:]value` form, so `"password":"x"` would
// otherwise leak. The key is matched by substring (mirroring secretParamKeyParts)
// so `"client_secret"`, `"x-api-key"`, `"current_password"` are all covered, and
// both `"k":"v"` and `"k": "v"` spacings are handled.
var textJSONFieldRe = regexp.MustCompile(`(?i)("[^"]*(?:password|passwd|pwd|secret|token|api[-_]?key|access[-_]?key)[^"]*"\s*:\s*)"[^"]*"`)

// textBearerRe matches an Authorization Bearer token or Basic credential blob
// in free text and rewrites it to ***. Group 1 captures the scheme word
// (Bearer/Basic) so the replacement keeps the scheme verbatim.
var textBearerRe = regexp.MustCompile(`(?i)((?:bearer|basic)\s+)\S+`)

// RedactText scrubs secrets from arbitrary multi-line text, the text-level
// counterpart to RedactArgs (which is argv-only). It applies conservative
// regexes for (a) URI userinfo passwords in free text, (b) quoted-key JSON
// secret fields (`"password":"x"`), (c) password/secret/token/api-key/auth
// key-value assignments, and (d) Bearer/Basic authorization headers, replacing
// each secret value with ***. It is the single source of truth for redacting
// captured command output before it is persisted. Non-secret text (e.g.
// `limit=10`, ordinary prose) is left intact.
func RedactText(s string) string {
	s = textURIUserinfoRe.ReplaceAllString(s, "${1}"+redactedPlaceholder+"@")
	// JSON pass runs before the bare-assignment pass: it rewrites the value to a
	// quoted "***", whose trailing `"` is not a valid assignment separator, so
	// the assignment regex cannot re-process (and double-mangle) the result.
	s = textJSONFieldRe.ReplaceAllString(s, `${1}"`+redactedPlaceholder+`"`)
	s = textAssignmentRe.ReplaceAllString(s, "${1}"+redactedPlaceholder)
	s = textBearerRe.ReplaceAllString(s, "${1}"+redactedPlaceholder)
	return s
}

// redactByFlag returns the scrubbed value for a known sensitive flag, and
// whether the flag is sensitive at all. Precedence matches the prior branch
// order: uri (userinfo) and param (key=value) before generic secret flags.
func redactByFlag(name, value string) (string, bool) {
	switch {
	case isURIFlag(name):
		return redactURIUserinfo(value), true
	case isParamFlag(name):
		return redactParamValue(value), true
	case isSecretFlag(name):
		return redactedPlaceholder, true
	}
	return value, false
}

// isURIFlag reports whether name (without leading dashes) is a connection-style
// flag whose value may carry credentials in userinfo.
func isURIFlag(name string) bool {
	return name == "uri"
}

// isParamFlag reports whether name (without leading dashes) is the query
// `--param` flag, whose values are bind parameters of the form key=value.
func isParamFlag(name string) bool {
	return name == "param"
}

// redactParamValue scrubs the value of a `--param key=value` token when the
// base key looks secret-bearing, preserving the key (e.g. `token=***`). A
// value without `=` (malformed) is returned unchanged. The base key is the
// part before the first `=` with any `:embed`-style modifier stripped, so
// `token:embed=...` matches on `token`.
func redactParamValue(value string) string {
	idx := strings.Index(value, "=")
	if idx < 0 {
		return value
	}
	key := value[:idx]
	if modIdx := strings.Index(key, ":"); modIdx >= 0 {
		key = key[:modIdx]
	}
	if !isSecretParamKey(key) {
		return value
	}
	return value[:idx+1] + redactedPlaceholder
}

// isSecretParamKey reports whether a bind-parameter key (case-insensitive)
// contains any secret-bearing substring.
func isSecretParamKey(key string) bool {
	lower := strings.ToLower(key)
	for _, part := range secretParamKeyParts {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}

// redactURIUserinfo rewrites a connection URI's userinfo password to *** while
// preserving scheme, user, host, port, and path. Non-URL values, and URLs with
// no embedded password, are returned unchanged.
func redactURIUserinfo(value string) string {
	u, err := url.Parse(value)
	if err != nil || u.User == nil {
		return value
	}
	if _, hasPassword := u.User.Password(); !hasPassword {
		return value
	}
	u.User = url.UserPassword(u.User.Username(), redactedPlaceholder)
	// url.UserPassword percent-encodes "*" to %2A; restore the readable form.
	return strings.Replace(u.String(), "%2A%2A%2A", redactedPlaceholder, 1)
}

// looksLikeFlag reports whether s is a flag-looking token (starts with a dash
// but is not the lone "-" stdin sentinel).
func looksLikeFlag(s string) bool {
	return strings.HasPrefix(s, "-") && s != "-"
}

// flagName returns the bare flag name (no dashes) if arg is a flag token of
// the form --name or -name (no `=`), and false otherwise.
func flagName(arg string) (string, bool) {
	if !strings.HasPrefix(arg, "-") {
		return "", false
	}
	if strings.Contains(arg, "=") {
		return "", false
	}
	stripped := strings.TrimLeft(arg, "-")
	if stripped == "" {
		return "", false
	}
	return stripped, true
}

// splitFlagEq splits a `--name=value` (or `-name=value`) token into its name
// and value parts. ok is false if arg is not a flag or has no `=`.
func splitFlagEq(arg string) (name, value string, ok bool) {
	if !strings.HasPrefix(arg, "-") {
		return "", "", false
	}
	idx := strings.Index(arg, "=")
	if idx < 0 {
		return "", "", false
	}
	stripped := strings.TrimLeft(arg[:idx], "-")
	if stripped == "" {
		return "", "", false
	}
	return stripped, arg[idx+1:], true
}

// dashPrefix returns the leading dashes of arg ("--" or "-"); used to
// preserve the original prefix shape when reconstructing a redacted token.
func dashPrefix(arg string) string {
	n := 0
	for n < len(arg) && arg[n] == '-' {
		n++
	}
	return arg[:n]
}

// isSecretFlag reports whether name (without leading dashes) is in the
// secret-flag list.
func isSecretFlag(name string) bool {
	for _, s := range secretFlags {
		if name == s {
			return true
		}
	}
	return false
}
