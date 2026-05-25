// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package authprovider_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/confirm"
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

func TestDeleteAuthProviderConfirmGate_NonTTYWithoutFlags_Exit2(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("flag.aura-beta", true)
	instanceId := "2f49c2b3"
	dataApiId := "a342b824"
	authProviderId := "87d46b4b-3bfb-4ad2-8dac-0e95cf72d39f"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s/auth-providers/%s", instanceId, dataApiId, authProviderId), http.StatusAccepted, "{}")

	err := helper.ExecuteCommandE(fmt.Sprintf("data-api graphql auth-provider delete %s --instance-id %s --data-api-id %s --rw", authProviderId, instanceId, dataApiId))

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("error %q missing 'pass both --yes and --force'", err.Error())
	}
	mockHandler.AssertCalledTimes(0)
}

func TestDeleteAuthProviderConfirmGate_NonTTYWithBothFlags_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("flag.aura-beta", true)
	instanceId := "2f49c2b3"
	dataApiId := "a342b824"
	authProviderId := "87d46b4b-3bfb-4ad2-8dac-0e95cf72d39f"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s/auth-providers/%s", instanceId, dataApiId, authProviderId), http.StatusAccepted, `{"data": {"id": "`+authProviderId+`"}}`)

	helper.ExecuteCommand(fmt.Sprintf("data-api graphql auth-provider delete %s --instance-id %s --data-api-id %s --rw --yes --force", authProviderId, instanceId, dataApiId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
}

func TestDeleteAuthProviderConfirmGate_TTYAnswerY_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetStdin("y\n")
	helper.SetConfigValue("flag.aura-beta", true)
	instanceId := "2f49c2b3"
	dataApiId := "a342b824"
	authProviderId := "87d46b4b-3bfb-4ad2-8dac-0e95cf72d39f"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s/auth-providers/%s", instanceId, dataApiId, authProviderId), http.StatusAccepted, `{"data": {"id": "`+authProviderId+`"}}`)

	helper.ExecuteCommand(fmt.Sprintf("data-api graphql auth-provider delete %s --instance-id %s --data-api-id %s --rw", authProviderId, instanceId, dataApiId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
	helper.AssertErrContainsStrings([]string{"Delete auth-provider"})
}

func TestDeleteAuthProviderConfirmGate_TTYAnswerN_Cancels(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetStdin("N\n")
	helper.SetConfigValue("flag.aura-beta", true)
	instanceId := "2f49c2b3"
	dataApiId := "a342b824"
	authProviderId := "87d46b4b-3bfb-4ad2-8dac-0e95cf72d39f"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s/auth-providers/%s", instanceId, dataApiId, authProviderId), http.StatusAccepted, "{}")

	err := helper.ExecuteCommandE(fmt.Sprintf("data-api graphql auth-provider delete %s --instance-id %s --data-api-id %s --rw", authProviderId, instanceId, dataApiId))

	if !errors.Is(err, confirm.ErrCancelled) {
		t.Fatalf("expected confirm.ErrCancelled on cancel, got %v", err)
	}
	mockHandler.AssertCalledTimes(0)
	helper.AssertErrContainsStrings([]string{"cancelled."})
}
