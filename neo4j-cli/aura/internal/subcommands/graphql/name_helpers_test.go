// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultGraphQLName(t *testing.T) {
	testCases := []struct {
		name          string
		existingNames []string
		want          string
	}{
		{
			name:          "no existing names returns GraphQL01",
			existingNames: nil,
			want:          "GraphQL01",
		},
		{
			name:          "empty slice returns GraphQL01",
			existingNames: []string{},
			want:          "GraphQL01",
		},
		{
			name:          "GraphQL01 taken returns GraphQL02",
			existingNames: []string{"GraphQL01"},
			want:          "GraphQL02",
		},
		{
			name:          "GraphQL01 and GraphQL03 taken returns GraphQL02 (gap)",
			existingNames: []string{"GraphQL01", "GraphQL03"},
			want:          "GraphQL02",
		},
		{
			name:          "GraphQL01 and GraphQL02 taken returns GraphQL03",
			existingNames: []string{"GraphQL01", "GraphQL02"},
			want:          "GraphQL03",
		},
		{
			name: "all two-digit names taken rolls to GraphQL100",
			existingNames: func() []string {
				names := make([]string, 99)
				for i := 1; i <= 99; i++ {
					names[i-1] = fmt.Sprintf("GraphQL%02d", i)
				}
				return names
			}(),
			want: "GraphQL100",
		},
		{
			name:          "case-insensitive: graphql01 (lowercase) blocks GraphQL01",
			existingNames: []string{"graphql01"},
			want:          "GraphQL02",
		},
		{
			name:          "case-insensitive: GRAPHQL01 blocks GraphQL01",
			existingNames: []string{"GRAPHQL01"},
			want:          "GraphQL02",
		},
		{
			name:          "mixed case entries still resolved correctly",
			existingNames: []string{"GRAPHQL01", "GraphQL02", "graphql03"},
			want:          "GraphQL04",
		},
		{
			name:          "unrelated names are ignored",
			existingNames: []string{"my-api", "production", "test-graphql"},
			want:          "GraphQL01",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := defaultGraphQLName(tc.existingNames)
			assert.Equal(t, tc.want, got)
		})
	}
}
