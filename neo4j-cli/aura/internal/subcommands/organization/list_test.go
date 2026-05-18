// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package organization_test

import (
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListOrganizations(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		body        string
		wantOutJSON string
		wantErr     string
	}{
		{
			name:   "success returns list of organizations",
			status: http.StatusOK,
			body: `{
				"data": [
					{"id": "org-1", "name": "Production Org"},
					{"id": "org-2", "name": "Staging Org"}
				]
			}`,
			wantOutJSON: `{
				"data": [
					{"id": "org-1", "name": "Production Org"},
					{"id": "org-2", "name": "Staging Org"}
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			helper.SetConfigValue("flag.aura-beta", true)

			mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations", tc.status, tc.body)

			helper.ExecuteCommand("organization list")

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

func TestListOrganizationsWithInvalidFormat(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("organization list --format invalid")

	helper.AssertErr("Error: invalid format value specified: invalid")
}

func TestListOrganizationsWithCredentialFlag(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
	}{
		{name: "--credential flag", command: "organization list --credential named-cred"},
		{name: "-c shorthand", command: "organization list -c named-cred"},
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

			helper.SetConfigValue("flag.aura-beta", true)

			mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations", http.StatusOK, `{"data": []}`)

			helper.ExecuteCommand(tc.command)

			mockHandler.AssertCalledTimes(1)
			helper.AsssertOk()
		})
	}
}
