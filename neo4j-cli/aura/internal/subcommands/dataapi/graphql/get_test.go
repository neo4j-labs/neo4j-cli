// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetGraphQLDataApi(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	instanceId := "2f49c2b3"
	dataApiId := "afdb4e9d"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusOK, `{
			"data": {
                "features": {
                        "subgraph": false
                },
                "id": "afdb4e9d",
                "name": "friendly-name",
                "status": "ready",
                "type_definitions": "dHlwZSBBY3RvciB7CiAgbmFtZTogU3RyaW5nCiAgbW92aWVzOiBbTW92aWUhXSEgQHJlbGF0aW9uc2hpcCh0eXBlOiAiQUNURURfSU4iLCBkaXJlY3Rpb246IE9VVCkKfQoKdHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwogIGFjdG9yczogW0FjdG9yIV0hIEByZWxhdGlvbnNoaXAodHlwZTogIkFDVEVEX0lOIiwgZGlyZWN0aW9uOiBJTikKfQ==",
                "url": "https://afdb4e9d.28be6e4d8d3e836019.graphql.neo4j.io/graphql"
        	}
		}`)

	helper.ExecuteCommand(fmt.Sprintf("data-api graphql get --format json --instance-id %s %s", instanceId, dataApiId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": {
			"features": {
					"subgraph": false
			},
			"id": "afdb4e9d",
			"name": "friendly-name",
			"status": "ready",
			"type_definitions": "dHlwZSBBY3RvciB7CiAgbmFtZTogU3RyaW5nCiAgbW92aWVzOiBbTW92aWUhXSEgQHJlbGF0aW9uc2hpcCh0eXBlOiAiQUNURURfSU4iLCBkaXJlY3Rpb246IE9VVCkKfQoKdHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwogIGFjdG9yczogW0FjdG9yIV0hIEByZWxhdGlvbnNoaXAodHlwZTogIkFDVEVEX0lOIiwgZGlyZWN0aW9uOiBJTikKfQ==",
			"url": "https://afdb4e9d.28be6e4d8d3e836019.graphql.neo4j.io/graphql"
        }
	}`)
}

func TestGetGraphQLDataApiIncludingGraphQLServerErrors(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	instanceId := "2f49c2b3"
	dataApiId := "afdb4e9d"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusOK, `{
			"data": {
                "features": {
                        "subgraph": false
                },
                "id": "afdb4e9d",
                "name": "friendly-name",
                "status": "ready",
                "type_definitions": "dHlwZSBBY3RvciB7CiAgbmFtZTogU3RyaW5nCiAgbW92aWVzOiBbTW92aWUhXSEgQHJlbGF0aW9uc2hpcCh0eXBlOiAiQUNURURfSU4iLCBkaXJlY3Rpb246IE9VVCkKfQoKdHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwogIGFjdG9yczogW0FjdG9yIV0hIEByZWxhdGlvbnNoaXAodHlwZTogIkFDVEVEX0lOIiwgZGlyZWN0aW9uOiBJTikKfQ==",
                "url": "https://afdb4e9d.28be6e4d8d3e836019.graphql.neo4j.io/graphql"
        	},
			"errors": [
				{
					"reason": "server-err",
					"message": "an error with the graphql server"
				},
				{
					"reason": "invalid-typedefs",
					"message": "an error with the type defs"
				}
			]
		}`)

	helper.ExecuteCommand(fmt.Sprintf("data-api graphql get --format json --instance-id %s %s", instanceId, dataApiId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": {
			"features": {
					"subgraph": false
			},
			"id": "afdb4e9d",
			"name": "friendly-name",
			"status": "ready",
			"type_definitions": "dHlwZSBBY3RvciB7CiAgbmFtZTogU3RyaW5nCiAgbW92aWVzOiBbTW92aWUhXSEgQHJlbGF0aW9uc2hpcCh0eXBlOiAiQUNURURfSU4iLCBkaXJlY3Rpb246IE9VVCkKfQoKdHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwogIGFjdG9yczogW0FjdG9yIV0hIEByZWxhdGlvbnNoaXAodHlwZTogIkFDVEVEX0lOIiwgZGlyZWN0aW9uOiBJTikKfQ==",
			"url": "https://afdb4e9d.28be6e4d8d3e836019.graphql.neo4j.io/graphql"
        },
		"errors": [
			{
				"message": "an error with the graphql server",
				"reason": "server-err"
			},
			{
				"message": "an error with the type defs",
				"reason": "invalid-typedefs"
			}
		]
	}`)
}
