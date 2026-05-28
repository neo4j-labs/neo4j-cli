//go:build darwin

// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import (
	"os"
	"os/user"
	"path/filepath"
)

func init() {
	// Use $HOME if set (allows subprocess isolation in tests and respects
	// environment-variable overrides). Fall back to user.Current().HomeDir
	// when $HOME is absent (e.g. some CI environments).
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		currentUser, _ := user.Current()
		homeDir = currentUser.HomeDir
	}

	ConfigPrefix = filepath.Join(homeDir, "Library/Preferences")
}
