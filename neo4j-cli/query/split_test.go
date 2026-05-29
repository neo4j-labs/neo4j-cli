// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitStatements(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single statement no semicolon",
			input: "RETURN 1",
			want:  []string{"RETURN 1"},
		},
		{
			name:  "single statement trailing semicolon",
			input: "RETURN 1;",
			want:  []string{"RETURN 1"},
		},
		{
			name:  "single statement trailing semicolon and newline",
			input: "RETURN 1;\n",
			want:  []string{"RETURN 1"},
		},
		{
			name:  "two statements",
			input: "RETURN 1;\nRETURN 2;",
			want:  []string{"RETURN 1", "RETURN 2"},
		},
		{
			name:  "three statements no final semicolon",
			input: "RETURN 1;\nRETURN 2;\nRETURN 3",
			want:  []string{"RETURN 1", "RETURN 2", "RETURN 3"},
		},
		{
			name:  "trailing semicolon yields no empty fragment",
			input: "RETURN 1;\nRETURN 2;\n",
			want:  []string{"RETURN 1", "RETURN 2"},
		},
		{
			name:  "mid-line semicolon is not a split point",
			input: "RETURN 'a; b'",
			want:  []string{"RETURN 'a; b'"},
		},
		{
			name:  "mid-line semicolon with trailing text kept verbatim",
			input: "RETURN 1; RETURN 2",
			want:  []string{"RETURN 1; RETURN 2"},
		},
		{
			name:  "blank lines between statements dropped",
			input: "RETURN 1;\n\n\nRETURN 2;",
			want:  []string{"RETURN 1", "RETURN 2"},
		},
		{
			name:  "semicolon with trailing whitespace before newline",
			input: "RETURN 1;   \nRETURN 2",
			want:  []string{"RETURN 1", "RETURN 2"},
		},
		{
			name:  "CRLF handled like LF",
			input: "RETURN 1;\r\nRETURN 2;\r\n",
			want:  []string{"RETURN 1", "RETURN 2"},
		},
		{
			name:  "multi-line statement accumulated",
			input: "MATCH (n)\nRETURN n;\nRETURN 2",
			want:  []string{"MATCH (n)\nRETURN n", "RETURN 2"},
		},
		{
			name:  "whitespace-only fragment dropped",
			input: "RETURN 1;\n   \nRETURN 2",
			want:  []string{"RETURN 1", "RETURN 2"},
		},
		{
			name:  "empty input yields nothing",
			input: "",
			want:  nil,
		},
		{
			name:  "whitespace-only input yields nothing",
			input: "   \n\t\n",
			want:  nil,
		},
		{
			name:  "only semicolon yields nothing",
			input: ";",
			want:  nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, splitStatements(tc.input))
		})
	}
}
