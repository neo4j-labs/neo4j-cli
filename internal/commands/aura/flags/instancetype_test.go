// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceTypeSet(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expected      string
		expectedError string
	}{
		{name: "free is accepted", input: "free", expected: "free"},
		{name: "professional is accepted", input: "professional", expected: "professional"},
		{name: "business-critical is accepted", input: "business-critical", expected: "business-critical"},
		{name: "virtual-dedicated-cloud is accepted", input: "virtual-dedicated-cloud", expected: "virtual-dedicated-cloud"},

		// Legacy v1 names are accepted as input, but normalized to the v2beta1
		// name the API expects.
		{name: "free-db is normalized to free", input: "free-db", expected: "free"},
		{name: "professional-db is normalized to professional", input: "professional-db", expected: "professional"},
		{name: "enterprise-db is normalized to virtual-dedicated-cloud", input: "enterprise-db", expected: "virtual-dedicated-cloud"},

		{
			name:          "professional-ds is rejected with guidance",
			input:         "professional-ds",
			expectedError: `AuraDS instance types are no longer offered; create a "professional" or "virtual-dedicated-cloud" instance instead, and configure graph analytics separately`,
		},
		{
			name:          "enterprise-ds is rejected with guidance",
			input:         "enterprise-ds",
			expectedError: `AuraDS instance types are no longer offered; create a "professional" or "virtual-dedicated-cloud" instance instead, and configure graph analytics separately`,
		},
		{
			name:          "unknown value is rejected with the accepted set",
			input:         "invalid-db",
			expectedError: `must be one of "free", "professional", "business-critical", or "virtual-dedicated-cloud"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var instanceType InstanceType
			err := instanceType.Set(tt.input)

			if tt.expectedError != "" {
				require.EqualError(t, err, tt.expectedError)
				assert.Equal(t, InstanceType(""), instanceType)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, instanceType.String())
		})
	}
}

// customer-managed-key create still POSTs to the v1 endpoint, so its flag must
// keep taking (and sending) the v1 names unchanged.
func TestLegacyInstanceTypeSet(t *testing.T) {
	for _, valid := range []string{"free-db", "professional-db", "business-critical", "enterprise-db", "professional-ds", "enterprise-ds"} {
		t.Run(valid+" is accepted unchanged", func(t *testing.T) {
			var instanceType LegacyInstanceType
			require.NoError(t, instanceType.Set(valid))
			assert.Equal(t, valid, instanceType.String())
		})
	}

	t.Run("v2beta1 name is rejected", func(t *testing.T) {
		var instanceType LegacyInstanceType
		require.EqualError(t, instanceType.Set("virtual-dedicated-cloud"), `must be one of "free-db", "professional-db", "business-critical", "enterprise-db", "professional-ds", or "enterprise-ds"`)
	})
}
