// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func allowedConfigsPath() string {
	return virtualGraphsPath() + "/allowed-configs"
}

func TestAllowedConfigs(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(allowedConfigsPath(), http.StatusOK, `{
		"data": {
			"default_memory": "4Gi",
			"configs": [
				{"memory": "4Gi"},
				{"memory": "8Gi"}
			]
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph allowed-configs --organization-id %s --project-id %s", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
  "data": {
    "configs": [
      {
        "memory": "4Gi"
      },
      {
        "memory": "8Gi"
      }
    ],
    "default_memory": "4Gi"
  }
}`)
}

func TestAllowedConfigsRejectsPositionalArg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph allowed-configs extra --organization-id %s --project-id %s", testOrgID, testProjectID))

	helper.AssertErrContainsStrings([]string{"unknown command"})
}
