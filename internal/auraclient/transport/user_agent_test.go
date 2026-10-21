// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package transport

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserAgent_IncludesVersionAndInvoker(t *testing.T) {
	assert.Regexp(t, `^Neo4jCLI/1\.2\.3 \((agent|script|human)\)$`, userAgent("1.2.3"))
}
