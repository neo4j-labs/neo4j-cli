// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/stretchr/testify/require"
)

func scopedSessionPath(sessionID string) string {
	return fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/graph-analytics/sessions/%s", testOrgID, testProjectID, sessionID)
}

func TestGetSession(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(scopedSessionPath(testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))

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

	mockHandler := helper.NewRequestHandlerMock(scopedSessionPath(testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))

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

func TestGetSessionWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	sessionId := "559c94c7-15de43fg"

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(scopedSessionPath(sessionId), http.StatusOK, sessionGetBody(sessionId, testProjectID))

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session get %s\"\n\" --organization-id %s --project-id %s", sessionId, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
}

// TestGetSessionNotFound verifies that a 404 on the v2beta1 scoped session
// path surfaces as a not-found error tagged with resource type "session" by
// the API layer's parseResourceFromRequest (which resolves the trailing
// plural/id pair of the nested path).
func TestGetSessionNotFound(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	helper.NewRequestHandlerMock(scopedSessionPath(testSessionID), http.StatusNotFound, `{
		"errors": [
			{"message": "session not found", "reason": "not-found"}
		]
	}`)

	err := helper.ExecuteCommandE(fmt.Sprintf("graph-analytics session get %s --organization-id %s --project-id %s", testSessionID, testOrgID, testProjectID))

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T: %v", err, err)
	require.Equal(t, 3, ce.Code)
	require.Equal(t, "session", ce.ResourceType)
	require.Equal(t, testSessionID, ce.ResourceID)
	require.Equal(t, "Run 'neo4j-cli aura graph-analytics session list --project-id <id>' to see sessions in this project.", ce.Suggestion)
}

func TestGetSessionError(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	sessionId := "s-f5138f3b-7956"

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(scopedSessionPath(sessionId), http.StatusNotFound, `
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
	helper.AssertErr(`Error: [
	session with id s-f5138f3b-7956 not found
]`)
}
