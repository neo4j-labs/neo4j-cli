//go:build windows

// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg

import (
	"golang.org/x/sys/windows/registry"
)

func init() {
	p, err := registry.ExpandString("%LOCALAPPDATA%")

	if err != nil {
		panic(err)
	}

	ConfigPrefix = p
}
