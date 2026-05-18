// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package organization_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetOrganization(t *testing.T) {
	const orgID = "org-111"

	for _, tc := range []struct {
		name        string
		status      int
		body        string
		wantOutJSON string
		wantErr     string
	}{
		{
			name:   "success returns organization details",
			status: http.StatusOK,
			body:   `{"data": {"id": "org-111", "name": "Production Org"}}`,
			wantOutJSON: `{
				"data": {"id": "org-111", "name": "Production Org"}
			}`,
		},
		{
			name:    "404 returns error",
			status:  http.StatusNotFound,
			body:    `{"errors": [{"message": "organization not found"}]}`,
			wantErr: "organization not found",
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

			helper.SetConfigValue("flag.aura-beta", true)

			mockHandler := helper.NewRequestHandlerMock(
				fmt.Sprintf("/v2beta1/organizations/%s", orgID),
				tc.status,
				tc.body,
			)

			helper.ExecuteCommand(fmt.Sprintf("organization get %s", orgID))

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

func TestGetOrganizationWithTrailingNewline(t *testing.T) {
	const orgID = "org-111"

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("flag.aura-beta", true)

	mockHandler := helper.NewRequestHandlerMock(
		fmt.Sprintf("/v2beta1/organizations/%s", orgID),
		http.StatusOK,
		`{"data": {"id": "org-111", "name": "Production Org"}}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("organization get %s\"\n\"", orgID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
}

func TestGetOrganizationMissingPositionalArg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("flag.aura-beta", true)

	helper.ExecuteCommand("organization get")

	helper.AssertErrContainsStrings([]string{"accepts 1 arg(s)"})
}
