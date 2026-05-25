// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestDeleteGraphQLDataApi(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("flag.aura-beta", true)

	instanceId := "2f49c2b3"
	dataApiId := "afdb4e9d"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusAccepted, `{
			"data": {
                "id": "afdb4e9d",
                "name": "friendly-name",
                "status": "ready",
                "url": "https://afdb4e9d.28be6e4d8d3e836019.graphql.neo4j.io/graphql"
        	}
		}`)

	helper.ExecuteCommand(fmt.Sprintf("data-api graphql delete --format json --instance-id %s %s --rw --yes --force", instanceId, dataApiId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOutJson(`{
		"data": {
			"id": "afdb4e9d",
			"name": "friendly-name",
			"status": "ready",
			"url": "https://afdb4e9d.28be6e4d8d3e836019.graphql.neo4j.io/graphql"
        }
	}`)
}

func TestDeleteGraphQLDataApiWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("flag.aura-beta", true)

	instanceId := "2f49c2b3"
	dataApiId := "afdb4e9d"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusAccepted, `{
			"data": {
                "id": "afdb4e9d",
                "name": "friendly-name",
                "status": "ready",
                "url": "https://afdb4e9d.28be6e4d8d3e836019.graphql.neo4j.io/graphql"
        	}
		}`)

	helper.ExecuteCommand(fmt.Sprintf("data-api graphql delete --format json --instance-id %s %s\"\n\" --rw --yes --force", instanceId, dataApiId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
}

func TestDeleteGraphQLDataApiConfirmGate_NonTTYWithoutFlags_Exit2(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("flag.aura-beta", true)
	instanceId := "2f49c2b3"
	dataApiId := "afdb4e9d"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusAccepted, "{}")

	err := helper.ExecuteCommandE(fmt.Sprintf("data-api graphql delete --instance-id %s %s --rw", instanceId, dataApiId))

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("error %q missing 'pass both --yes and --force'", err.Error())
	}
	mockHandler.AssertCalledTimes(0)
}

func TestDeleteGraphQLDataApiConfirmGate_NonTTYWithBothFlags_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("flag.aura-beta", true)
	instanceId := "2f49c2b3"
	dataApiId := "afdb4e9d"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusAccepted, `{"data": {"id": "afdb4e9d", "status": "deleting"}}`)

	helper.ExecuteCommand(fmt.Sprintf("data-api graphql delete --instance-id %s %s --rw --yes --force", instanceId, dataApiId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
}

func TestDeleteGraphQLDataApiConfirmGate_TTYAnswerY_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetStdin("y\n")
	helper.SetConfigValue("flag.aura-beta", true)
	instanceId := "2f49c2b3"
	dataApiId := "afdb4e9d"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusAccepted, `{"data": {"id": "afdb4e9d", "status": "deleting"}}`)

	helper.ExecuteCommand(fmt.Sprintf("data-api graphql delete --instance-id %s %s --rw", instanceId, dataApiId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
	helper.AssertErrContainsStrings([]string{"Delete graphql"})
}

func TestDeleteGraphQLDataApiConfirmGate_TTYAnswerN_Cancels(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetStdin("N\n")
	helper.SetConfigValue("flag.aura-beta", true)
	instanceId := "2f49c2b3"
	dataApiId := "afdb4e9d"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusAccepted, "{}")

	err := helper.ExecuteCommandE(fmt.Sprintf("data-api graphql delete --instance-id %s %s --rw", instanceId, dataApiId))

	if err != nil {
		t.Fatalf("expected nil on cancel, got %v", err)
	}
	mockHandler.AssertCalledTimes(0)
	helper.AssertErrContainsStrings([]string{"cancelled."})
}
