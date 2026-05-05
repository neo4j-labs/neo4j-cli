// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// CopyBundle copies every regular file in `bundle` onto `dst` rooted at
// `dstDir`. The bundle's directory structure is preserved (e.g.
// SKILL.md -> dstDir/SKILL.md, references/foo.md -> dstDir/references/foo.md).
//
// Callers that have an embed.FS rooted above the bundle directory should
// scope it first with fs.Sub (e.g. fs.Sub(Bundle, "bundle")). Embed.FS
// paths use forward slashes; destination paths are built with
// filepath.Join so they're correct on Windows.
//
// Existing files at the destination are overwritten. Parent directories
// are created with mode 0755 (files mode 0600), matching
// common/clicfg/fileutils.WriteFile semantics.
func CopyBundle(dst afero.Fs, dstDir string, bundle fs.FS) error {
	if bundle == nil {
		return errors.New("skill: nil bundle FS")
	}
	if dstDir == "" {
		return errors.New("skill: empty destination dir")
	}

	return fs.WalkDir(bundle, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Embed.FS guarantees forward slashes; translate to OS separators
		// for the destination so Windows paths are correct.
		destPath := filepath.Join(dstDir, filepath.FromSlash(p))

		data, rerr := fs.ReadFile(bundle, p)
		if rerr != nil {
			return rerr
		}

		if mkerr := dst.MkdirAll(filepath.Dir(destPath), 0755); mkerr != nil {
			return mkerr
		}
		if werr := afero.WriteFile(dst, destPath, data, 0600); werr != nil {
			return werr
		}
		return nil
	})
}

// RemoveDir recursively removes `dir` from `fs`. Idempotent: returns nil
// when the directory does not exist.
func RemoveDir(filesystem afero.Fs, dir string) error {
	if dir == "" {
		return errors.New("skill: empty directory path")
	}
	err := filesystem.RemoveAll(dir)
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
