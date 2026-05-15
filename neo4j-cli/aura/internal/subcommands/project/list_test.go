// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListProjects(t *testing.T) {
	const orgID = "org-111"

	for _, tc := range []struct {
		name        string
		status      int
		body        string
		wantOutJSON string
		wantErr     string
	}{
		{
			name:   "success returns list of projects",
			status: http.StatusOK,
			body: `{
				"data": [
					{"id": "proj-1", "name": "My Project"},
					{"id": "proj-2", "name": "Another Project"}
				]
			}`,
			wantOutJSON: `{
				"data": [
					{"id": "proj-1", "name": "My Project"},
					{"id": "proj-2", "name": "Another Project"}
				]
			}`,
		},
		{
			name:   "success returns empty list",
			status: http.StatusOK,
			body:   `{"data": []}`,
			wantOutJSON: `{
				"data": []
			}`,
		},
		{
			name:    "API error returns error",
			status:  http.StatusInternalServerError,
			body:    `{"errors": [{"message": "internal server error"}]}`,
			wantErr: "internal server error",
		},
		{
			name:    "404 returns error",
			status:  http.StatusNotFound,
			body:    `{"errors": [{"message": "organization not found"}]}`,
			wantErr: "organization not found",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			helper.SetConfigValue("aura.beta-enabled", true)

			mockHandler := helper.NewRequestHandlerMock(
				fmt.Sprintf("/v2beta1/organizations/%s/projects", orgID),
				tc.status,
				tc.body,
			)

			helper.ExecuteCommand(fmt.Sprintf("project list --organization-id=%s", orgID))

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

func TestListProjectsFromDefaultContext(t *testing.T) {
	const orgID = "org-111"
	const projectID = "proj-999"

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetDefaultProjectInConfig(orgID, projectID)

	mockHandler := helper.NewRequestHandlerMock(
		fmt.Sprintf("/v2beta1/organizations/%s/projects", orgID),
		http.StatusOK,
		`{"data": [{"id": "proj-1", "name": "My Project"}]}`,
	)

	helper.ExecuteCommand("project list")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	helper.AsssertOk()
}

func TestListProjectsWithFlagOverridesContext(t *testing.T) {
	const contextOrgID = "org-from-context"
	const flagOrgID = "org-from-flag"
	const projectID = "proj-999"

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetDefaultProjectInConfig(contextOrgID, projectID)

	mockHandler := helper.NewRequestHandlerMock(
		fmt.Sprintf("/v2beta1/organizations/%s/projects", flagOrgID),
		http.StatusOK,
		`{"data": []}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("project list --organization-id=%s", flagOrgID))

	mockHandler.AssertCalledTimes(1)
	helper.AsssertOk()
}

func TestListProjectsMissingOrganizationId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	helper.ExecuteCommand("project list")

	helper.AssertErrContainsStrings([]string{"organization-id", "aura.default-context"})
}

func TestListProjectsWithInvalidFormat(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("project list --format invalid")

	helper.AssertErr("Error: invalid format value specified: invalid")
}

func TestListProjectsWithCredentialFlag(t *testing.T) {
	const orgID = "org-111"

	for _, tc := range []struct {
		name    string
		command string
	}{
		{name: "--credential flag", command: fmt.Sprintf("project list --organization-id=%s --credential named-cred", orgID)},
		{name: "-c shorthand", command: fmt.Sprintf("project list --organization-id=%s -c named-cred", orgID)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			helper.SetCredentialsValue("aura", map[string]interface{}{
				"credentials": []map[string]interface{}{
					{
						"name":          "named-cred",
						"client-id":     "named-client-id",
						"client-secret": "named-client-secret",
						"access-token":  "",
						"token-expiry":  0,
					},
				},
				"default-credential": "",
			})

			helper.SetConfigValue("aura.beta-enabled", true)

			mockHandler := helper.NewRequestHandlerMock(
				fmt.Sprintf("/v2beta1/organizations/%s/projects", orgID),
				http.StatusOK,
				`{"data": []}`,
			)

			helper.ExecuteCommand(tc.command)

			mockHandler.AssertCalledTimes(1)
			helper.AsssertOk()
		})
	}
}
