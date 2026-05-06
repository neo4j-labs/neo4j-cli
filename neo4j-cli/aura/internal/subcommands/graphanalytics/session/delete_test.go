// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestDeleteSession(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	sessionId := "42-24"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", sessionId), http.StatusAccepted, `{
		"data": {
		  "id": "42-24"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --rw", sessionId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOutJson(`{
		"data": {
		  "id": "42-24"
		}
	  }`)
}

func TestDeleteSessionError(t *testing.T) {

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	sessionId := "s-f5138f3b-7956"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", sessionId), http.StatusNotFound, `
{
  "data": null,
  "errors": [
    {
      "id": "",
      "message": "session with id s-f5138f3b-7956 not found",
      "reason": ""
    }
  ]
}
`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --rw", sessionId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOut("")
	helper.AssertErr("Error: [session with id s-f5138f3b-7956 not found]")

}
