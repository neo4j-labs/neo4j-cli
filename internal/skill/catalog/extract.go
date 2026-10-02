// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalog

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

// MaxDecompressedBytes caps the total decompressed payload size accepted by
// Extract. Defends against a gzip bomb feeding an unbounded tar body. Sized
// per the PRD (20 MiB) — comfortable headroom over the current
// neo4j-contrib/neo4j-skills tarball.
const MaxDecompressedBytes int64 = 20 << 20

// Extract streams a gzip+tar archive from r, strips the single top-level
// archive directory (e.g. `neo4j-skills-<sha>/`), and writes entries whose
// first surviving path segment matches one of allowed into destFs rooted at
// destRoot. Files are written 0600, directories 0755. The total decompressed
// payload is capped at MaxDecompressedBytes; exceeding it aborts mid-stream.
//
// Rejects (with error):
//   - absolute paths, `..` traversal, NUL bytes in entry names;
//   - symlinks/hardlinks (tar.TypeSymlink / tar.TypeLink);
//   - device, fifo, char-special, block-special, anything non-regular other
//     than directory.
//
// Silently skips entries whose first segment is not in allowed (or entries
// that consist solely of the stripped top-level directory).
func Extract(r io.Reader, destFs afero.Fs, destRoot string, allowed []string) error {
	if destFs == nil {
		return errors.New("catalog: nil destination filesystem")
	}
	if destRoot == "" {
		return errors.New("catalog: empty destination root")
	}

	allowSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		if name == "" {
			continue
		}
		allowSet[name] = struct{}{}
	}

	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("catalog: gzip reader: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	var written int64

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("catalog: tar header: %w", err)
		}

		rel, ok, err := classifyEntry(hdr)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		segments := strings.Split(rel, "/")
		if len(segments) == 0 || segments[0] == "" {
			continue
		}
		if _, allowed := allowSet[segments[0]]; !allowed {
			continue
		}

		destPath := filepath.Join(destRoot, filepath.FromSlash(rel))
		// Canonical Zip Slip prefix check — matches the AST shape CodeQL's
		// go/zipslip sanitizer recognizes, and provides defense-in-depth over
		// classifyEntry's per-segment `..` rejection.
		if !strings.HasPrefix(destPath, filepath.Clean(destRoot)+string(os.PathSeparator)) {
			return fmt.Errorf("catalog: tar entry path %q escapes destination root %q", destPath, destRoot)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if mkerr := destFs.MkdirAll(destPath, 0755); mkerr != nil {
				return fmt.Errorf("catalog: mkdir %s: %w", destPath, mkerr)
			}
		case tar.TypeReg:
			if mkerr := destFs.MkdirAll(filepath.Dir(destPath), 0755); mkerr != nil {
				return fmt.Errorf("catalog: mkdir %s: %w", filepath.Dir(destPath), mkerr)
			}
			n, werr := writeCapped(destFs, destPath, tr, MaxDecompressedBytes-written)
			written += n
			if werr != nil {
				return werr
			}
		}
	}

	return nil
}

// classifyEntry validates a tar header and returns the archive-relative
// forward-slash path with the single top-level archive directory stripped.
//
// Returns (rel, true, nil) when the entry should be considered for
// extraction. Returns ("", false, nil) when the entry is the stripped
// top-level directory itself (or otherwise empty after stripping) and
// should be silently skipped. Returns ("", false, err) when the entry is
// hostile or unsupported and the extract must abort.
func classifyEntry(hdr *tar.Header) (string, bool, error) {
	name := hdr.Name
	if name == "" {
		return "", false, errors.New("catalog: empty tar entry name")
	}
	if strings.Contains(name, "\x00") {
		return "", false, fmt.Errorf("catalog: tar entry %q contains NUL", name)
	}
	if path.IsAbs(name) || filepath.IsAbs(name) {
		return "", false, fmt.Errorf("catalog: rejecting absolute tar entry %q", name)
	}

	switch hdr.Typeflag {
	case tar.TypeReg, tar.TypeDir: //nolint:staticcheck
	case tar.TypeXHeader, tar.TypeXGlobalHeader:
		// PAX extended/global headers are metadata containers the Go tar
		// reader merges into the next real entry — never written to disk.
		// GitHub's codeload tarballs always include a leading
		// `pax_global_header` so silently skipping is the correct behaviour.
		return "", false, nil
	case tar.TypeSymlink, tar.TypeLink:
		return "", false, fmt.Errorf("catalog: rejecting link tar entry %q (type %c)", name, hdr.Typeflag)
	default:
		return "", false, fmt.Errorf("catalog: rejecting non-regular tar entry %q (type %c)", name, hdr.Typeflag)
	}

	cleaned := path.Clean(strings.TrimPrefix(name, "./"))
	if cleaned == "." || cleaned == "" {
		return "", false, nil
	}
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".." {
			return "", false, fmt.Errorf("catalog: rejecting traversal in tar entry %q", name)
		}
	}

	parts := strings.SplitN(cleaned, "/", 2)
	if len(parts) < 2 {
		// Top-level archive directory (e.g. `neo4j-skills-<sha>`) with no
		// child path — nothing to extract from this entry.
		return "", false, nil
	}
	rel := parts[1]
	if rel == "" {
		return "", false, nil
	}
	return rel, true, nil
}

// writeCapped streams src into destFs at destPath (mode 0600), enforcing a
// per-file remaining-budget cap. Returns the number of bytes written and
// any error. On error the partial file is removed best-effort.
func writeCapped(destFs afero.Fs, destPath string, src io.Reader, remaining int64) (int64, error) {
	if remaining <= 0 {
		return 0, fmt.Errorf("catalog: extract exceeds %d byte cap", MaxDecompressedBytes)
	}

	f, err := destFs.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return 0, fmt.Errorf("catalog: create %s: %w", destPath, err)
	}

	// LimitReader allows one extra byte so we can distinguish "fits exactly"
	// from "overflows" and abort with a size-limit error.
	n, copyErr := io.Copy(f, io.LimitReader(src, remaining+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = destFs.Remove(destPath)
		return n, fmt.Errorf("catalog: write %s: %w", destPath, copyErr)
	}
	if closeErr != nil {
		_ = destFs.Remove(destPath)
		return n, fmt.Errorf("catalog: close %s: %w", destPath, closeErr)
	}
	if n > remaining {
		_ = destFs.Remove(destPath)
		return n, fmt.Errorf("catalog: extract exceeds %d byte cap", MaxDecompressedBytes)
	}
	return n, nil
}
