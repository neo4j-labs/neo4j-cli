// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/neo4j/cli/common/clievents"
)

// randSource is the injectable seam for crypto-grade random bytes used to
// generate a default Neo4j password. Tests swap in a deterministic reader so the
// rendered password is assertable.
var randSource io.Reader = rand.Reader

// generatedPasswordBytes is the byte length consumed from randSource before
// base64-URL-safe encoding without padding. 16 bytes → 22 base64 characters,
// which is well above the entropy floor for a local Bolt password.
const generatedPasswordBytes = 16

// generatePassword is the SINGLE place a Neo4j password is minted for a local
// container: `docker create` (no --password) and LoadDumpIntoNewContainer
// (`docker load`, and `aura instance load`, which stages through an ephemeral
// container) both route through it. The funnel exists so the registration below
// cannot be forgotten — a new leaf minting its own password would silently
// reintroduce CLI-228.
//
// Registering the literal value matters because clievents.RedactText is
// shape-based: it catches `NEO4J_PASSWORD=<v>` (the --ephemeral .env blob) and
// `"password":"<v>"` (--format json), but NOT a value cell in a box-drawing
// table (--format table) nor a TOON array row (--format toon, the agent-harness
// default), both of which put the value on a different line from its header.
//
// No current path feeds SUCCESSFUL stdout through RedactText (the tee buffer is
// persisted only on failure), so this is prep for consumers that do — notably
// CLI-218's `mcp serve`, whose default format is toon.
//
// In the docker leaves only GENERATED passwords are registered — an
// operator-supplied --password is deliberately left unregistered: redaction of a
// known value is a literal strings.ReplaceAll, so a short value like "neo4j"
// would rewrite `neo4j://localhost:7687`, `neo4j:enterprise` and
// `username: neo4j` to *** in every capture. Argv-level cover for --password
// already exists via clievents.RedactArgs. This is deliberately asymmetric with
// the desktop leaves (desktop/dbms/create.go, desktop/connection/create.go and
// update.go), which DO register operator-supplied passwords.
func generatePassword() (string, error) {
	buf := make([]byte, generatedPasswordBytes)
	if _, err := io.ReadFull(randSource, buf); err != nil {
		return "", fmt.Errorf("docker: generate password: %w", err)
	}
	pw := base64.RawURLEncoding.EncodeToString(buf)
	clievents.RegisterSecretValue(pw)
	return pw, nil
}
