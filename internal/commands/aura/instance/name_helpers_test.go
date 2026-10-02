// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultInstanceName(t *testing.T) {
	testCases := []struct {
		name          string
		existingNames []string
		want          string
	}{
		{
			name:          "no existing instances returns Instance01",
			existingNames: nil,
			want:          "Instance01",
		},
		{
			name:          "empty slice returns Instance01",
			existingNames: []string{},
			want:          "Instance01",
		},
		{
			name:          "Instance01 taken returns Instance02",
			existingNames: []string{"Instance01"},
			want:          "Instance02",
		},
		{
			name:          "Instance01 and Instance03 taken returns Instance02 (gap)",
			existingNames: []string{"Instance01", "Instance03"},
			want:          "Instance02",
		},
		{
			name:          "Instance01 and Instance02 taken returns Instance03",
			existingNames: []string{"Instance01", "Instance02"},
			want:          "Instance03",
		},
		{
			name: "all two-digit names taken rolls to Instance100",
			existingNames: func() []string {
				names := make([]string, 99)
				for i := 1; i <= 99; i++ {
					names[i-1] = fmt.Sprintf("Instance%02d", i)
				}
				return names
			}(),
			want: "Instance100",
		},
		{
			name:          "case-insensitive: instance01 (lowercase) blocks Instance01",
			existingNames: []string{"instance01"},
			want:          "Instance02",
		},
		{
			name:          "case-insensitive: INSTANCE01 blocks Instance01",
			existingNames: []string{"INSTANCE01"},
			want:          "Instance02",
		},
		{
			name:          "mixed case entries still resolved correctly",
			existingNames: []string{"INSTANCE01", "Instance02", "instance03"},
			want:          "Instance04",
		},
		{
			name:          "unrelated names are ignored",
			existingNames: []string{"my-db", "production", "test-instance"},
			want:          "Instance01",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := defaultInstanceName(tc.existingNames)
			assert.Equal(t, tc.want, got)
		})
	}
}
