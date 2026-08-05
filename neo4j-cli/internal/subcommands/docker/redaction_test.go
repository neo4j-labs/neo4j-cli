// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"encoding/json"
	"testing"

	"github.com/neo4j/cli/common/clievents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertRedactionScrubs asserts both halves of the CLI-228 contract on a
// captured output: raw still carries pw verbatim (registration is additive — the
// rendered row is unchanged) and the redacted copy replaces it with the
// placeholder. Returns the redacted text so callers can add shape-specific
// assertions on it.
func assertRedactionScrubs(t *testing.T, raw, pw string) string {
	t.Helper()

	require.Contains(t, raw, pw,
		"output must still render the generated password verbatim; got: %q", raw)

	redacted := clievents.RedactText(raw)
	assert.NotContains(t, redacted, pw,
		"output must not survive redaction with the generated password intact; redacted: %q", redacted)
	assert.Contains(t, redacted, "***",
		"redacted output must show the placeholder where the password was; redacted: %q", redacted)

	return redacted
}

// TestCreate_GeneratedPassword_ScrubbedFromRedactedCapture — CLI-228, the
// primary pin. For every format `docker create` can render, the generated
// password must be gone after a pass through clievents.RedactText. Note the fix
// is not output-neutral everywhere: it also unified the randSource-failure
// message on `docker: generate password:` (REQ-F-005), which is stderr on a path
// unreachable unless crypto/rand fails.
//
// Scope caveat — this pins a HELPER-LEVEL invariant ("the minted value is
// registered, therefore RedactText can scrub it"), NOT a closed end-to-end leak.
// The test calls RedactText itself; no production path currently feeds
// SUCCESSFUL stdout through it (the tee buffer is persisted only when the
// command fails, and --debug covers stderr plus env NAMES only).
//
// --format is passed explicitly in every subtest because the default resolution
// is environment-dependent (output.ResolveOutput: explicit flag > agent harness
// → toon > TTY → table > json), so an implicit default would silently retarget
// the matrix depending on where the suite runs.
func TestCreate_GeneratedPassword_ScrubbedFromRedactedCapture(t *testing.T) {
	tests := []struct {
		name   string
		format string
	}{
		// table and toon are the two formats that actually leak pre-fix: both
		// put the value on a different line from its header (a box-drawing cell
		// and a TOON array row). toon matters most — it is the agent-harness
		// default.
		{"table", "table"},
		{"toon", "toon"},
		// json passes both BEFORE and AFTER this fix: `"password":"<v>"` is
		// already caught by textJSONFieldRe. It is here so the matrix documents
		// coverage of every renderable format, not because it pins this change.
		{"json", "json"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expectedPassword := stubRandSource(t)

			_, _, stdout, err := runCreate(t, "--name dev --format "+tc.format)
			require.NoError(t, err)

			assertRedactionScrubs(t, stdout, expectedPassword)
		})
	}
}

// TestCreate_Ephemeral_GeneratedPassword_ScrubbedFromRedactedCapture covers the
// --ephemeral `.env` blob, the one `docker create` output path that bypasses the
// table/JSON/TOON renderer entirely.
//
// Honest framing: this test passes BEFORE the fix too. The blob's
// `NEO4J_PASSWORD=<v>` line is caught by RedactText's unanchored
// textAssignmentRe, which matches on the `PASSWORD=` substring, so the assertion
// holds whether or not the value is registered. It is a regression guard for the
// blob path — it would catch a future reshaping that moves the value off its key
// — not a pin of CLI-228. The same helper-level caveat as the format-matrix test
// above applies: it calls RedactText itself rather than exercising a live
// capture surface.
func TestCreate_Ephemeral_GeneratedPassword_ScrubbedFromRedactedCapture(t *testing.T) {
	expectedPassword := stubRandSource(t)

	_, _, _, stdout, _, err := runCreateForEphemeral(t, "--name tmp --ephemeral")
	require.NoError(t, err)

	// Anchored on the key, which assertRedactionScrubs deliberately is not: a
	// blob that emitted the password under some OTHER key would still satisfy
	// both halves of the generic contract.
	require.Contains(t, stdout, "NEO4J_PASSWORD="+expectedPassword,
		"env blob must still carry the generated password on its key; got: %q", stdout)

	redacted := assertRedactionScrubs(t, stdout, expectedPassword)
	assert.Contains(t, redacted, "NEO4J_PASSWORD=***",
		"redacted env blob must keep the key and show the placeholder; redacted: %q", redacted)
}

