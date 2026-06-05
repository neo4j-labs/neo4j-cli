// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package workspace_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestWorkspaceList(t *testing.T) {
	const org1ID = "org-aaa"
	const org2ID = "org-bbb"
	const proj1ID = "proj-111"
	const proj2ID = "proj-222"
	const proj3ID = "proj-333"

	for _, tc := range []struct {
		name             string
		defaultWorkspace string
		orgsBody         string
		proj1Body        string
		proj2Body        string
		wantOutJSON      string
		wantErr          string
	}{
		{
			name:             "two orgs with projects, default matches first entry",
			defaultWorkspace: fmt.Sprintf("%s/%s", org1ID, proj1ID),
			orgsBody: fmt.Sprintf(`{
				"data": [
					{"id": "%s", "name": "Org Alpha"},
					{"id": "%s", "name": "Org Beta"}
				]
			}`, org1ID, org2ID),
			proj1Body: fmt.Sprintf(`{
				"data": [
					{"id": "%s", "name": "Project One"}
				]
			}`, proj1ID),
			proj2Body: fmt.Sprintf(`{
				"data": [
					{"id": "%s", "name": "Project Two"},
					{"id": "%s", "name": "Project Three"}
				]
			}`, proj2ID, proj3ID),
			wantOutJSON: fmt.Sprintf(`{
				"data": [
					{
						"workspace": "%s/%s",
						"organization_id": "%s",
						"project_id": "%s",
						"project_name": "Project One",
						"default": true
					},
					{
						"workspace": "%s/%s",
						"organization_id": "%s",
						"project_id": "%s",
						"project_name": "Project Two",
						"default": false
					},
					{
						"workspace": "%s/%s",
						"organization_id": "%s",
						"project_id": "%s",
						"project_name": "Project Three",
						"default": false
					}
				]
			}`, org1ID, proj1ID, org1ID, proj1ID,
				org2ID, proj2ID, org2ID, proj2ID,
				org2ID, proj3ID, org2ID, proj3ID),
		},
		{
			name:             "no default workspace set — all entries have default=false",
			defaultWorkspace: "",
			orgsBody: fmt.Sprintf(`{
				"data": [
					{"id": "%s", "name": "Org Alpha"}
				]
			}`, org1ID),
			proj1Body: fmt.Sprintf(`{
				"data": [
					{"id": "%s", "name": "Project One"}
				]
			}`, proj1ID),
			wantOutJSON: fmt.Sprintf(`{
				"data": [
					{
						"workspace": "%s/%s",
						"organization_id": "%s",
						"project_id": "%s",
						"project_name": "Project One",
						"default": false
					}
				]
			}`, org1ID, proj1ID, org1ID, proj1ID),
		},
		{
			name:             "empty org list returns empty data array",
			defaultWorkspace: "",
			orgsBody:         `{"data": []}`,
			wantOutJSON:      `{"data": []}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			helper.SetConfigValue("flag.aura-beta", true)
			if tc.defaultWorkspace != "" {
				helper.SetConfigValue("aura.default-workspace", tc.defaultWorkspace)
			}

			orgsHandler := helper.NewRequestHandlerMock("/v2beta1/organizations", http.StatusOK, tc.orgsBody)

			if tc.proj1Body != "" {
				helper.NewRequestHandlerMock(
					fmt.Sprintf("/v2beta1/organizations/%s/projects", org1ID),
					http.StatusOK,
					tc.proj1Body,
				)
			}
			if tc.proj2Body != "" {
				helper.NewRequestHandlerMock(
					fmt.Sprintf("/v2beta1/organizations/%s/projects", org2ID),
					http.StatusOK,
					tc.proj2Body,
				)
			}

			helper.ExecuteCommand("workspace list --format json")

			orgsHandler.AssertCalledTimes(1)
			orgsHandler.AssertCalledWithMethod(http.MethodGet)

			if tc.wantErr != "" {
				helper.AssertErrContainsStrings([]string{tc.wantErr})
				return
			}

			helper.AssertOutJson(tc.wantOutJSON)
		})
	}
}

func TestWorkspaceListAPIError(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("flag.aura-beta", true)

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations",
		http.StatusInternalServerError,
		`{"errors": [{"message": "internal server error"}]}`,
	)

	helper.ExecuteCommand("workspace list")

	helper.AssertErrContainsStrings([]string{"failed to list organizations", "internal server error"})
}

func TestWorkspaceListProjectsAPIError(t *testing.T) {
	const orgID = "org-aaa"

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("flag.aura-beta", true)

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations",
		http.StatusOK,
		fmt.Sprintf(`{"data": [{"id": "%s", "name": "Org Alpha"}]}`, orgID),
	)
	helper.NewRequestHandlerMock(
		fmt.Sprintf("/v2beta1/organizations/%s/projects", orgID),
		http.StatusInternalServerError,
		`{"errors": [{"message": "projects fetch failed"}]}`,
	)

	helper.ExecuteCommand("workspace list")

	helper.AssertErrContainsStrings([]string{"failed to list projects", "projects fetch failed"})
}

func TestWorkspaceListWithInvalidFormat(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("workspace list --format invalid")

	helper.AssertErr("Error: invalid format value specified: invalid")
}
