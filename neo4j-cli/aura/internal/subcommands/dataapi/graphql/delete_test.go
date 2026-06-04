// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestDeleteGraphQLDataApi(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

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

func TestDeleteGraphQLDataApiConfirmGate(t *testing.T) {
	instanceId := "2f49c2b3"
	dataApiId := "afdb4e9d"
	base := fmt.Sprintf("data-api graphql delete --instance-id %s %s --rw", instanceId, dataApiId)
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "aura data-api graphql delete",
		NoFlagsArgs:   base,
		BothFlagsArgs: base + " --yes --force",
		ResourceLabel: "graphql",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			helper := testutils.NewAuraTestHelper(t)
			t.Cleanup(helper.Close)
			mock := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusAccepted, `{"data": {"id": "afdb4e9d", "status": "deleting"}}`)
			helper.SetStdin(stdin)
			err := helper.ExecuteCommandE(args)
			return confirmtest.GateRunResult{Err: err, Stderr: helper.PrintErr(), Invoked: mock.CalledWithMethod(http.MethodDelete)}
		},
	})
}
