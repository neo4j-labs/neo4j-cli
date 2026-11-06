// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
)

func TestResolveStorePath(t *testing.T) {
	t.Run("bare name resolves under the managed store dir", func(t *testing.T) {
		assert.Equal(t,
			filepath.Join(StoreDir(), "movies"),
			ResolveStorePath("movies"))
	})

	t.Run("absolute path is taken verbatim", func(t *testing.T) {
		assert.Equal(t, "/tmp/store.rl", ResolveStorePath("/tmp/store.rl"))
	})

	t.Run("explicit relative path is taken verbatim", func(t *testing.T) {
		assert.Equal(t, "./movies", ResolveStorePath("./movies"))
		assert.Equal(t, "../store", ResolveStorePath("../store"))
	})

	t.Run("path with a separator is taken verbatim", func(t *testing.T) {
		assert.Equal(t, "some/dir/store", ResolveStorePath("some/dir/store"))
	})

	t.Run("dot forms are taken verbatim", func(t *testing.T) {
		assert.Equal(t, ".", ResolveStorePath("."))
		assert.Equal(t, "..", ResolveStorePath(".."))
	})

	t.Run("empty argument stays empty", func(t *testing.T) {
		assert.Equal(t, "", ResolveStorePath(""))
	})
}

func TestNotCompiledError(t *testing.T) {
	err := NotCompiledError("--relate")

	// Typed as a usage error (exit code 2) so an untagged build refuses
	// cleanly instead of failing on a nil hook.
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "--relate")
	assert.Contains(t, err.Error(), "not compiled into this build")
	assert.Contains(t, err.Error(), "-tags relate")
}

func TestEngineErrorMessage(t *testing.T) {
	// The boundary error renders the engine's message, not a reparse of it.
	e := &EngineError{Code: "Neo.ClientError.Statement.SyntaxError", Message: "bad"}
	assert.Equal(t, "bad", e.Error())
	assert.False(t, strings.Contains(e.Error(), "Neo."))
}
