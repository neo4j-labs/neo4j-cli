// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/query/embed"
)

// TestQueryEmbed_PositionalArg verifies the happy path with a positional
// argument: the provider is called with the supplied text and the resulting
// vector is rendered as a raw JSON array under --format json.
func TestQueryEmbed_PositionalArg(t *testing.T) {
	stub := &stubEmbedProvider{}
	restore := embed.WithFactory(func(_ embed.Config) (embed.Provider, error) {
		return stub, nil
	})
	t.Cleanup(restore)
	t.Setenv("NEO4J_EMBED_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "k")

	h := newRunHarness(t, "json")
	err := h.execute(t, ":embed", "hello")
	require.NoError(t, err)
	assert.Equal(t, 1, stub.calls)
	assert.Equal(t, []string{"hello"}, stub.inputs)

	var got []float32
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, []float32{1, 2, 3}, got)
}

// TestQueryEmbed_StdinInputWhenNoArg verifies piped stdin is consumed when
// no positional argument is supplied. The vector output must match the
// positional-arg case byte-for-byte (modulo trailing newline).
func TestQueryEmbed_StdinInputWhenNoArg(t *testing.T) {
	stub := &stubEmbedProvider{}
	restore := embed.WithFactory(func(_ embed.Config) (embed.Provider, error) {
		return stub, nil
	})
	t.Cleanup(restore)
	t.Setenv("NEO4J_EMBED_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "k")

	h := newRunHarness(t, "json")
	stdinIsTTY = func() bool { return false }
	stdinReader = func() io.Reader { return strings.NewReader("hello\n") }

	err := h.execute(t, ":embed")
	require.NoError(t, err)
	assert.Equal(t, []string{"hello"}, stub.inputs, "trailing whitespace must be trimmed")

	var got []float32
	require.NoError(t, json.Unmarshal(h.stdout.Bytes(), &got))
	assert.Equal(t, []float32{1, 2, 3}, got)
}

// TestQueryEmbed_TTYStdinNoArgReturnsUsageError verifies that a TTY stdin
// with no positional arg surfaces a usage error referencing the "text"
// label, mirroring the parent command's "no Cypher" error path.
func TestQueryEmbed_TTYStdinNoArgReturnsUsageError(t *testing.T) {
	h := newRunHarness(t, "table")
	// stdinIsTTY default is true via harness.

	err := h.execute(t, ":embed")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no text")
}

// TestQueryEmbed_TableOutput renders the vector in table mode. The cell
// content is the compact JSON form of the vector and the header is
// "embedding". The render uses PrintBodyMap so the header text is upper-cased
// by go-pretty's default style.
func TestQueryEmbed_TableOutput(t *testing.T) {
	restore := embed.WithFactory(func(_ embed.Config) (embed.Provider, error) {
		return &stubEmbedProvider{}, nil
	})
	t.Cleanup(restore)
	t.Setenv("NEO4J_EMBED_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "k")

	h := newRunHarness(t, "table")
	err := h.execute(t, ":embed", "hi")
	require.NoError(t, err)

	out := h.stdout.String()
	assert.Contains(t, strings.ToLower(out), "embedding", "table header must contain 'embedding'")
	assert.Contains(t, out, "[1,2,3]", "table cell must show the compact JSON vector")
}

// TestQueryEmbed_ToonOutput verifies --format toon yields a TOON document
// that does NOT parse as JSON (toon uses key:value, not JSON syntax).
func TestQueryEmbed_ToonOutput(t *testing.T) {
	restore := embed.WithFactory(func(_ embed.Config) (embed.Provider, error) {
		return &stubEmbedProvider{}, nil
	})
	t.Cleanup(restore)
	t.Setenv("NEO4J_EMBED_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "k")

	h := newRunHarness(t, "toon")
	err := h.execute(t, ":embed", "hi")
	require.NoError(t, err)

	out := h.stdout.String()
	assert.NotEmpty(t, out)
	var v any
	err = json.Unmarshal([]byte(out), &v)
	assert.Error(t, err, "toon output must not parse as JSON, got: %s", out)
}

// TestQueryEmbed_ProviderErrorPropagates verifies a provider error surfaces
// as an error from the cobra command with the documented `query: embed:`
// prefix. The driverOpener is swapped to panic so we additionally prove no
// Bolt connection is attempted (the password prompt path would also panic
// because the harness defaults to TTY without a password reader).
func TestQueryEmbed_ProviderErrorPropagates(t *testing.T) {
	stub := &stubEmbedProvider{failOn: "boom-input"}
	restore := embed.WithFactory(func(_ embed.Config) (embed.Provider, error) {
		return stub, nil
	})
	t.Cleanup(restore)
	t.Setenv("NEO4J_EMBED_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "k")

	origOpener := driverOpener
	t.Cleanup(func() { driverOpener = origOpener })
	driverOpener = func(_, _, _, _ string, _ bool) (neo4j.Driver, error) {
		panic("driverOpener must not be called from `:embed`")
	}

	h := newRunHarness(t, "json")
	err := h.execute(t, ":embed", "boom-input")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embed")
}

// TestQueryEmbed_NoBoltConnection_NoPasswordPrompt locks REQ-F-029: the
// `:embed` leaf must NOT open a Bolt driver and MUST NOT trigger the password
// prompt even when no --password / NEO4J_PASSWORD is set. Both paths are
// proven by panicking seams: a panicking driverOpener fails any neo4j.NewDriver
// call, and a passwordReader that calls t.Fatal fails the test if invoked.
func TestQueryEmbed_NoBoltConnection_NoPasswordPrompt(t *testing.T) {
	restore := embed.WithFactory(func(_ embed.Config) (embed.Provider, error) {
		return &stubEmbedProvider{}, nil
	})
	t.Cleanup(restore)
	t.Setenv("NEO4J_EMBED_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "k")
	t.Setenv(dbconn.EnvPassword, "")

	origOpener := driverOpener
	t.Cleanup(func() { driverOpener = origOpener })
	driverOpener = func(_, _, _, _ string, _ bool) (neo4j.Driver, error) {
		panic("driverOpener must not be called from `:embed`")
	}

	origRunFn := runStatementResponseFn
	t.Cleanup(func() { runStatementResponseFn = origRunFn })
	runStatementResponseFn = func(_ context.Context, _ *conn, statement string, _ map[string]any, _ bool) (*queryResponse, error) {
		t.Fatalf("statement seam must not be called from `:embed`: got %q", statement)
		return nil, nil
	}

	h := newRunHarness(t, "json")
	passwordReader = func() (string, error) {
		t.Fatal("passwordReader must NOT be invoked from `:embed`")
		return "", nil
	}

	err := h.execute(t, ":embed", "hello")
	require.NoError(t, err)
	assert.NotContains(t, h.stderr.String(), "Password:")
}

// TestQueryEmbed_FactoryErrorPropagates verifies an error returned from the
// provider factory (e.g. validation in embed.New for a missing/invalid
// provider) surfaces verbatim from the cobra command.
func TestQueryEmbed_FactoryErrorPropagates(t *testing.T) {
	restore := embed.WithFactory(func(_ embed.Config) (embed.Provider, error) {
		return nil, errors.New("missing embed provider: bogus")
	})
	t.Cleanup(restore)

	h := newRunHarness(t, "json")
	err := h.execute(t, ":embed", "hi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing embed provider")
}

// TestEmbedVector_MarshalJSON verifies the JSON shape locks to a raw float
// array (not an object envelope), and an empty/nil vector marshals to `[]`.
func TestEmbedVector_MarshalJSON(t *testing.T) {
	t.Run("populated", func(t *testing.T) {
		b, err := json.Marshal(embedVector{1.5, 2.5, 3.5})
		require.NoError(t, err)
		assert.Equal(t, "[1.5,2.5,3.5]", string(b))
	})
	t.Run("nil", func(t *testing.T) {
		var v embedVector
		b, err := json.Marshal(v)
		require.NoError(t, err)
		assert.Equal(t, "[]", string(b))
	})
}

// TestEmbedVector_AsArray verifies the table-render shape: a single row with
// an "embedding" key mapped to the compact JSON form of the vector.
func TestEmbedVector_AsArray(t *testing.T) {
	rows := embedVector{1, 2, 3}.AsArray()
	require.Len(t, rows, 1)
	assert.Equal(t, "[1,2,3]", rows[0]["embedding"])
}
