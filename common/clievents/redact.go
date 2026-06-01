// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clievents

import (
	"net/url"
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

		// --uri=value form: a Bolt URI can embed credentials as
		// user:password@host userinfo, so strip only the password while
		// keeping scheme/host/port/path (which are useful and non-secret).
		if name, value, ok := splitFlagEq(arg); ok && isURIFlag(name) {
			out = append(out, dashPrefix(arg)+name+"="+redactURIUserinfo(value))
			continue
		}

		// --uri value form.
		if name, ok := flagName(arg); ok && isURIFlag(name) {
			out = append(out, arg)
			if i+1 < len(args) {
				out = append(out, redactURIUserinfo(args[i+1]))
				i++
			}
			continue
		}

		// --param=key=value form (equals-on-flag, defensive).
		if name, value, ok := splitFlagEq(arg); ok && isParamFlag(name) {
			out = append(out, dashPrefix(arg)+name+"="+redactParamValue(value))
			continue
		}

		// --param key=value form (the common space-separated shape).
		if name, ok := flagName(arg); ok && isParamFlag(name) {
			out = append(out, arg)
			if i+1 < len(args) {
				out = append(out, redactParamValue(args[i+1]))
				i++
			}
			continue
		}

		// --flag=value or -flag=value form.
		if name, _, ok := splitFlagEq(arg); ok && isSecretFlag(name) {
			out = append(out, dashPrefix(arg)+name+"="+redactedPlaceholder)
			continue
		}

		// --flag value or -flag value form.
		if name, ok := flagName(arg); ok && isSecretFlag(name) {
			out = append(out, arg)
			if i+1 < len(args) {
				out = append(out, redactedPlaceholder)
				i++
			}
			continue
		}

		out = append(out, arg)
	}
	return strings.Join(out, " ")
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
