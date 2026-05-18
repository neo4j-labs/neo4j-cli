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

	registerProjectsMock(&helper)

	// Single mock for /v1/graph-analytics/sessions/{id}: first call is GET (pre-flight), second is DELETE.
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{"data": {"id": "`+testSessionID+`"}}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --organization-id %s --project-id %s --rw", testSessionID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOutJson(`{
		"data": {
		  "id": "` + testSessionID + `"
		}
	  }`)
}

func TestDeleteSessionWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{"data": {"id": "`+testSessionID+`"}}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --rw", testSessionID))

	mockHandler.AssertCalledTimes(2)
	helper.AsssertOk()
}

func TestDeleteSessionMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --rw", testSessionID))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestDeleteSessionMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --organization-id %s --rw", testSessionID, testOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestDeleteSessionProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --organization-id %s --project-id unknown-project --rw", testSessionID, testOrgID))

	helper.AssertErr("Error: could not find project unknown-project in organization " + testOrgID)
}

func TestDeleteSessionNotInProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	// Session belongs to a different project.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, "other-project-id"))

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --organization-id %s --project-id %s --rw", testSessionID, testOrgID, testProjectID))

	helper.AssertErr(fmt.Sprintf("Error: could not find session %s in project %s", testSessionID, testProjectID))
}

func TestDeleteSessionError(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	sessionId := "s-f5138f3b-7956"

	registerProjectsMock(&helper)

	// Pre-flight GET succeeds (session in project), then DELETE returns an error.
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", sessionId), http.StatusOK, sessionGetBody(sessionId, testProjectID))
	mockHandler.AddResponse(http.StatusNotFound, `
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

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --organization-id %s --project-id %s --rw", sessionId, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOut("")
	helper.AssertErr("Error: [session with id s-f5138f3b-7956 not found]")
}
