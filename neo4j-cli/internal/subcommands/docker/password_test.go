// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/neo4j/cli/common/clievents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errRandSource is the sentinel returned by errReader, so the wrapping
// assertion can use errors.Is rather than string matching.
var errRandSource = errors.New("password_test: randSource always fails")

// errReader is an always-failing io.Reader used to drive generatePassword's
// crypto/rand failure branch, which nothing else in the repo exercises.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errRandSource }

func TestGeneratePassword_RegistersSecretValueForRedaction(t *testing.T) {
	origRand := randSource
	randSource = constantReader{b: 0x01}
	defer func() { randSource = origRand }()

	pw, err := generatePassword()
	require.NoError(t, err)

	expected := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, generatedPasswordBytes))
	require.Equal(t, expected, pw)

	assert.Equal(t, "***", clievents.RedactText(pw), "minted password must be registered so RedactText scrubs it")

	// The cell assertion reproduces the exact shape RedactText's regex passes
	// do NOT model — a value in a box-drawing column, on a different line from
	// its header (see the coverage note on clievents.RedactText). It is what
	// makes this a pin of the leak CLI-228 closes rather than of redaction in
	// the abstract, and it keeps holding if a future regex pass is added that
	// happens to satisfy the bare-value assertion above for some other reason.
	cell := "│ " + pw + " │"
	assert.Equal(t, "│ *** │", clievents.RedactText(cell), "password must be scrubbed from a table cell, not just from key=value shapes")
}

func TestGeneratePassword_RandSourceError_Wrapped(t *testing.T) {
	origRand := randSource
	randSource = errReader{}
	defer func() { randSource = origRand }()

	pw, err := generatePassword()
	require.Error(t, err)
	assert.Empty(t, pw)
	assert.ErrorIs(t, err, errRandSource)
	assert.Contains(t, err.Error(), "docker: generate password")
}
