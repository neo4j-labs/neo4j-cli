// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build !windows

package k8s

import (
	"os"
	"path/filepath"
	"syscall"
)

// validateLocation applies the trust rules for a resolved binary on unix:
// neither the file nor its directory can be writable by other users, and the
// file must belong to the current user or to root. A binary that fails these
// rules is replaceable by a local attacker, so the engine refuses to run it.
func validateLocation(name, resolved string, info os.FileInfo) error {
	if info.Mode().Perm()&0o002 != 0 {
		return untrustedBinaryErr(name, resolved, "the binary is writable by other users")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if uid := os.Geteuid(); stat.Uid != 0 && int(stat.Uid) != uid {
			return untrustedBinaryErr(name, resolved, "the binary is owned by neither the current user nor root")
		}
	}
	dir := filepath.Dir(resolved)
	dirInfo, err := os.Stat(dir)
	if err != nil {
		return untrustedBinaryErr(name, resolved, "the containing directory cannot be read")
	}
	if dirInfo.Mode().Perm()&0o002 != 0 {
		return untrustedBinaryErr(name, resolved, "the containing directory "+dir+" is writable by other users")
	}
	return nil
}
