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
	const orgID = "org-111"
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

			helper.SetConfigValue("aura.beta-enabled", true)

			mockHandler := helper.NewRequestHandlerMock(
				fmt.Sprintf("/v2beta1/organizations/%s/projects/%s", orgID, projectID),
				tc.status,
				tc.body,
			)

			helper.ExecuteCommand(fmt.Sprintf("project get %s --organization-id=%s", projectID, orgID))

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

func TestGetProjectFromDefaultContext(t *testing.T) {
	const orgID = "org-111"
	const defaultProjectID = "proj-default"
	const getProjectID = "proj-222"

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetDefaultProjectInConfig(orgID, defaultProjectID)

	mockHandler := helper.NewRequestHandlerMock(
		fmt.Sprintf("/v2beta1/organizations/%s/projects/%s", orgID, getProjectID),
		http.StatusOK,
		`{"data": {"id": "proj-222", "name": "My Project"}}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("project get %s", getProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	helper.AsssertOk()
}

func TestGetProjectWithFlagOverridesContext(t *testing.T) {
	const contextOrgID = "org-from-context"
	const flagOrgID = "org-from-flag"
	const defaultProjectID = "proj-default"
	const getProjectID = "proj-222"

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetDefaultProjectInConfig(contextOrgID, defaultProjectID)

	mockHandler := helper.NewRequestHandlerMock(
		fmt.Sprintf("/v2beta1/organizations/%s/projects/%s", flagOrgID, getProjectID),
		http.StatusOK,
		`{"data": {"id": "proj-222", "name": "My Project"}}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("project get %s --organization-id=%s", getProjectID, flagOrgID))

	mockHandler.AssertCalledTimes(1)
	helper.AsssertOk()
}

func TestGetProjectMissingOrganizationId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	helper.ExecuteCommand("project get proj-222")

	helper.AssertErrContainsStrings([]string{"organization-id", "aura.default-context"})
}

func TestGetProjectMissingPositionalArg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	helper.ExecuteCommand("project get --organization-id=org-111")

	helper.AssertErrContainsStrings([]string{"accepts 1 arg(s)"})
}
