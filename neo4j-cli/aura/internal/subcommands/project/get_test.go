// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetProject(t *testing.T) {
	const projectID = "proj-222"

	for _, tc := range []struct {
		name        string
		status      int
		body        string
		wantOutJSON string
		wantErr     string
	}{
		{
			name:   "success returns project details",
			status: http.StatusOK,
			body:   `{"data": {"id": "proj-222", "name": "My Project"}}`,
			wantOutJSON: `{
				"data": {"id": "proj-222", "name": "My Project"}
			}`,
		},
		{
			name:    "404 returns error",
			status:  http.StatusNotFound,
			body:    `{"errors": [{"message": "project not found"}]}`,
			wantErr: "project not found",
		},
		{
			name:    "API error returns error",
			status:  http.StatusInternalServerError,
			body:    `{"errors": [{"message": "internal server error"}]}`,
			wantErr: "internal server error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			mockHandler := helper.NewRequestHandlerMock(
				fmt.Sprintf("/v1/tenants/%s", projectID),
				tc.status,
				tc.body,
			)

			helper.ExecuteCommand(fmt.Sprintf("project get %s", projectID))

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodGet)

			if tc.wantErr != "" {
				helper.AssertErrContainsStrings([]string{tc.wantErr})
				return
			}

			helper.AssertOutJson(tc.wantOutJSON)
		})
	}
}

func TestGetProjectOrganizationIdFlagAcceptedButNotUsed(t *testing.T) {
	const projectID = "proj-222"
	const orgID = "org-111"

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	// Only mock the v1 tenants endpoint — passing --organization-id must not change the API call
	mockHandler := helper.NewRequestHandlerMock(
		fmt.Sprintf("/v1/tenants/%s", projectID),
		http.StatusOK,
		`{"data": {"id": "proj-222", "name": "My Project"}}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("project get %s --organization-id=%s", projectID, orgID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	helper.AsssertOk()
}

func TestGetProjectMissingPositionalArg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("project get")

	helper.AssertErrContainsStrings([]string{"accepts 1 arg(s)"})
}
