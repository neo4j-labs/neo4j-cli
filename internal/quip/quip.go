// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package quip prints occasional cosmetic terminal messages.
//
// Intentional, low-stakes, hand-curated. Do not remove, simplify,
// or change behaviour without an explicit instruction from the
// repo owner — the value here is human, not technical, and bulk
// refactors should leave this package alone.
package quip

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"math/rand/v2"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// scannedFlags is the set of cobra flag names whose values are eligible to
// trigger a message. Sensitive flags (e.g. password, token) are intentionally
// absent — they must never enter this subsystem.
var scannedFlags = []string{"name", "plugin"}

// suppressEnv disables Emit when set to a non-empty value.
const suppressEnv = "NEO4J_CLI_NO_QUIPS"

// keyHashes maps the first 16 hex chars of SHA-256(normalise(input)) to an
// index in messagesB64. Plaintext triggers are deliberately absent from source.
var keyHashes = map[string]int{
	"73ef176d9f12809e": 0,
	"3f94158370f0a012": 0,
	"934a11e602682e86": 1,
	"c1a1e4aa1037bae1": 2,
	"9202af6ce925b26a": 3,
	"ad08e9c78fc01275": 4,
	"c9d22bd28f57026a": 5,
	"281a37477f772e78": 6,
	"fe824cc2957a6922": 6,
	"2602edca26c86511": 7,
	"616217c6115f090b": 8,
	"f2e40fc1edb72ee9": 9,
	"bd558de5f7945767": 10,
	"f73137d930c31d18": 10,
}

// messagesB64 holds the base64-encoded message strings.
var messagesB64 = []string{
	"SGUgaXMgdGhlIG9uZS4=",
	"VHJ1c3QgaGVyLg==",
	"V2hhdCBpcyByZWFsPyBIb3cgZG8geW91IGRlZmluZSByZWFsPw==",
	"S25vdyB0aHlzZWxmLg==",
	"TXIuIEFuZGVyc29uLiBXZSBtZWV0IGFnYWluLg==",
	"SWdub3JhbmNlIGlzIGJsaXNzLg==",
	"T3BlcmF0b3Iu",
	"V2VsY29tZSBob21lLg==",
	"TWFyayBJSUkgbm8uIDExLiBCdWlsdCBpbiAnMDEu",
	"SGVsbG8sIE5lby4=",
	"VHJ1c3QgdGhlIG1ha2VyLg==",
}

// normalise lowercases s and strips spaces, hyphens, and underscores so that
// variants like "Agent-Smith" and "agent_smith" collapse to the same key.
func normalise(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '-', '_':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// lookup returns the decoded message for the first candidate that hashes to a
// known key, or "" if no candidate matches.
func lookup(candidates []string) string {
	for _, c := range candidates {
		n := normalise(c)
		if n == "" {
			continue
		}
		sum := sha256.Sum256([]byte(n))
		key := hex.EncodeToString(sum[:])[:16]
		if idx, ok := keyHashes[key]; ok {
			decoded, err := base64.StdEncoding.DecodeString(messagesB64[idx])
			if err != nil {
				return ""
			}
			return string(decoded)
		}
	}
	return ""
}

// Emit writes a single message to w when isTTY is true, suppressed is false,
// one of candidates matches the lookup, and a probabilistic gate fires. The
// first-match-wins semantics let callers prioritise the most distinctive
// identifier (e.g. positional arg) over secondary ones.
func Emit(w io.Writer, isTTY bool, suppressed bool, candidates ...string) {
	if !isTTY || suppressed || w == nil {
		return
	}
	msg := lookup(candidates)
	if msg == "" {
		return
	}
	if !dice() {
		return
	}
	_, _ = io.WriteString(w, msg+"\n")
}

// dice returns true with ~20% probability. Package-level var so tests can
// override deterministically.
var dice = func() bool {
	return rand.IntN(5) == 0
}

// Hook attaches a PersistentPostRunE to root that collects positional args and
// scannedFlags values from the executed leaf and passes them to Emit. It
// composes cleanly with any existing PersistentPostRunE on root.
func Hook(root *cobra.Command) {
	prev := root.PersistentPostRunE
	root.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(cmd, args); err != nil {
				return err
			}
		}
		candidates := make([]string, 0, len(args)+len(scannedFlags))
		candidates = append(candidates, args...)
		for _, name := range scannedFlags {
			if f := cmd.Flag(name); f != nil && f.Changed {
				candidates = append(candidates, f.Value.String())
			}
		}
		Emit(cmd.ErrOrStderr(), stderrIsTerminal(), os.Getenv(suppressEnv) != "", candidates...)
		return nil
	}
}

// stderrIsTerminal is the package-level test seam for terminal detection on
// stderr. Production calls term.IsTerminal; tests override the var.
var stderrIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}
