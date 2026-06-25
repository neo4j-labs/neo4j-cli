// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGetenv builds a getenv func backed by a fixed map.
func fakeGetenv(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

func TestHasAnyCredentialEnvVar(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"empty", map[string]string{}, false},
		{"aura sentinel", map[string]string{credentials.EnvAuraClientID: "id"}, true},
		{"dbms sentinel", map[string]string{credentials.EnvURI: "neo4j://x"}, true},
		{"embed sentinel", map[string]string{credentials.EnvEmbedProvider: "openai"}, true},
		{"non-sentinel only", map[string]string{credentials.EnvPassword: "p", credentials.EnvEmbedAPIKey: "k"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, credentials.HasAnyCredentialEnvVar(fakeGetenv(tt.env)))
		})
	}
}

func TestValidateEnvCredentialSet_Aura(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		wantErr   bool
		errSubstr []string
	}{
		{"empty", map[string]string{}, false, nil},
		{
			"complete",
			map[string]string{credentials.EnvAuraClientID: "id", credentials.EnvAuraClientSecret: "secret"},
			false, nil,
		},
		{
			"id only",
			map[string]string{credentials.EnvAuraClientID: "id"},
			true, []string{credentials.EnvAuraClientSecret},
		},
		{
			"secret only",
			map[string]string{credentials.EnvAuraClientSecret: "secret"},
			true, []string{credentials.EnvAuraClientID},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := credentials.ValidateEnvCredentialSet(credentials.AuraEnvSpec, fakeGetenv(tt.env))
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, s := range tt.errSubstr {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}

func TestValidateEnvCredentialSet_DBMS(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		wantErr   bool
		errSubstr []string
	}{
		{"empty", map[string]string{}, false, nil},
		{
			"complete",
			map[string]string{
				credentials.EnvURI:      "neo4j://x",
				credentials.EnvUsername: "neo4j",
				credentials.EnvPassword: "pw",
			},
			false, nil,
		},
		{
			"complete with optional database",
			map[string]string{
				credentials.EnvURI:      "neo4j://x",
				credentials.EnvUsername: "neo4j",
				credentials.EnvPassword: "pw",
				credentials.EnvDatabase: "movies",
			},
			false, nil,
		},
		{
			"uri only",
			map[string]string{credentials.EnvURI: "neo4j://x"},
			true, []string{credentials.EnvUsername, credentials.EnvPassword},
		},
		{
			"missing password",
			map[string]string{credentials.EnvURI: "neo4j://x", credentials.EnvUsername: "neo4j"},
			true, []string{credentials.EnvPassword},
		},
		{
			"database alone does not trigger error",
			map[string]string{credentials.EnvDatabase: "movies"},
			false, nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := credentials.ValidateEnvCredentialSet(credentials.DBMSEnvSpec, fakeGetenv(tt.env))
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, s := range tt.errSubstr {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}
