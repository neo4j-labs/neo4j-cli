// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package context_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestContextUseWithSlug(t *testing.T) {
	const orgID = "org-111"
	const projectID = "proj-222"
	const slug = orgID + "/" + projectID

	for _, tc := range []struct {
		name        string
		status      int
		body        string
		wantContext string
		wantErr     string
	}{
		{
			name:        "success with positional slug",
			status:      http.StatusOK,
			body:        fmt.Sprintf(`{"data": {"id": "%s", "name": "My Project"}}`, projectID),
			wantContext: slug,
		},
		{
			name:    "404 from API returns error without persisting",
			status:  http.StatusNotFound,
			body:    `{"errors": [{"message": "project not found"}]}`,
			wantErr: "project not found",
		},
		{
			name:    "500 from API returns error without persisting",
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

			helper.ExecuteCommand(fmt.Sprintf("context use %s --rw", slug))

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodGet)

			if tc.wantErr != "" {
				helper.AssertErrContainsStrings([]string{tc.wantErr})
				return
			}

			helper.AsssertOk()
			helper.AssertConfigValue("aura.default-context", tc.wantContext)
		})
	}
}

func TestContextUseWithFlags(t *testing.T) {
	const orgID = "org-111"
	const projectID = "proj-222"
	const slug = orgID + "/" + projectID

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	mockHandler := helper.NewRequestHandlerMock(
		fmt.Sprintf("/v2beta1/organizations/%s/projects/%s", orgID, projectID),
		http.StatusOK,
		fmt.Sprintf(`{"data": {"id": "%s", "name": "My Project"}}`, projectID),
	)

	helper.ExecuteCommand(fmt.Sprintf("context use --organization-id=%s --project-id=%s --rw", orgID, projectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	helper.AsssertOk()
	helper.AssertConfigValue("aura.default-context", slug)
}

func TestContextUseMixedFormError(t *testing.T) {
	const orgID = "org-111"
	const projectID = "proj-222"
	const slug = orgID + "/" + projectID

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("context use %s --organization-id=%s --rw", slug, orgID))

	helper.AssertErrContainsStrings([]string{"mutually exclusive"})
}

func TestContextUseMissingOrganizationId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("context use --project-id=proj-222 --rw")

	helper.AssertErrContainsStrings([]string{"organization-id"})
}

func TestContextUseMissingProjectId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("context use --organization-id=org-111 --rw")

	helper.AssertErrContainsStrings([]string{"project-id"})
}

func TestContextUseInvalidSlugNoSlash(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("context use noslash --rw")

	helper.AssertErrContainsStrings([]string{"expected format {organizationId}/{projectId}"})
}

func TestContextUseEmptyOrgInSlug(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("context use /proj-222 --rw")

	helper.AssertErrContainsStrings([]string{"organization ID must not be empty"})
}

func TestContextUseEmptyProjectInSlug(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("context use org-111/ --rw")

	helper.AssertErrContainsStrings([]string{"project ID must not be empty"})
}
