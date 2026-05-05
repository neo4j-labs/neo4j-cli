// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package authprovider_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetAuthProvider(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	instanceId := "2f49c2b3"
	dataApiId := "a342b824"
	authProviderId := "87d46b4b-3bfb-4ad2-8dac-0e95cf72d39f"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s/auth-providers/%s", instanceId, dataApiId, authProviderId), http.StatusOK, `{
		"data": {
			"id": "87d46b4b-3bfb-4ad2-8dac-0e95cf72d39f",
			"name": "test-key",
			"type": "jwks",
			"enabled": true,
			"url": "https://test.com/.well-known/jwks.json"
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("data-api graphql auth-provider get %s --format json --instance-id %s --data-api-id %s", authProviderId, instanceId, dataApiId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": {
			"enabled": true,
			"id": "87d46b4b-3bfb-4ad2-8dac-0e95cf72d39f",
			"name": "test-key",
			"type": "jwks",
			"url": "https://test.com/.well-known/jwks.json"
		}
	}
	`)
}
