// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
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

func TestDeleteSessionConfirmGate_NonTTYWithoutFlags_Exit2(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))

	err := helper.ExecuteCommandE(fmt.Sprintf("graph-analytics session delete %s --organization-id %s --project-id %s --rw", testSessionID, testOrgID, testProjectID))

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("error %q missing 'pass both --yes and --force'", err.Error())
	}
	mockHandler.AssertCalledTimes(1)
}

func TestDeleteSessionConfirmGate_NonTTYWithBothFlags_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{"data": {"id": "`+testSessionID+`"}}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --organization-id %s --project-id %s --rw --yes --force", testSessionID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
}

func TestDeleteSessionConfirmGate_TTYAnswerY_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetStdin("y\n")
	registerProjectsMock(&helper)
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{"data": {"id": "`+testSessionID+`"}}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session delete %s --organization-id %s --project-id %s --rw", testSessionID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
	helper.AssertErrContainsStrings([]string{"Delete session"})
}

func TestDeleteSessionConfirmGate_TTYAnswerN_Cancels(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetStdin("N\n")
	registerProjectsMock(&helper)
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/graph-analytics/sessions/%s", testSessionID), http.StatusOK, sessionGetBody(testSessionID, testProjectID))

	err := helper.ExecuteCommandE(fmt.Sprintf("graph-analytics session delete %s --organization-id %s --project-id %s --rw", testSessionID, testOrgID, testProjectID))

	if err != nil {
		t.Fatalf("expected nil on cancel, got %v", err)
	}
	mockHandler.AssertCalledTimes(1)
	helper.AssertErrContainsStrings([]string{"cancelled."})
}
