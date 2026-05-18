// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

const (
	testProjectID = "proj-222"
	testOrgID     = "org-111"
)

func listProjectsBody(projectIDs ...string) string {
	items := ""
	for i, id := range projectIDs {
		if i > 0 {
			items += ","
		}
		items += fmt.Sprintf(`{"id":%q,"name":"Project %s"}`, id, id)
	}
	return `{"data":[` + items + `]}`
}

func TestGetProject(t *testing.T) {
	for _, tc := range []struct {
		name        string
		listStatus  int
		listBody    string
		wantOutJSON string
		wantErr     string
	}{
		{
			name:       "success returns project details",
			listStatus: http.StatusOK,
			listBody:   listProjectsBody(testProjectID),
			wantOutJSON: `{
				"data": {"id": "proj-222", "name": "My Project"}
			}`,
		},
		{
			name:       "project not in org returns error",
			listStatus: http.StatusOK,
			listBody:   listProjectsBody("other-proj"),
			wantErr:    fmt.Sprintf("project %s not found in organization %s", testProjectID, testOrgID),
		},
		{
			name:       "list projects API error returns error",
			listStatus: http.StatusInternalServerError,
			listBody:   `{"errors": [{"message": "internal server error"}]}`,
			wantErr:    "internal server error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			helper.NewRequestHandlerMock(
				fmt.Sprintf("/v2beta1/organizations/%s/projects", testOrgID),
				tc.listStatus,
				tc.listBody,
			)
			// v1 handler present for the success case; ignored by error cases that return early.
			helper.NewRequestHandlerMock(
				fmt.Sprintf("/v1/tenants/%s", testProjectID),
				http.StatusOK,
				`{"data": {"id": "proj-222", "name": "My Project"}}`,
			)

			helper.ExecuteCommand(fmt.Sprintf("project get %s --organization-id=%s", testProjectID, testOrgID))

			if tc.wantErr != "" {
				helper.AssertErrContainsStrings([]string{tc.wantErr})
				return
			}

			helper.AssertOutJson(tc.wantOutJSON)
		})
	}
}

func TestGetProjectV1Error(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		fmt.Sprintf("/v2beta1/organizations/%s/projects", testOrgID),
		http.StatusOK,
		listProjectsBody(testProjectID),
	)
	helper.NewRequestHandlerMock(
		fmt.Sprintf("/v1/tenants/%s", testProjectID),
		http.StatusNotFound,
		`{"errors": [{"message": "project not found"}]}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("project get %s --organization-id=%s", testProjectID, testOrgID))

	helper.AssertErrContainsStrings([]string{"project not found"})
}

func TestGetProjectFromWorkspaceConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)

	listHandler := helper.NewRequestHandlerMock(
		fmt.Sprintf("/v2beta1/organizations/%s/projects", testOrgID),
		http.StatusOK,
		listProjectsBody(testProjectID),
	)
	getHandler := helper.NewRequestHandlerMock(
		fmt.Sprintf("/v1/tenants/%s", testProjectID),
		http.StatusOK,
		`{"data": {"id": "proj-222", "name": "My Project"}}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("project get %s", testProjectID))

	listHandler.AssertCalledTimes(1)
	getHandler.AssertCalledTimes(1)
	helper.AsssertOk()
}

func TestGetProjectWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	listHandler := helper.NewRequestHandlerMock(
		fmt.Sprintf("/v2beta1/organizations/%s/projects", testOrgID),
		http.StatusOK,
		listProjectsBody(testProjectID),
	)
	getHandler := helper.NewRequestHandlerMock(
		fmt.Sprintf("/v1/tenants/%s", testProjectID),
		http.StatusOK,
		`{"data": {"id": "proj-222", "name": "My Project"}}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("project get %s\"\n\" --organization-id=%s", testProjectID, testOrgID))

	listHandler.AssertCalledTimes(1)
	getHandler.AssertCalledTimes(1)
	helper.AsssertOk()
}

func TestGetProjectMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("project get %s", testProjectID))

	helper.AssertErrContainsStrings([]string{"organization-id", "aura.default-workspace"})
}

func TestGetProjectMissingPositionalArg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("project get")

	helper.AssertErrContainsStrings([]string{"accepts 1 arg(s)"})
}
