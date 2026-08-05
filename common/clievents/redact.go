// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clievents

import (
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
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
//
// "header"/"H" is `aura api`'s repeatable `Name: value` header flag. The whole
// value is scrubbed rather than just the header value, because the header NAME
// is as likely to be the secret-bearing part (`-H 'Authorization: Bearer …'`)
// and a name-based heuristic here would fail open.
var secretFlags = []string{
	"password",
	"p",
	"client-secret",
	"api-key",
	"instance-password",
	"set-password",
	"new-password",
	"header",
	"H",
}

// keyValueFlags are the flags whose values are `key=value` pairs, scrubbed
// selectively by redactParamValue: only the value is replaced, and only when
// the key looks secret-bearing, so non-secret pairs stay readable in history.
//
// "F"/"f" are `aura api`'s --field/--raw-field shorthands. "f" is also
// `update --force`'s shorthand, which is harmless: redactParamValue leaves any
// value without an `=` untouched.
var keyValueFlags = []string{
	"param",
	"field",
	"raw-field",
	"F",
	"f",
}

// redactedPlaceholder is what the secret value is replaced with in output.
const redactedPlaceholder = "***"

// secretWords is the single canonical vocabulary of case-insensitive substrings
// that mark a key (a `--param` bind-parameter key, a JSON field name, or a
// key=value LHS) as secret-bearing. Every secret-key heuristic in this file is
// driven from this one slice — secretParamKeyParts for the argv `--param`
// matcher, and the assignment/JSON regexes (built at package init) — so the
// word list lives in exactly one place. The list is intentionally conservative
// to avoid over-redacting common non-secret keys (e.g. `limit`, `name`).
//
// "auth" is deliberately NOT in this list: the assignment regex needs special
// handling for it (its own `auth(?:[-_]?token|[-_]?key)?` branch in
// textAssignmentRe) so it does not swallow the HTTP header name "Authorization",
// and treating "auth" as a generic substring would over-match. It is added back
// in only where that nuance is encoded.
var secretWords = []string{
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

// secretParamKeyParts are the substrings that mark a `key=value` flag's key
// secret. It extends the canonical vocabulary rather than aliasing it: the
// words excluded from secretWords are excluded only because of regex hazards
// (see above), and this matcher is a plain substring test over an argv key, so
// it has no such hazard. `--field`/`--raw-field` carry arbitrary API keys
// rather than a fixed schema, so failing closed on these matters more here.
var secretParamKeyParts = append(slices.Clone(secretWords),
	"auth",
	"credential",
	"passphrase",
)

// quotedAlternation joins the words into a regex alternation, QuoteMeta'ing
// each so metacharacters in any future vocabulary entry stay literal.
func quotedAlternation(words []string) string {
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = regexp.QuoteMeta(w)
	}
	return strings.Join(quoted, "|")
}

// RedactArgs renders an argv slice as a single space-separated string with
// the values of secret-bearing flags replaced by ***. It handles four argv
// shapes:
//
//	--flag value          -> --flag ***
//	--flag=value          -> --flag=***
//	-flag value           -> -flag ***   (defensive single-dash form)
//	-Fvalue               -> -F***      (shorthand with attached value)
//
// A trailing secret flag with no following argument is left as-is — there is
// no value to scrub. Positional arguments and non-secret flags are emitted
// unchanged.
//
// For the `key=value` flags (`--param`, `--field`, `--raw-field`; also the
// single-dash and shorthand spellings, both shapes), a best-effort heuristic
// scrubs only the value when the base key (the part before any `:embed`
// modifier and the first `=`) looks secret-bearing — see secretParamKeyParts.
// Non-secret pairs (e.g. `--param limit=10`, `--field name=my-db`) pass through
// unchanged so the history stays useful. This is a name-based heuristic: a
// secret stored under an innocuous key name (e.g. `--param x=sk-live-...`) is
// NOT caught.
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
				// flag: for uri and key=value flags a flag-looking next token is
				// not a value, so leaving it unconsumed lets the loop redact it (e.g.
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

		// Attached-shorthand form (-Hvalue), the dominant curl/gh idiom for the
		// shorthands of the flags this file guards. It is tried last so an exact
		// name match always wins — otherwise `-password s3cret` would be
		// mis-split into `-p` + `assword`, leaving the real secret unconsumed.
		if name, value, ok := splitAttachedShorthand(arg); ok {
			if red, sensitive := redactByFlag(name, value); sensitive {
				out = append(out, "-"+name+red)
				continue
			}
		}

		out = append(out, arg)
	}
	return strings.Join(out, " ")
}

// knownSecrets holds literal secret VALUES (not flag names) registered at
// runtime that must be scrubbed from captured output regardless of how they are
// formatted. It exists because the regex passes below are shape-based: they
// catch `key=value`, JSON fields, URIs, and auth headers, but cannot catch a
// secret that appears in a layout they don't model — most notably a value cell
// in a horizontal box-drawing table (`aura instance create --format table`
// prints the Aura-generated password in its own column, on a different line
// from the "password" header, offset by dynamic column widths). Registering the
// exact value lets RedactText scrub it by literal match in any format without
// over-redacting unrelated output. See RegisterSecretValue.
var (
	knownSecretsMu sync.RWMutex
	knownSecrets   []string
)

