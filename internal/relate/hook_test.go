// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build !relate

// In a build without the `relate` tag the engine hooks must be nil (not
// installed), so the command surfaces surface NotCompiledError instead of
// nil-dereferencing. The mirrored assertion for tagged builds lives in
// engine_test.go.

package relate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenStoreHookNotInstalledWithoutTag(t *testing.T) {
	assert.Nil(t, OpenStore)
	assert.Nil(t, OpenStoreReadOnly)
}
