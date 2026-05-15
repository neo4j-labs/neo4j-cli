// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package fileutils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

/* Reads a file, if it doesn't exist returns empty []byte */
func ReadFileSafe(fs afero.Fs, path string) []byte {
	exists := FileExists(fs, path)

	if exists {
		data, err := afero.ReadFile(fs, path)
		if err != nil {
			panic(err)
		}
		return data
	} else {
		return []byte{}
	}
}

func ReadOrCreateFile(fs afero.Fs, path string) []byte {
	exists := FileExists(fs, path)

	if exists {
		data, err := afero.ReadFile(fs, path)
		if err != nil {
			panic(err)
		}
		return data
	} else {
		createFile(fs, path)
		return []byte{}
	}
}

// WriteFile atomically writes data to path with mode 0600.
//
// The bytes are first written to <path>.tmp (created O_CREATE|O_WRONLY|O_TRUNC,
// mode 0600), Sync'd, Close'd, then Rename'd over path. On any failure the
// temp file is removed so a partially-written sibling is never left around,
// and the original path (if any) stays intact.
//
// On POSIX rename(2) is atomic; on Windows MoveFileEx with MOVEFILE_REPLACE_EXISTING
// (which Go's os.Rename uses) is atomic for files on the same volume.
func WriteFile(fs afero.Fs, path string, data []byte) {
	if err := writeFileAtomic(fs, path, data); err != nil {
		panic(err)
	}
}

func writeFileAtomic(fs afero.Fs, path string, data []byte) error {
	if err := fs.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	// Acquire a cross-process advisory lock for the duration of the
	// open/write/sync/close/rename sequence so parallel writers don't
	// interleave their temp files or clobber each other's rename.
	release, lockErr := acquireLock(path)
	if lockErr != nil {
		return lockErr
	}
	defer release()

	tmpPath := path + ".tmp"

	f, err := fs.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	cleanup := func() {
		_ = fs.Remove(tmpPath)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}

	// Sync best-effort: some filesystems (and afero.MemMapFs) may not implement it.
	if err := f.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) {
		_ = f.Close()
		cleanup()
		return fmt.Errorf("sync %s: %w", tmpPath, err)
	}

	if err := f.Close(); err != nil {
		cleanup()
		return err
	}

	if err := fs.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}

	return nil
}

func FileExists(fs afero.Fs, path string) bool {
	if _, err := fs.Stat(path); err == nil {
		return true
	} else if errors.Is(err, os.ErrNotExist) {
		return false
	} else {
		panic(err)
	}
}

func createFile(fs afero.Fs, path string) {
	if err := fs.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		panic(err)
	}

	// Single OpenFile call with mode 0600 closes the umask race window
	// that the prior Create -> Chmod chain left open.
	f, err := fs.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
}
