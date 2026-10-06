// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"crypto/sha256"
	"encoding/base64"
	engine "github.com/neo4j/cli/internal/docker"
	"io"
	"testing"
)

// repeatingReader is a deterministic io.Reader that cycles a fixed byte slice,
// used to seed the password-byte generation seam (the password entropy seam).
type repeatingReader struct {
	buf []byte
}

func (r repeatingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.buf[i%len(r.buf)]
	}
	return len(p), nil
}

// stubRandSource installs a deterministic the password entropy seam for the duration of t and
// returns the exact password generatePassword will mint while it is installed.
//
// The bytes are derived from t.Name() — which the testing package makes unique
// per test AND subtest — because per-test password uniqueness is load-bearing,
// not cosmetic: clievents' knownSecrets registry is process-global, additive and
// has no exported reset, so two tests minting the same literal would let one
// test's registration silently satisfy the other's assertion about redaction.
// Name-keyed derivation means no human has to track which values are taken.
// Every value derived here is exactly 22 base64url characters, so none can be a
// substring of another either: redactKnownSecrets is a literal strings.ReplaceAll,
// so a registered value contained in another test's asserted literal would
// rewrite part of that literal.
func stubRandSource(t *testing.T) string {
	t.Helper()

	sum := sha256.Sum256([]byte(t.Name()))
	buf := sum[:engine.GeneratedPasswordBytes]
	setRandSource(t, repeatingReader{buf: buf})

	return base64.RawURLEncoding.EncodeToString(buf)
}

// setRandSource installs r as the password-byte seam for the duration of t.
// the password entropy seam is package-global, so callers must not t.Parallel().
func setRandSource(t *testing.T, r io.Reader) {
	t.Helper()

	engine.SetRandSourceForTest(t, r)
}
