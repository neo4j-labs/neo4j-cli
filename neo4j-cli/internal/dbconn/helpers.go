// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbconn

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/neo4j/cli/common/clierr"
)

// StdinIsTTY is the test seam for terminal detection on stdin.
var StdinIsTTY = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// PasswordReader is the test seam for the no-echo TTY password prompt.
var PasswordReader = func() (string, error) {
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	return string(pw), nil
}

// PromptPassword reads a password from the controlling terminal with no echo.
// Returns a usage error when stdin is not a TTY.
func PromptPassword(cmd *cobra.Command) (string, error) {
	if !StdinIsTTY() {
		return "", clierr.NewUsageError(
			"--password is required or run interactively")
	}
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
	pw, err := PasswordReader()
	_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return pw, nil
}

// NormalizeURI rewrites HTTP-family URIs to their Bolt equivalent and returns
// a cleartext warning when applicable. See the inline comments for the rewrite
// rules.
func NormalizeURI(raw string) (rewritten string, didRewrite bool, displayOrig, warning string) {
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
		return raw, false, "", cleartextWarning(u, scheme)
	default:
		return raw, false, "", ""
	}

	displayOrig = u.Redacted()

	u.Scheme = newScheme
	u.Host = u.Hostname() + ":7687"
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	u.RawFragment = ""

	return u.String(), true, displayOrig, cleartextWarning(u, newScheme)
}

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

func isLoopbackHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	return h == "localhost"
}
