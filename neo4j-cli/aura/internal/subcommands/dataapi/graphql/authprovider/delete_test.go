// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package authprovider_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestDeleteAuthProvider(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("flag.aura-beta", true)

	instanceId := "2f49c2b3"
	dataApiId := "a342b824"
	authProviderId := "87d46b4b-3bfb-4ad2-8dac-0e95cf72d39f"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s/auth-providers/%s", instanceId, dataApiId, authProviderId), http.StatusAccepted, `{
		"data": {
			"id": "87d46b4b-3bfb-4ad2-8dac-0e95cf72d39f",
			"name": "test-key",
			"type": "jwks",
			"enabled": true,
			"url": "https://test.com/.well-known/jwks.json"
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("data-api graphql auth-provider delete %s --format json --instance-id %s --data-api-id %s --rw --yes --force", authProviderId, instanceId, dataApiId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

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

func TestDeleteAuthProviderWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("flag.aura-beta", true)

	instanceId := "2f49c2b3"
	dataApiId := "a342b824"
	authProviderId := "87d46b4b-3bfb-4ad2-8dac-0e95cf72d39f"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s/auth-providers/%s", instanceId, dataApiId, authProviderId), http.StatusAccepted, `{
		"data": {
			"id": "87d46b4b-3bfb-4ad2-8dac-0e95cf72d39f",
			"name": "test-key",
			"type": "jwks",
			"enabled": true,
			"url": "https://test.com/.well-known/jwks.json"
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("data-api graphql auth-provider delete %s\"\n\" --format json --instance-id %s --data-api-id %s --rw --yes --force", authProviderId, instanceId, dataApiId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
}

func TestDeleteAuthProviderConfirmGate(t *testing.T) {
	instanceId := "2f49c2b3"
	dataApiId := "a342b824"
	authProviderId := "87d46b4b-3bfb-4ad2-8dac-0e95cf72d39f"
	base := fmt.Sprintf("data-api graphql auth-provider delete %s --instance-id %s --data-api-id %s --rw", authProviderId, instanceId, dataApiId)
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "aura data-api graphql auth-provider delete",
		NoFlagsArgs:   base,
		BothFlagsArgs: base + " --yes --force",
		ResourceLabel: "auth-provider",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			helper := testutils.NewAuraTestHelper(t)
			t.Cleanup(helper.Close)
			helper.SetConfigValue("flag.aura-beta", true)
			mock := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s/auth-providers/%s", instanceId, dataApiId, authProviderId), http.StatusAccepted, `{"data": {"id": "`+authProviderId+`"}}`)
			helper.SetStdin(stdin)
			err := helper.ExecuteCommandE(args)
			return confirmtest.GateRunResult{Err: err, Stderr: helper.PrintErr(), Invoked: mock.CalledWithMethod(http.MethodDelete)}
		},
	})
}