// RegisterSecretValue records a literal secret value so RedactText scrubs every
// later occurrence of it (any format) to ***. Use it for secrets minted at
// runtime that are printed in machine-capturable output whose layout the
// shape-based regexes don't cover (e.g. a generated DB password rendered into a
// table cell). Empty or very short values are ignored to avoid scrubbing common
// substrings. Registration is process-local and additive; it is never persisted.
func RegisterSecretValue(v string) {
	if len(v) < 4 {
		return
	}
	knownSecretsMu.Lock()
	defer knownSecretsMu.Unlock()
	for _, s := range knownSecrets {
		if s == v {
			return
		}
	}
	knownSecrets = append(knownSecrets, v)
}

// redactKnownSecrets replaces every registered literal secret value in s with
// ***. Longest-first so a secret that is a substring of another is handled
// before its container.
func redactKnownSecrets(s string) string {
	knownSecretsMu.RLock()
	values := make([]string, len(knownSecrets))
	copy(values, knownSecrets)
	knownSecretsMu.RUnlock()
	if len(values) == 0 {
		return s
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, v := range values {
		s = strings.ReplaceAll(s, v, redactedPlaceholder)
	}
	return s
}

// textURIUserinfoRe matches a connection URI's userinfo password embedded in
// free text: scheme://user:password@ with the password rewritten to ***. Only
// the password segment (group 1 is scheme://user:) is replaced; host/port/path
// are preserved.
var textURIUserinfoRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://[^\s:/@]+:)[^\s@]+@`)

// secretWordsAlt is the regex alternation of the canonical vocabulary, each
// word QuoteMeta'd so any future entry with regex metacharacters stays literal
// (current words are alnum/dash/underscore — safe — but the quoting fails closed
// against drift).
var secretWordsAlt = quotedAlternation(secretWords)

// textAssignmentRe matches a secret key-value assignment (= or :) in free text
// and rewrites the value to ***. Group 1 captures the key and separator so the
// replacement keeps them verbatim. The alternation is built from the canonical
// secretWords vocabulary (with a trailing `\w*` so `MY_PASSWORD`/`api_key2`
// match) plus the special `auth` branch, which deliberately ends at a
// `[=:]`-bounded word rather than using `auth\w*` so it does NOT swallow the
// HTTP header name "Authorization" (handled by the bearer pass) while still
// matching auth/auth_token/authkey.
var textAssignmentRe = regexp.MustCompile(`(?i)((?:(?:` + secretWordsAlt + `)\w*|auth(?:[-_]?token|[-_]?key)?)\s*[=:]\s*)(\S+)`)

// textJSONFieldRe matches a quoted JSON key containing a secret-bearing
// substring followed by a quoted value, rewriting only the value to "***". It
// exists because textAssignmentRe cannot match the JSON shape: the `"` between
// the key and the `:` breaks its `key[=:]value` form, so `"password":"x"` would
// otherwise leak. The key is matched by substring against the canonical
// vocabulary so `"client_secret"`, `"x-api-key"`, `"current_password"` are all
// covered, and both `"k":"v"` and `"k": "v"` spacings are handled.
var textJSONFieldRe = regexp.MustCompile(`(?i)("[^"]*(?:` + secretWordsAlt + `)[^"]*"\s*:\s*)"[^"]*"`)

// textBearerRe matches an Authorization Bearer token or Basic credential blob
// in free text and rewrites it to ***. Group 1 captures the scheme word
// (Bearer/Basic) so the replacement keeps the scheme verbatim.
var textBearerRe = regexp.MustCompile(`(?i)((?:bearer|basic)\s+)\S+`)

// RedactText scrubs secrets from arbitrary multi-line text, the text-level
// counterpart to RedactArgs (which is argv-only). It applies conservative
// regexes for (a) URI userinfo passwords in free text, (b) quoted-key JSON
// secret fields (`"password":"x"`), (c) password/secret/token/api-key/auth
// key-value assignments, and (d) Bearer/Basic authorization headers, replacing
// each secret value with ***, plus (e) any literal value registered via
// RegisterSecretValue. It is the single source of truth for redacting captured
// command output before it is persisted. Non-secret text (e.g. `limit=10`,
// ordinary prose) is left intact.
//
// Coverage limit: the regex passes are shape-based and fully cover key=value,
// JSON fields, connection URIs, and auth headers. They do NOT model
// table-formatted output (horizontal box-drawing tables put a value in a column
// on a different line from its header, offset by dynamic widths), so a secret
// printed only as a table cell is caught solely by the RegisterSecretValue
// literal-match pass — callers that emit a runtime secret into a table must
// register it (e.g. `aura instance create` registers the generated password).
func RedactText(s string) string {
	s = redactKnownSecrets(s)
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
// order: uri (userinfo) and key=value flags before generic secret flags.
func redactByFlag(name, value string) (string, bool) {
	switch {
	case isURIFlag(name):
		return redactURIUserinfo(value), true
	case isKeyValueFlag(name):
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

// isKeyValueFlag reports whether name (without leading dashes) is a flag whose
// values are `key=value` pairs — see keyValueFlags.
func isKeyValueFlag(name string) bool {
	return slices.Contains(keyValueFlags, name)
}

// redactParamValue scrubs the value of a `key=value` flag token when the
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

// splitAttachedShorthand splits a `-Xvalue` token whose X is a registered
// single-character secret-bearing flag into that shorthand and the attached
// value. ok is false for anything else, including a bare `-X` (no value) and
// any double-dash token.
func splitAttachedShorthand(arg string) (name, value string, ok bool) {
	if len(arg) < 3 || arg[0] != '-' || arg[1] == '-' {
		return "", "", false
	}
	short := arg[1:2]
	if !isSecretFlag(short) && !isKeyValueFlag(short) {
		return "", "", false
	}
	return short, arg[2:], true
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
	return slices.Contains(secretFlags, name)
}
