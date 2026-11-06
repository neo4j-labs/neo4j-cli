// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build windows

package k8s

import "os"

// validateLocation is a no-op on Windows. The unix ownership and permission
// bits do not map onto NTFS access control lists. The absolute-path and
// regular-file rules in validateBinary are the Windows control.
func validateLocation(_, _ string, _ os.FileInfo) error {
	return nil
}
