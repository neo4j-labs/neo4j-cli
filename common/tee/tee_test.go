// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package tee

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestConfig(t *testing.T, config string) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetTestFs(config, "{}")
	require.NoError(t, err)
	return clicfg.NewConfig(fs, "test-version", clicfg.GlobalScope)
}

func TestLimitedBuffer_BelowCap(t *testing.T) {
	var b LimitedBuffer
	n, err := b.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, []byte("hello"), b.Bytes())
}

func TestLimitedBuffer_CapsAndAppendsFooter(t *testing.T) {
	var b LimitedBuffer
	over := maxCaptureBytes + 100
	n, err := b.Write(bytes.Repeat([]byte("x"), over))
	require.NoError(t, err)
	assert.Equal(t, over, n, "Write always reports the full length")

	out := b.Bytes()
	head := out[:maxCaptureBytes]
	assert.Equal(t, bytes.Repeat([]byte("x"), maxCaptureBytes), head, "keeps the head")
	footer := string(out[maxCaptureBytes:])
	assert.Equal(t, fmt.Sprintf("\n[output truncated: exceeded %d bytes]\n", over), footer)
}

func TestLimitedBuffer_ExactlyAtCapNoFooter(t *testing.T) {
	var b LimitedBuffer
	_, err := b.Write(bytes.Repeat([]byte("x"), maxCaptureBytes))
	require.NoError(t, err)
	assert.Equal(t, maxCaptureBytes, len(b.Bytes()), "no footer at exactly the cap")
}

func TestLimitedBuffer_MultipleWritesOverflow(t *testing.T) {
	var b LimitedBuffer
	_, _ = b.Write(bytes.Repeat([]byte("a"), maxCaptureBytes-1))
	_, _ = b.Write([]byte("bb"))
	out := b.Bytes()
	assert.Equal(t, byte('b'), out[maxCaptureBytes-1], "boundary byte kept")
	assert.Contains(t, string(out), "[output truncated: exceeded")
}

func TestDir(t *testing.T) {
	assert.Equal(t, filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "tee"), Dir())
}

func TestSave_WritesRedactedFile(t *testing.T) {
	cfg := newTestConfig(t, `{"format":"json"}`)
	content := []byte("connecting to neo4j://neo4j:hunter2@host\npassword=topsecret\nok")

	path, err := Save(cfg, "query", content)
	require.NoError(t, err)
	require.NotEmpty(t, path)

	assert.Equal(t, Dir(), filepath.Dir(path))
	assert.True(t, strings.HasSuffix(path, "_query.log"))

	data, err := afero.ReadFile(cfg.Aura.Fs(), path)
	require.NoError(t, err)
	got := string(data)
	assert.Contains(t, got, "neo4j://neo4j:***@host")
	assert.Contains(t, got, "password=***")
	assert.NotContains(t, got, "hunter2")
	assert.NotContains(t, got, "topsecret")
	assert.Contains(t, got, "ok")
}

func TestSave_FileMode0600(t *testing.T) {
	cfg := newTestConfig(t, `{"format":"json"}`)
	path, err := Save(cfg, "query", []byte("x"))
	require.NoError(t, err)
	info, err := cfg.Aura.Fs().Stat(path)
	require.NoError(t, err)
	assert.Equal(t, "-rw-------", info.Mode().String())
}

func TestSave_ShortCircuits(t *testing.T) {
	tests := []struct {
		name    string
		cfg     func(t *testing.T) *clicfg.Config
		content []byte
	}{
		{
			name:    "tee disabled",
			cfg:     func(t *testing.T) *clicfg.Config { return newTestConfig(t, `{"format":"json","tee-enabled":false}`) },
			content: []byte("data"),
		},
		{
			name:    "tee-limit zero",
			cfg:     func(t *testing.T) *clicfg.Config { return newTestConfig(t, `{"format":"json","tee-limit":0}`) },
			content: []byte("data"),
		},
		{
			name:    "empty content",
			cfg:     func(t *testing.T) *clicfg.Config { return newTestConfig(t, `{"format":"json"}`) },
			content: []byte{},
		},
		{
			name:    "nil cfg",
			cfg:     func(t *testing.T) *clicfg.Config { return nil },
			content: []byte("data"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg(t)
			path, err := Save(cfg, "query", tc.content)
			require.NoError(t, err)
			assert.Empty(t, path)
		})
	}
}

func TestSave_RotationPerSlug(t *testing.T) {
	cfg := newTestConfig(t, `{"format":"json","tee-limit":3}`)
	fs := cfg.Aura.Fs()

	// Seed 4 older query files plus 2 other-slug files that must be untouched.
	seed := []string{
		"2026-01-01T00-00-01Z_query.log",
		"2026-01-01T00-00-02Z_query.log",
		"2026-01-01T00-00-03Z_query.log",
		"2026-01-01T00-00-04Z_query.log",
		"2026-01-01T00-00-01Z_instance-list.log",
		"2026-01-01T00-00-02Z_instance-list.log",
	}
	for _, n := range seed {
		require.NoError(t, afero.WriteFile(fs, filepath.Join(Dir(), n), []byte("old"), 0o600))
	}

	path, err := Save(cfg, "query", []byte("new"))
	require.NoError(t, err)
	require.NotEmpty(t, path)

	entries, err := afero.ReadDir(fs, Dir())
	require.NoError(t, err)
	var query, other int
	for _, e := range entries {
		switch {
		case strings.HasSuffix(e.Name(), "_query.log"):
			query++
		case strings.HasSuffix(e.Name(), "_instance-list.log"):
			other++
		}
	}
	assert.Equal(t, 3, query, "keeps at most tee-limit query files (incl. the new one)")
	assert.Equal(t, 2, other, "other slug untouched")

	// Oldest two query files were the deletion targets.
	_, err = fs.Stat(filepath.Join(Dir(), "2026-01-01T00-00-01Z_query.log"))
	assert.Error(t, err)
	_, err = fs.Stat(filepath.Join(Dir(), "2026-01-01T00-00-04Z_query.log"))
	assert.NoError(t, err, "newest seeded file retained")
}
