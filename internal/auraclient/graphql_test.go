// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/clievents"
)

func TestGraphQLPath_ValidatesEveryID(t *testing.T) {
	s := graphqlService{}

	p, err := s.path("inst1", "")
	require.NoError(t, err)
	assert.Equal(t, "/instances/inst1/data-apis/graphql", p)

	p, err = s.path("inst1", "api1", "pause")
	require.NoError(t, err)
	assert.Equal(t, "/instances/inst1/data-apis/graphql/api1/pause", p)

	for name, call := range map[string]func() (string, error){
		"instance traversal":   func() (string, error) { return s.path("../x", "") },
		"data api traversal":   func() (string, error) { return s.path("inst1", "../../y") },
		"data api query chars": func() (string, error) { return s.path("inst1", "a?b=c") },
	} {
		t.Run(name, func(t *testing.T) {
			_, err := call()
			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce), "want a validation clierr, got %v", err)
		})
	}

	ap := authProviderService{}
	p, err = ap.path("inst1", "api1", "prov1")
	require.NoError(t, err)
	assert.Equal(t, "/instances/inst1/data-apis/graphql/api1/auth-providers/prov1", p)
	_, err = ap.path("inst1", "api1", "../../z")
	assert.Error(t, err)
	_, err = ap.path("inst1", "", "")
	assert.Error(t, err, "an empty data api id must not collapse the path")
}

func TestGraphQLCreateBody(t *testing.T) {
	body := GraphQLCreate{Memory: "512MB", ServiceAccount: "read_only", TypeDefinitions: "dHlwZQ=="}.body("GraphQL01")

	assert.Equal(t, "GraphQL01", body["name"])
	assert.Equal(t, "512MB", body["memory"])
	assert.Equal(t, "dHlwZQ==", body["type_definitions"])
	assert.Equal(t, map[string]string{"service_account": "read_only"}, body["aura_instance"])
	assert.Equal(t, map[string]any{
		"authentication_providers": []map[string]any{{"type": "api-key", "name": "default", "enabled": true}},
	}, body["security"], "a default api-key provider is always created")
}

func TestGraphQLPatchBody_OnlySendsNonEmptyFields(t *testing.T) {
	assert.Empty(t, GraphQLPatch{}.body())
	assert.Equal(t,
		map[string]any{"name": "n", "aura_instance": map[string]string{"service_account": "read_write"}},
		GraphQLPatch{Name: "n", ServiceAccount: "read_write"}.body())
}

func TestOriginsBody_EmptyListSendsWorkaroundField(t *testing.T) {
	withOrigins := originsBody([]string{"https://a.example"})
	assert.NotContains(t, withOrigins, "test")

	empty := originsBody(nil)
	assert.Equal(t, "ignore me", empty["test"], "the API rejects a PATCH that only empties the list")
	cors := empty["security"].(map[string]any)["cors_policy"].(map[string]any)
	assert.Empty(t, cors["allowed_origins"])
}

func TestRegisterProviderKeys_RedactsKeysFromBothRecordShapes(t *testing.T) {
	const dataAPIKey = "dataapi-secret-key-1111"
	const providerKey = "provider-secret-key-2222"

	registerProviderKeys([]map[string]any{
		{"id": "a", "authentication_providers": []any{map[string]any{"key": dataAPIKey}, map[string]any{"name": "no-key"}}},
		{"id": "p", "key": providerKey},
	})

	assert.NotContains(t, clievents.RedactText("created with "+dataAPIKey), dataAPIKey)
	assert.NotContains(t, clievents.RedactText("created with "+providerKey), providerKey)
}