// TestCreate_ExplicitPassword_NotRegisteredAsSecret — CLI-228 REQ-F-006. Of the
// two DOCKER container passwords, only the GENERATED one is registered with
// clievents.RegisterSecretValue; an operator-supplied --password is deliberately
// left out. This test pins that so a future widening is a conscious choice rather
// than a silent regression. The scope is docker-local, not a CLI-wide rule —
// desktop/dbms/create.go and desktop/connection/create.go DO register their
// supplied passwords; that asymmetry is out of scope here.
//
// Why the exclusion (full rationale on generatePassword in password.go):
// redaction of a registered value is a literal strings.ReplaceAll over the whole
// captured text, and RegisterSecretValue only refuses values under 4 characters.
// A supplied password of `neo4j` would therefore rewrite `neo4j://localhost:7687`,
// `neo4j:enterprise` and `username: neo4j` to *** in every capture, mangling
// exactly the diagnostics tee-on-failure exists to preserve. The trade is
// acceptable because a supplied value is already known to whoever supplied it,
// and clievents.RedactArgs already scrubs it at the argv/history/telemetry level
// (`password` is in secretFlags) — this is only about the free-text pass.
//
// The literal must be collision-proof: knownSecrets is a process-global, additive
// registry with no exported reset, so this negative assertion would break if any
// earlier test in the package registered a SUBSTRING of it. Every registration in
// this test binary comes from generatePassword — no other registering package is
// even linked in — so every registered value is 22 base64url characters. The dots
// below are outside the base64url alphabet, which makes the exclusion structural:
// no substring of the literal can be a generatePassword output at all. `mysecret`
// (TestCreate_ExplicitPassword_HonouredAndSurfaced) is deliberately avoided.
func TestCreate_ExplicitPassword_NotRegisteredAsSecret(t *testing.T) {
	const explicitPassword = "cli228.operator.supplied"

	_, _, stdout, err := runCreate(t, "--name dev --password "+explicitPassword+" --format json")
	require.NoError(t, err)

	// Sanity: the run really did use the supplied password, so the assertion
	// below is about a value that actually reached the render path. Without this,
	// a regression that ignored --password entirely would pass vacuously.
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	require.Equal(t, explicitPassword, rows[0]["password"])

	// The literal is asserted directly rather than through the captured stdout,
	// which is the only way to isolate the registry: `"password":"<v>"` in the JSON
	// capture is legitimately scrubbed by textJSONFieldRe whether or not the value
	// is registered. The bare literal carries no key=value / JSON / URI /
	// auth-header shape, so none of RedactText's regex passes can touch it.
	assert.Equal(t, explicitPassword, clievents.RedactText(explicitPassword),
		"an operator-supplied --password must NOT be registered as a known secret value")
}

// TestLoad_NewContainer_GeneratedPassword_ScrubbedFromRedactedCapture — CLI-228,
// the second-site pin. `docker load`'s new-container path mints its password
// inside LoadDumpIntoNewContainer and renders it in the result row, so it must
// inherit the registration that generatePassword performs.
//
// Scope caveat — like TestCreate_GeneratedPassword_ScrubbedFromRedactedCapture,
// this pins a HELPER-LEVEL invariant ("the minted value is registered, therefore
// RedactText can scrub it"), not a closed end-to-end leak; see that test's comment
// for why no production path feeds successful stdout through RedactText.
//
// --format table is passed explicitly because default resolution is
// environment-dependent (output.ResolveOutput: explicit flag > agent harness →
// toon > TTY → table > json), so an implicit default would silently retarget the
// assertion away from the box-drawing shape this is meant to cover.
//
// Unlike `docker create`, this leaf has no --password, --no-print-password or
// --no-store-credential flags, so there is no supplied-password or suppression
// path to cover here.
func TestLoad_NewContainer_GeneratedPassword_ScrubbedFromRedactedCapture(t *testing.T) {
	expectedPassword := stubRandSource(t)

	fake := newFakeDockerClient() // Inspect default-misses → ErrNotFound → new path
	deps := &loadDeps{resolveSpec: moviesSpec()}

	_, stdout, _, err := runLoad(t, fake, deps, "neo4j-graph-examples/movies --name movies --format table")
	require.NoError(t, err)

	assertRedactionScrubs(t, stdout, expectedPassword)
}
