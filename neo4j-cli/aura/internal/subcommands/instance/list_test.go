// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance_test

import (
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

const (
	testListOrgID     = "test-org-id"
	testListProjectID = "YOUR_TENANT_ID"
)

// registerProjectsMock registers a mock for the v2beta1 list-projects endpoint
// that returns testListProjectID as a valid project.
func registerProjectsMock(helper *testutils.AuraTestHelper) {
	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testListOrgID+"/projects",
		http.StatusOK,
		`{"data": [{"id": "`+testListProjectID+`", "name": "Test Project"}]}`,
	)
}

func TestListInstances(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, `{
			"data": [
				{
					"id": "2f49c2b3",
					"name": "Production",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "gcp"
				},
				{
					"id": "b51dc964",
					"name": "Instance01",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "aws"
				},
				{
					"id": "432392ae",
					"name": "Recommendations",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "azure"
				},
				{
					"id": "524b7d8d",
					"name": "Northwind",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "gcp"
				}
			]
		}`)

	helper.ExecuteCommand("instance list --organization-id " + testListOrgID + " --project-id " + testListProjectID)

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	mockHandler.AssertCalledWithQueryParam("tenantId", testListProjectID)

	helper.AssertOutJson(`{
	  "data": [
		{
		  "cloud_provider": "gcp",
		  "id": "2f49c2b3",
		  "name": "Production",
		  "project_id": "YOUR_TENANT_ID"
		},
		{
		  "cloud_provider": "aws",
		  "id": "b51dc964",
		  "name": "Instance01",
		  "project_id": "YOUR_TENANT_ID"
		},
		{
		  "cloud_provider": "azure",
		  "id": "432392ae",
		  "name": "Recommendations",
		  "project_id": "YOUR_TENANT_ID"
		},
		{
		  "cloud_provider": "gcp",
		  "id": "524b7d8d",
		  "name": "Northwind",
		  "project_id": "YOUR_TENANT_ID"
		}
	  ]
	}`)
}

func TestListInstancesWithProjectIdFlag(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, `{
			"data": [
				{
					"id": "2f49c2b3",
					"name": "Production",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "gcp"
				}
			]
		}`)

	helper.ExecuteCommand("instance list --organization-id " + testListOrgID + " --project-id " + testListProjectID)

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	mockHandler.AssertCalledWithQueryParam("tenantId", testListProjectID)

	helper.AssertOutJson(`{
	  "data": [
		{
		  "cloud_provider": "gcp",
		  "id": "2f49c2b3",
		  "name": "Production",
		  "project_id": "YOUR_TENANT_ID"
		}
	  ]
	}`)
}

func TestListInstancesWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testListOrgID, testListProjectID)

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, `{
			"data": [
				{
					"id": "2f49c2b3",
					"name": "Production",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "gcp"
				}
			]
		}`)

	helper.ExecuteCommand("instance list")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	mockHandler.AssertCalledWithQueryParam("tenantId", testListProjectID)
}

func TestListInstancesMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, `{"data": []}`)

	helper.ExecuteCommand("instance list")

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestListInstancesMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, `{"data": []}`)

	helper.ExecuteCommand("instance list --organization-id " + testListOrgID)

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestListInstancesProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testListOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, `{"data": []}`)

	helper.ExecuteCommand("instance list --organization-id " + testListOrgID + " --project-id unknown-project")

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: could not find project unknown-project in organization " + testListOrgID)
}

func TestListCustomerManagedKeysWithInvalidOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("instance list --format invalid")

	helper.AssertErr("Error: invalid format value specified: invalid")
}

func TestListInstancesWithCredentialFlag(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
	}{
		{name: "--credential flag", command: "instance list --organization-id " + testListOrgID + " --project-id " + testListProjectID + " --credential named-cred"},
		{name: "-c shorthand", command: "instance list --organization-id " + testListOrgID + " --project-id " + testListProjectID + " -c named-cred"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			// Replace the default credential with a named credential (no default set).
			// If --credential resolution falls back to GetDefault, the command would fail
			// because there is no default. Success means the named credential was used.
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

			helper.NewRequestHandlerMock(
				"/v2beta1/organizations/"+testListOrgID+"/projects",
				http.StatusOK,
				`{"data": [{"id": "`+testListProjectID+`", "name": "Test Project"}]}`,
			)

			mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, `{"data": []}`)

			helper.ExecuteCommand(tc.command)

			mockHandler.AssertCalledTimes(1)
			helper.AsssertOk()
		})
	}
}
