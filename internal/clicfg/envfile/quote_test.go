// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package envfile

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQuote_RoundTripsThroughParse writes each quoted value and reads it back
// with Parse, the reader that `query --env` uses.
func TestQuote_RoundTripsThroughParse(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{name: "plain", value: "s3cr3t"},
		{name: "empty", value: ""},
		{name: "dollar and backslash", value: `pa$$"wo\rd` + "\tkey"},
		{name: "variable reference", value: "$HOME"},
		{name: "apostrophe", value: "it's a password"},
		{name: "apostrophe and variable", value: `it's $HOME`},
		{name: "line endings", value: "first\nsecond\rthird"},
		{name: "hash", value: "before # after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			quoted, err := Quote(tc.value)
			require.NoError(t, err)
			fs := afero.NewMemMapFs()
			require.NoError(t, afero.WriteFile(fs, "/env", []byte("KEY="+quoted+"\n"), 0o600))
			vals, _, err := Parse(fs, "/env")
			require.NoError(t, err)
			assert.Equal(t, tc.value, vals["KEY"])
		})
	}
}

func TestQuote_RejectsValuesParseWouldChange(t *testing.T) {
	for _, value := range []string{`trailing\`, `it's \new`, "line\nand \\r"} {
		t.Run(value, func(t *testing.T) {
			_, err := Quote(value)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), value)
		})
	}
}
