// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package fileutils

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/spf13/afero"
)

// renameFailFs wraps an afero.Fs and returns a fixed error from Rename.
type renameFailFs struct {
	afero.Fs
	err error
}

func (r *renameFailFs) Rename(oldname, newname string) error {
	return r.err
}

func TestWriteFile_Atomic_HappyPath(t *testing.T) {
	fs := afero.NewMemMapFs()
	path := "/tmp/cli/credentials.json"
	data := []byte(`{"k":"v"}`)

	WriteFile(fs, path, data)

	got, err := afero.ReadFile(fs, path)
	if err != nil {
		t.Fatalf("read after write: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("data mismatch: got %q want %q", got, data)
	}

	// tmp sibling must be gone after a successful rename.
	if exists, _ := afero.Exists(fs, path+".tmp"); exists {
		t.Fatalf(".tmp sibling not cleaned up after successful write")
	}
}

func TestWriteFile_Atomic_RenameFailure_OriginalIntactTmpCleaned(t *testing.T) {
	mem := afero.NewMemMapFs()
	path := "/tmp/cli/credentials.json"

	// Seed an existing original file.
	original := []byte(`{"old":true}`)
	if err := afero.WriteFile(mem, path, original, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	wantErr := errors.New("simulated rename failure")
	fs := &renameFailFs{Fs: mem, err: wantErr}

	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("expected panic on rename failure, got none")
			}
			err, ok := r.(error)
			if !ok {
				t.Fatalf("panic value is not error: %v", r)
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("panic does not wrap simulated rename failure: %v", err)
			}
		}()
		WriteFile(fs, path, []byte(`{"new":true}`))
	}()

	// Original must be intact.
	got, err := afero.ReadFile(mem, path)
	if err != nil {
		t.Fatalf("original file unreadable after failed rename: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("original file changed after failed rename: got %q want %q", got, original)
	}

	// tmp must be cleaned up.
	exists, _ := afero.Exists(mem, path+".tmp")
	if exists {
		t.Fatalf(".tmp sibling not cleaned up after failed rename")
	}
}

func TestWriteFile_OverwritesExisting(t *testing.T) {
	fs := afero.NewMemMapFs()
	path := "/tmp/cli/credentials.json"

	WriteFile(fs, path, []byte(`{"v":1}`))
	WriteFile(fs, path, []byte(`{"v":2}`))

	got, err := afero.ReadFile(fs, path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != `{"v":2}` {
		t.Fatalf("expected overwrite to v=2, got %q", got)
	}
}

func TestCreateFile_SingleOpenFile_NoChmodChain(t *testing.T) {
	fs := afero.NewMemMapFs()
	path := "/tmp/cli/config.json"

	createFile(fs, path)

	if exists, _ := afero.Exists(fs, path); !exists {
		t.Fatalf("createFile did not produce a file at %q", path)
	}

	info, err := fs.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("expected empty file, got size=%d", info.Size())
	}
}

// TestCreateFile_DirMode_OnOsFs verifies the parent directory is created with mode
// 0o700 (umask-permitting) on a real OS fs. Skipped on Windows where the POSIX
// permission bits don't apply the same way.
func TestCreateFile_DirMode_OnOsFs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits not applicable on Windows")
	}

	// Tighten umask so 0o700 isn't masked further than expected.
	old := umaskZero(t)
	defer old()

	tmp := t.TempDir()
	fs := afero.NewOsFs()
	dir := filepath.Join(tmp, "neo4j", "cli")
	path := filepath.Join(dir, "credentials.json")

	createFile(fs, path)

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("dir mode = %#o, want 0700", got)
	}
}

// TestWriteFile_ConcurrentWrites verifies that two goroutines calling WriteFile
// on the same real-OS path concurrently do not panic and leave a valid JSON file.
// A real OsFs is required because acquireLock uses os.OpenFile for the lock file.
func TestWriteFile_ConcurrentWrites(t *testing.T) {
	tmp := t.TempDir()
	fs := afero.NewOsFs()
	path := filepath.Join(tmp, "credentials.json")

	payloadA := []byte(`{"writer":"A"}`)
	payloadB := []byte(`{"writer":"B"}`)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		WriteFile(fs, path, payloadA)
	}()
	go func() {
		defer wg.Done()
		WriteFile(fs, path, payloadB)
	}()

	wg.Wait()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read final file: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("final file is not valid JSON: %q", raw)
	}
}

// TestWriteFile_DirMode_OnOsFs verifies WriteFile creates the parent dir 0o700.
func TestWriteFile_DirMode_OnOsFs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits not applicable on Windows")
	}

	old := umaskZero(t)
	defer old()

	tmp := t.TempDir()
	fs := afero.NewOsFs()
	dir := filepath.Join(tmp, "neo4j", "cli")
	path := filepath.Join(dir, "credentials.json")

	WriteFile(fs, path, []byte(`{}`))

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("dir mode = %#o, want 0700", got)
	}
}
