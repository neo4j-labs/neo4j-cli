// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package login

import (
	"errors"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/stretchr/testify/assert"
)

func TestReadLoginConfig(t *testing.T) {
	allVars := map[string]string{
		envDeviceEndpoint: "https://device.example.com/authorize",
		envTokenEndpoint:  "https://token.example.com/oauth/token",
		envClientID:       "my-client-id",
		envAudience:       "https://api.example.com",
	}

	setEnv := func(t *testing.T, vars map[string]string) {
		t.Helper()
		for k, v := range vars {
			t.Setenv(k, v)
		}
	}

	t.Run("all vars set returns populated struct", func(t *testing.T) {
		setEnv(t, allVars)
		cfg, err := readLoginConfig()
		assert.NoError(t, err)
		assert.Equal(t, allVars[envDeviceEndpoint], cfg.DeviceEndpoint)
		assert.Equal(t, allVars[envTokenEndpoint], cfg.TokenEndpoint)
		assert.Equal(t, allVars[envClientID], cfg.ClientID)
		assert.Equal(t, allVars[envAudience], cfg.Audience)
	})

	missingVarCases := []struct {
		name       string
		missingVar string
	}{
		{
			name:       "missing device endpoint",
			missingVar: envDeviceEndpoint,
		},
		{
			name:       "missing token endpoint",
			missingVar: envTokenEndpoint,
		},
		{
			name:       "missing client ID",
			missingVar: envClientID,
		},
		{
			name:       "missing audience",
			missingVar: envAudience,
		},
	}

	for _, tc := range missingVarCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set all vars then clear the one under test.
			for k, v := range allVars {
				if k != tc.missingVar {
					t.Setenv(k, v)
				}
			}
			t.Setenv(tc.missingVar, "")

			cfg, err := readLoginConfig()
			assert.Nil(t, cfg)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.missingVar)

			var cliErr *clierr.CLIError
			assert.True(t, errors.As(err, &cliErr), "error should be a CLIError")
			assert.Equal(t, 2, cliErr.Code, "should be a usage error (exit code 2)")
		})
	}
}
