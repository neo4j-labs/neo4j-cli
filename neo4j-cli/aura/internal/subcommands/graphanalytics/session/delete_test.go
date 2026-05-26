// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/stretchr/testify/require"
)

func TestDeleteSession(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	// Single mock for /v1/graph-analytics/sessions/{id}: first call is GET (pre-flight), second is DELETE.
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{"data": {"id": "`+testSessionID+`"}}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --organization-id %s --project-id %s --rw --yes --force", testSessionID, testOrgID, testProjectID))

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

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --rw --yes --force", testSessionID))

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
	helper.AssertUsageNotShown()
}

func TestDeleteSessionWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	sessionId := "42-24"

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", sessionId), http.StatusOK, sessionGetBody(sessionId, testProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{
		"data": {
		  "id": "42-24"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s\"\n\" --organization-id %s --project-id %s --rw --yes --force", sessionId, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
}

// TestDeleteSessionNotFound_HasSuggestion locks the WithNotFoundContext
// rewrite at the session-DELETE call site: when the API returns 404 for
// the DELETE call (after the preflight succeeds), the API layer's
// parseResourceFromRequest mis-segments the nested path; the caller
// rewrites ResourceType to "graph-analytics-session" and attaches the
// session-list Suggestion (REQ-F-013).
func TestDeleteSessionNotFound_HasSuggestion(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	// Pre-flight GET succeeds (session in project), then DELETE returns 404.
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))
	mockHandler.AddResponse(http.StatusNotFound, `{
		"errors": [
			{"message": "session not found", "reason": "not-found"}
		]
	}`)

	err := helper.ExecuteCommandE(fmt.Sprintf("graph-analytics session delete %s --organization-id %s --project-id %s --rw --yes --force", testSessionID, testOrgID, testProjectID))

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T: %v", err, err)
	require.Equal(t, 3, ce.Code)
	require.Equal(t, "graph-analytics-session", ce.ResourceType)
	require.Equal(t, testSessionID, ce.ResourceID)
	require.Equal(t, "Run 'neo4j-cli aura graph-analytics session list --project-id <id>' to see sessions in this project.", ce.Suggestion)
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

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --organization-id %s --project-id %s --rw --yes --force", sessionId, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOut("")
	helper.AssertErr(`Error: [
	session with id s-f5138f3b-7956 not found
]`)
}

func TestDeleteSessionConfirmGate(t *testing.T) {
	base := fmt.Sprintf("graph-analytics session delete %s --organization-id %s --project-id %s --rw", testSessionID, testOrgID, testProjectID)
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "aura graph-analytics session delete",
		NoFlagsArgs:   base,
		BothFlagsArgs: base + " --yes --force",
		ResourceLabel: "session",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			helper := testutils.NewAuraTestHelper(t)
			t.Cleanup(helper.Close)
			registerProjectsMock(&helper)
			mock := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))
			mock.AddResponse(http.StatusAccepted, `{"data": {"id": "`+testSessionID+`"}}`)
			helper.SetStdin(stdin)
			err := helper.ExecuteCommandE(args)
			return confirmtest.GateRunResult{Err: err, Stderr: helper.PrintErr(), Invoked: mock.CalledWithMethod(http.MethodDelete)}
		},
	})
}
