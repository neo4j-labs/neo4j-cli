// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package analytics

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetParentProcess_ReturnsBinaryNameOnly(t *testing.T) {
	got := GetParentProcess()
	// "" is acceptable when the parent cannot be resolved (e.g. restricted CI);
	// otherwise it must be a bare file name with no directory component.
	assert.False(t, strings.ContainsAny(got, `/\`), "got %q, want no directory", got)
}
