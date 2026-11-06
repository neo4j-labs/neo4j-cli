// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
)

// StoreEntry is one store found under the managed store directory.
type StoreEntry struct {
	// Name is the entry's name inside the managed directory (the bare name
	// `query --relate` and the relate tree resolve back to Path).
	Name string
	// Path is the store's full path.
	Path string
	// Kind is "directory" (an unpacked store) or "bundle" (a .rl file).
	Kind string
	// Size is the total byte size of the store's files.
	Size int64
	// Modified is the store's own mtime.
	Modified time.Time
}

// Exists reports whether path exists on fs at all.
func Exists(fs afero.Fs, path string) bool {
	_, err := fs.Stat(path)
	return err == nil
}

// IsStore reports whether path holds a Relate store on fs: a directory
// carrying the engine's manifest.json store marker, or a .rl single-file
// bundle. Transient lock/writer files do not affect the check, so a store
// mid-write still counts.
func IsStore(fs afero.Fs, path string) bool {
	info, err := fs.Stat(path)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return strings.HasSuffix(path, ".rl")
	}
	_, err = fs.Stat(filepath.Join(path, "manifest.json"))
	return err == nil
}

// IsEmptyDir reports whether path is an existing directory with no entries.
func IsEmptyDir(fs afero.Fs, path string) bool {
	entries, err := afero.ReadDir(fs, path)
	return err == nil && len(entries) == 0
}

// ManagedStores lists every store under the managed store directory. A
// missing directory is an empty list, not an error; entries that are not
// stores are skipped.
func ManagedStores(fs afero.Fs) ([]StoreEntry, error) {
	entries, err := afero.ReadDir(fs, StoreDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []StoreEntry{}, nil
		}
		return nil, err
	}

	stores := []StoreEntry{}
	for _, entry := range entries {
		path := filepath.Join(StoreDir(), entry.Name())
		if !IsStore(fs, path) {
			continue
		}
		kind := "directory"
		if !entry.IsDir() {
			kind = "bundle"
		}
		size, _, _ := TreeSize(fs, path)
		stores = append(stores, StoreEntry{
			Name:     entry.Name(),
			Path:     path,
			Kind:     kind,
			Size:     size,
			Modified: entry.ModTime(),
		})
	}
	return stores, nil
}

// TreeSize sums the byte sizes of every file under path (a store directory
// or a single bundle file) and returns the total together with the root's
// mtime. Errors on individual files are skipped — a lock file that vanishes
// mid-walk must not fail the listing.
func TreeSize(fs afero.Fs, path string) (int64, time.Time, error) {
	info, err := fs.Stat(path)
	if err != nil {
		return 0, time.Time{}, err
	}
	if !info.IsDir() {
		return info.Size(), info.ModTime(), nil
	}
	var total int64
	_ = afero.Walk(fs, path, func(_ string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		total += fi.Size()
		return nil
	})
	return total, info.ModTime(), nil
}

// RemoveStore removes the store at path: the whole directory for an unpacked
// store, the file for a .rl bundle. Callers gate it (IsStore + confirmation).
func RemoveStore(fs afero.Fs, path string) error {
	info, err := fs.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fs.RemoveAll(path)
	}
	return fs.Remove(path)
}
