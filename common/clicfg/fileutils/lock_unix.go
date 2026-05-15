// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build !windows

package fileutils

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// acquireLock opens <path>.lock, acquires a blocking exclusive advisory lock
// via flock(2), and returns a release function that closes the lock file.
// The OS releases the flock automatically on close.
// The lock file is created if absent and is never deleted.
// The parent directory is created with mode 0o700 if it does not already exist.
func acquireLock(path string) (release func(), err error) {
	lockPath := path + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}
