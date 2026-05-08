// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clievents

import "strings"

// secretFlags is the canonical list of flag names whose values must never be
// echoed in telemetry, panic templates, or error output. The list is matched
// against both the long form (--flag) and the defensive single-dash form
// (-flag), and both space-separated (--flag value) and equals-separated
// (--flag=value) shapes.
var secretFlags = []string{
	"password",
	"client-secret",
	"api-key",
	"instance-password",
}

// redactedPlaceholder is what the secret value is replaced with in output.
const redactedPlaceholder = "***"

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
