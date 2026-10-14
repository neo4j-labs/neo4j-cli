// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"errors"
	"testing"
)

// setGOOSForTest points the per-platform branches of appSupportDir at
// `goos` for the duration of the test. currentGOOS is a process global, so
// callers must not t.Parallel().
func setGOOSForTest(t *testing.T, goos string) {
	t.Helper()
	prev := currentGOOS
	currentGOOS = goos
	t.Cleanup(func() { currentGOOS = prev })
}

// setHomeForTest points the homeDirFn seam at `home` for the duration of the
// test. An empty home makes the seam return an error, preserving the
// "no home directory is available" (ok=false) expectations without mutating
// the process environment, which is not cross-platform: Windows PowerShell
// sessions don't set HOME and os.UserHomeDir reads %USERPROFILE% there.
// homeDirFn is a process global, so callers must not t.Parallel().
func setHomeForTest(t *testing.T, home string) {
	t.Helper()
	restore := SetHomeDirFnForTest(func() (string, error) {
		if home == "" {
			return "", errors.New("no home directory")
		}
		return home, nil
	})
	t.Cleanup(restore)
}
