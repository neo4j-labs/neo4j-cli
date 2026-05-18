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

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session get %s --organization-id %s --project-id %s", testSessionID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
  "data": {
	"cloud_provider": "gcp",
    "created_at": "2025-04-04T09:32:35Z",
    "expiry_date": "2025-04-11T09:32:35Z",
    "host": "559c94c7-15de43fg.neo4j.io",
    "id": "559c94c7-15de43fg",
    "instance_id": null,
    "memory": "8GB",
    "name": "people-and-fruits",
    "project_id": "YOUR_PROJECT_ID",
    "region": "europe-west1",
    "status": "Ready",
    "ttl": "20m0s",
    "user_id": "YOUR_USER_ID"
  }
}`)
}

func TestGetSessionWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session get %s", testSessionID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AsssertOk()
}

func TestGetSessionMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session get %s", testSessionID))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestGetSessionMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session get %s --organization-id %s", testSessionID, testOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestGetSessionProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session get %s --organization-id %s --project-id unknown-project", testSessionID, testOrgID))

	helper.AssertErr("Error: could not find project unknown-project in organization " + testOrgID)
}

func TestGetSessionNotInProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	// Session belongs to a different project.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, "other-project-id"))

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session get %s --organization-id %s --project-id %s", testSessionID, testOrgID, testProjectID))

	helper.AssertErr(fmt.Sprintf("Error: could not find session %s in project %s", testSessionID, testProjectID))
}

func TestGetSessionError(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	sessionId := "s-f5138f3b-7956"

	registerProjectsMock(&helper)

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

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session get %s --organization-id %s --project-id %s", sessionId, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut("")
	helper.AssertErr("Error: [session with id s-f5138f3b-7956 not found]")
}
