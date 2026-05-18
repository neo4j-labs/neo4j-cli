// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetSession(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	sessionId := "559c94c7-15de43fg"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", sessionId), http.StatusOK, `{
  "data": {
    "id": "559c94c7-15de43fg",
    "name": "people-and-fruits-with-db",
    "memory": "4GB",
    "instance_id": "559c94c7",
    "status": "Ready",
    "created_at": "2025-04-04T09:32:35Z",
    "host": "559c94c7-15de43fg.ORCHESTRA.neo4j.io",
    "expiry_date": "2025-04-11T09:32:35Z",
    "ttl": "8m",
    "user_id": "YOUR_USER_ID",
    "tenant_id": "YOUR_PROJECT_ID",
    "cloud_provider": "gcp",
    "region": "europe-west1"
  }
}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session get %s", sessionId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
  "data": {
	"cloud_provider": "gcp",
    "created_at": "2025-04-04T09:32:35Z",
    "expiry_date": "2025-04-11T09:32:35Z",
    "host": "559c94c7-15de43fg.ORCHESTRA.neo4j.io",
    "id": "559c94c7-15de43fg",
    "instance_id": "559c94c7",
    "memory": "4GB",
    "name": "people-and-fruits-with-db",
    "region": "europe-west1",
    "status": "Ready",
    "tenant_id": "YOUR_PROJECT_ID",
    "ttl": "8m",
    "user_id": "YOUR_USER_ID"
  }
}`)
}

func TestGetSessionWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	sessionId := "559c94c7-15de43fg"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", sessionId), http.StatusOK, `{
  "data": {
    "id": "559c94c7-15de43fg",
    "name": "people-and-fruits-with-db",
    "memory": "4GB",
    "instance_id": "559c94c7",
    "status": "Ready",
    "created_at": "2025-04-04T09:32:35Z",
    "host": "559c94c7-15de43fg.ORCHESTRA.neo4j.io",
    "expiry_date": "2025-04-11T09:32:35Z",
    "ttl": "8m",
    "user_id": "YOUR_USER_ID",
    "tenant_id": "YOUR_PROJECT_ID",
    "cloud_provider": "gcp",
    "region": "europe-west1"
  }
}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session get %s\"\n\"", sessionId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
}

func TestGetSessionError(t *testing.T) {

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

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session get %s", sessionId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut("")
	helper.AssertErr("Error: [session with id s-f5138f3b-7956 not found]")

}
