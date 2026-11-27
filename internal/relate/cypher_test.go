// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuoteIdent(t *testing.T) {
	assert.Equal(t, "`Person`", QuoteIdent("Person"))
	assert.Equal(t, "`fo``o`", QuoteIdent("fo`o"))
}
