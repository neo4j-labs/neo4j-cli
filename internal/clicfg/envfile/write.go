// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package envfile

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/afero"
)

// Write stores contents with owner-only permissions on Unix. It writes a
// temporary file in the target directory, then renames it over path.
// This prevents a planted symlink from redirecting the credential write.
// The temporary file never exposes the contents with broader permissions.
// If any step fails, Write removes its temporary file.
func Write(fs afero.Fs, path, contents string) error {
	dir := filepath.Dir(path)
	tmp, err := afero.TempFile(fs, dir, ".neo4j-cli-env-")
	if err != nil {
		return fmt.Errorf("create temp env-file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = fs.Remove(tmpPath) }
	if _, err := tmp.WriteString(contents); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp env-file %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp env-file %s: %w", tmpPath, err)
	}
	if err := fs.Chmod(tmpPath, 0o600); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp env-file %s: %w", tmpPath, err)
	}
	if err := fs.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("rename env-file to %s: %w", path, err)
	}
	return nil
}
