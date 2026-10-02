// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/aura"
	"github.com/neo4j/cli/internal/clierr"
)

func TestValidateResourceID(t *testing.T) {
	for _, tc := range []struct {
		name    string
		id      string
		wantErr bool
	}{
		{name: "valid short id", id: "2f49c2b3", wantErr: false},
		{name: "valid uuid", id: "11111111-1111-1111-1111-111111111111", wantErr: false},
		{name: "dots within a segment are fine", id: "a..b", wantErr: false},
		{name: "empty", id: "", wantErr: true},
		{name: "dot segment", id: ".", wantErr: true},
		{name: "dotdot segment", id: "..", wantErr: true},
		{name: "traversal", id: "../../../target", wantErr: true},
		{name: "embedded slash", id: "a/b", wantErr: true},
		{name: "embedded backslash", id: `a\b`, wantErr: true},
		{name: "query separator", id: "x?admin=true", wantErr: true},
		{name: "fragment separator", id: "x#frag", wantErr: true},
		{name: "percent escape", id: "x%2e%2e", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := aura.ValidateResourceID("instance", tc.id)
			if tc.wantErr {
				require.Error(t, err)
				var ce *clierr.CLIError
				require.True(t, errors.As(err, &ce))
				assert.Contains(t, ce.Message, fmt.Sprintf("invalid instance id %q", tc.id))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
