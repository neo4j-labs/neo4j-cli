// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tarEntry is a synthetic tar header + body used to assemble fixture archives.
type tarEntry struct {
	name     string
	typeflag byte
	body     string
	size     int64 // overrides len(body) when non-zero; used for oversize-claim cases
	linkname string
}

// buildTarGz assembles entries into an in-memory gzipped tar.
func buildTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0o600,
			Typeflag: e.typeflag,
			Linkname: e.linkname,
		}
		if e.typeflag == tar.TypeDir {
			hdr.Mode = 0o755
		}
		hdr.Size = int64(len(e.body))
		if e.size != 0 {
			hdr.Size = e.size
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if e.body != "" {
			_, werr := tw.Write([]byte(e.body))
			require.NoError(t, werr)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

const topDir = "neo4j-skills-abc123/"

func TestExtract_AllowlistedContentLands(t *testing.T) {
	archive := buildTarGz(t, []tarEntry{
		{name: topDir, typeflag: tar.TypeDir},
		{name: topDir + "neo4j-cypher-skill/", typeflag: tar.TypeDir},
		{name: topDir + "neo4j-cypher-skill/SKILL.md", typeflag: tar.TypeReg, body: "# Cypher\n"},
		{name: topDir + "neo4j-cypher-skill/references/api.md", typeflag: tar.TypeReg, body: "ref"},
		{name: topDir + "README.md", typeflag: tar.TypeReg, body: "ignored"},
		{name: topDir + "other-skill/SKILL.md", typeflag: tar.TypeReg, body: "skipped"},
	})

	memFs := afero.NewMemMapFs()
	destRoot := filepath.Join("cache", "content", "1.0.0")

	require.NoError(t, Extract(bytes.NewReader(archive), memFs, destRoot, []string{"neo4j-cypher-skill"}))

	skillMd, err := afero.ReadFile(memFs, filepath.Join(destRoot, "neo4j-cypher-skill", "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Cypher\n", string(skillMd))

	refMd, err := afero.ReadFile(memFs, filepath.Join(destRoot, "neo4j-cypher-skill", "references", "api.md"))
	require.NoError(t, err)
	assert.Equal(t, "ref", string(refMd))

	_, err = memFs.Stat(filepath.Join(destRoot, "README.md"))
	assert.Error(t, err, "non-allowlisted entry must not be written")

	_, err = memFs.Stat(filepath.Join(destRoot, "other-skill", "SKILL.md"))
	assert.Error(t, err, "non-allowlisted skill must not be written")
}

func TestExtract_RejectsTraversalAndAbsolutePaths(t *testing.T) {
	tests := []struct {
		name    string
		entries []tarEntry
	}{
		{
			name: "dot-dot traversal",
			entries: []tarEntry{
				{name: topDir, typeflag: tar.TypeDir},
				{name: topDir + "../../../etc/passwd", typeflag: tar.TypeReg, body: "x"},
			},
		},
		{
			name: "absolute path",
			entries: []tarEntry{
				{name: "/etc/passwd", typeflag: tar.TypeReg, body: "x"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			archive := buildTarGz(t, tc.entries)
			memFs := afero.NewMemMapFs()
			err := Extract(bytes.NewReader(archive), memFs, "dest", []string{"neo4j-cypher-skill", "etc"})
			require.Error(t, err)
		})
	}
}

func TestExtract_RejectsLinksAndDevices(t *testing.T) {
	tests := []struct {
		name     string
		typeflag byte
		linkname string
	}{
		{name: "symlink", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
		{name: "hardlink", typeflag: tar.TypeLink, linkname: "/etc/passwd"},
		{name: "char device", typeflag: tar.TypeChar},
		{name: "block device", typeflag: tar.TypeBlock},
		{name: "fifo", typeflag: tar.TypeFifo},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			archive := buildTarGz(t, []tarEntry{
				{name: topDir, typeflag: tar.TypeDir},
				{name: topDir + "neo4j-cypher-skill/evil", typeflag: tc.typeflag, linkname: tc.linkname},
			})
			memFs := afero.NewMemMapFs()
			err := Extract(bytes.NewReader(archive), memFs, "dest", []string{"neo4j-cypher-skill"})
			require.Error(t, err)
		})
	}
}

func TestExtract_RejectsOversizedPayload(t *testing.T) {
	// Two ~12 MiB regular entries — together they exceed the 20 MiB cap and
	// the second write aborts mid-stream.
	chunk := strings.Repeat("a", 12<<20)
	archive := buildTarGz(t, []tarEntry{
		{name: topDir, typeflag: tar.TypeDir},
		{name: topDir + "neo4j-cypher-skill/first.bin", typeflag: tar.TypeReg, body: chunk},
		{name: topDir + "neo4j-cypher-skill/second.bin", typeflag: tar.TypeReg, body: chunk},
	})

	memFs := afero.NewMemMapFs()
	err := Extract(bytes.NewReader(archive), memFs, "dest", []string{"neo4j-cypher-skill"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cap")
}

func TestExtract_SingleEntryOverCapAborts(t *testing.T) {
	oversized := strings.Repeat("a", int(MaxDecompressedBytes)+1)
	archive := buildTarGz(t, []tarEntry{
		{name: topDir, typeflag: tar.TypeDir},
		{name: topDir + "neo4j-cypher-skill/huge.bin", typeflag: tar.TypeReg, body: oversized},
	})

	memFs := afero.NewMemMapFs()
	err := Extract(bytes.NewReader(archive), memFs, "dest", []string{"neo4j-cypher-skill"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cap")
}

func TestExtract_StripsTopLevelDirectory(t *testing.T) {
	archive := buildTarGz(t, []tarEntry{
		{name: "neo4j-skills-deadbeef/", typeflag: tar.TypeDir},
		{name: "neo4j-skills-deadbeef/neo4j-cypher-skill/SKILL.md", typeflag: tar.TypeReg, body: "skill"},
	})

	memFs := afero.NewMemMapFs()
	require.NoError(t, Extract(bytes.NewReader(archive), memFs, "dest", []string{"neo4j-cypher-skill"}))

	data, err := afero.ReadFile(memFs, filepath.Join("dest", "neo4j-cypher-skill", "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "skill", string(data))
}

func TestExtract_EmptyAllowlistWritesNothing(t *testing.T) {
	archive := buildTarGz(t, []tarEntry{
		{name: topDir, typeflag: tar.TypeDir},
		{name: topDir + "neo4j-cypher-skill/SKILL.md", typeflag: tar.TypeReg, body: "x"},
	})

	memFs := afero.NewMemMapFs()
	require.NoError(t, Extract(bytes.NewReader(archive), memFs, "dest", nil))

	_, err := memFs.Stat(filepath.Join("dest", "neo4j-cypher-skill", "SKILL.md"))
	assert.Error(t, err)
}

func TestExtract_BadGzipErrors(t *testing.T) {
	memFs := afero.NewMemMapFs()
	err := Extract(bytes.NewReader([]byte("not gzip")), memFs, "dest", []string{"x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gzip")
}

func TestExtract_TruncatedTarErrors(t *testing.T) {
	archive := buildTarGz(t, []tarEntry{
		{name: topDir + "neo4j-cypher-skill/SKILL.md", typeflag: tar.TypeReg, body: "abc"},
	})
	// Truncate the tar payload halfway through; gzip still decompresses but
	// tar.Next() errors mid-stream.
	truncated := archive[:len(archive)-32]

	memFs := afero.NewMemMapFs()
	err := Extract(bytes.NewReader(truncated), memFs, "dest", []string{"neo4j-cypher-skill"})
	require.Error(t, err)
}

func TestExtract_RejectsNilFsAndEmptyRoot(t *testing.T) {
	archive := buildTarGz(t, []tarEntry{
		{name: topDir + "neo4j-cypher-skill/SKILL.md", typeflag: tar.TypeReg, body: "x"},
	})
	require.Error(t, Extract(bytes.NewReader(archive), nil, "dest", []string{"neo4j-cypher-skill"}))
	require.Error(t, Extract(bytes.NewReader(archive), afero.NewMemMapFs(), "", []string{"neo4j-cypher-skill"}))
}

// Sanity: stdlib io interface satisfies our reader contract — extract reads
// arbitrary streams, not just bytes.Buffer.
var _ io.Reader = (*bytes.Reader)(nil)

func TestClassifyEntry(t *testing.T) {
	tests := []struct {
		name    string
		hdr     *tar.Header
		wantRel string
		wantOK  bool
		wantErr bool
	}{
		{
			name:    "regular file under top dir",
			hdr:     &tar.Header{Name: "top/skill/SKILL.md", Typeflag: tar.TypeReg},
			wantRel: "skill/SKILL.md",
			wantOK:  true,
		},
		{
			name:   "top-level dir itself stripped",
			hdr:    &tar.Header{Name: "top/", Typeflag: tar.TypeDir},
			wantOK: false,
		},
		{
			name:    "regular file directly under top",
			hdr:     &tar.Header{Name: "top/README.md", Typeflag: tar.TypeReg},
			wantRel: "README.md",
			wantOK:  true,
		},
		{name: "empty name", hdr: &tar.Header{Name: "", Typeflag: tar.TypeReg}, wantErr: true},
		{name: "NUL in name", hdr: &tar.Header{Name: "top/foo\x00bar", Typeflag: tar.TypeReg}, wantErr: true},
		{name: "absolute", hdr: &tar.Header{Name: "/etc/passwd", Typeflag: tar.TypeReg}, wantErr: true},
		{name: "traversal", hdr: &tar.Header{Name: "top/../../etc/passwd", Typeflag: tar.TypeReg}, wantErr: true},
		{name: "symlink", hdr: &tar.Header{Name: "top/x", Typeflag: tar.TypeSymlink}, wantErr: true},
		{name: "hardlink", hdr: &tar.Header{Name: "top/x", Typeflag: tar.TypeLink}, wantErr: true},
		{name: "char device", hdr: &tar.Header{Name: "top/x", Typeflag: tar.TypeChar}, wantErr: true},
		{name: "fifo", hdr: &tar.Header{Name: "top/x", Typeflag: tar.TypeFifo}, wantErr: true},
		{name: "pax global header skipped", hdr: &tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader}, wantOK: false},
		{name: "pax extended header skipped", hdr: &tar.Header{Name: "top/skill/SKILL.md", Typeflag: tar.TypeXHeader}, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rel, ok, err := classifyEntry(tc.hdr)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantRel, rel)
		})
	}
}
