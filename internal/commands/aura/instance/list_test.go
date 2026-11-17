// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/commands/aura/testutils"
	"github.com/stretchr/testify/require"
)

const listInstancesPath = "/v2beta1/organizations/" + testListOrgID + "/projects/" + testListProjectID + "/instances"

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

	mockHandler := helper.NewRequestHandlerMock(listInstancesPath, http.StatusOK, `{
			"data": [
				{
					"id": "2f49c2b3",
					"name": "Production",
					"tenant_id": "YOUR_TENANT_ID",
					"legacy_status": "running",
					"cloud_provider": "gcp"
				},
				{
					"id": "b51dc964",
					"name": "Instance01",
					"tenant_id": "YOUR_TENANT_ID",
					"legacy_status": "running",
					"cloud_provider": "aws"
				},
				{
					"id": "432392ae",
					"name": "Recommendations",
					"tenant_id": "YOUR_TENANT_ID",
					"legacy_status": "running",
					"cloud_provider": "azure"
				},
				{
					"id": "524b7d8d",
					"name": "Northwind",
					"tenant_id": "YOUR_TENANT_ID",
					"legacy_status": "running",
					"cloud_provider": "gcp"
				}
			]
		}`)

	helper.ExecuteCommand("instance list --organization-id " + testListOrgID + " --project-id " + testListProjectID)

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
	  "data": [
		{
		  "cloud_provider": "gcp",
		  "id": "2f49c2b3",
		  "name": "Production",
		  "organization_id": "` + testListOrgID + `",
		  "project_id": "YOUR_TENANT_ID",
		  "status": "running"
		},
		{
		  "cloud_provider": "aws",
		  "id": "b51dc964",
		  "name": "Instance01",
		  "organization_id": "` + testListOrgID + `",
		  "project_id": "YOUR_TENANT_ID",
		  "status": "running"
		},
		{
		  "cloud_provider": "azure",
		  "id": "432392ae",
		  "name": "Recommendations",
		  "organization_id": "` + testListOrgID + `",
		  "project_id": "YOUR_TENANT_ID",
		  "status": "running"
		},
		{
		  "cloud_provider": "gcp",
		  "id": "524b7d8d",
		  "name": "Northwind",
		  "organization_id": "` + testListOrgID + `",
		  "project_id": "YOUR_TENANT_ID",
		  "status": "running"
		}
	  ]
	}`)
}

func TestListInstances_EmptyData(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(listInstancesPath, http.StatusOK, `{"data": []}`)

	helper.ExecuteCommand("instance list --organization-id " + testListOrgID + " --project-id " + testListProjectID)

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{"data": []}`)
}

const threeInstancesBody = `{"data": [
	{"id": "inst0001", "name": "One", "tenant_id": "YOUR_TENANT_ID"},
	{"id": "inst0002", "name": "Two", "tenant_id": "YOUR_TENANT_ID"},
	{"id": "inst0003", "name": "Three", "tenant_id": "YOUR_TENANT_ID"}
]}`

func TestListInstancesLimit(t *testing.T) {
	for _, tc := range []struct {
		name       string
		limit      string
		wantIDs    []string
		wantCut    bool
		wantAbsent string
	}{
		{name: "cuts a longer list", limit: "--limit 2", wantIDs: []string{"inst0001", "inst0002"}, wantCut: true, wantAbsent: "inst0003"},
		{name: "limit equal to total is not cut", limit: "--limit 3", wantIDs: []string{"inst0001", "inst0002", "inst0003"}},
		{name: "limit above total is not cut", limit: "--limit 10", wantIDs: []string{"inst0001", "inst0002", "inst0003"}},
		{name: "zero returns everything", limit: "--limit 0", wantIDs: []string{"inst0001", "inst0002", "inst0003"}},
		{name: "omitted returns everything", limit: "", wantIDs: []string{"inst0001", "inst0002", "inst0003"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			registerProjectsMock(&helper)

			// The v2beta1 list endpoint is unpaginated: one GET carries the whole
			// collection, so --limit trims it locally and never adds a second request.
			mockHandler := helper.NewRequestHandlerMock(listInstancesPath, http.StatusOK, threeInstancesBody)

			helper.ExecuteCommand("instance list --organization-id " + testListOrgID + " --project-id " + testListProjectID + " --format json " + tc.limit)

			mockHandler.AssertCalledTimes(1)
			// The helper's buffers drain on read, so read each stream once.
			stdout, stderr := helper.PrintOut(), helper.PrintErr()
			for _, id := range tc.wantIDs {
				require.Contains(t, stdout, id)
			}
			if tc.wantAbsent != "" {
				require.NotContains(t, stdout, tc.wantAbsent, "results beyond --limit must be trimmed")
			}
			if tc.wantCut {
				require.Contains(t, stderr, "Showing the first 2 instances; more are available")
			} else {
				require.NotContains(t, stderr, "more are available")
			}
		})
	}
}

func TestListInstancesLimitKeepsStdoutValidJSON(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)
	helper.NewRequestHandlerMock(listInstancesPath, http.StatusOK, threeInstancesBody)

	helper.ExecuteCommand("instance list --limit 1 --organization-id " + testListOrgID + " --project-id " + testListProjectID + " --format json")

	var out struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(helper.PrintOut()), &out), "the stderr note must not leak into stdout")
	require.Len(t, out.Data, 1)
}

func TestListInstancesRejectsNegativeLimit(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)
	mockHandler := helper.NewRequestHandlerMock(listInstancesPath, http.StatusOK, threeInstancesBody)

	err := helper.ExecuteCommandE("instance list --limit -1 --organization-id " + testListOrgID + " --project-id " + testListProjectID)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T: %v", err, err)
	require.Equal(t, 2, ce.Code)
	require.Contains(t, ce.Message, "--limit must be zero or greater")
	mockHandler.AssertCalledTimes(0)
}

func TestListInstancesWithProjectIdFlag(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(listInstancesPath, http.StatusOK, `{
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

	helper.AssertOutJson(`{
	  "data": [
		{
		  "cloud_provider": "gcp",
		  "id": "2f49c2b3",
		  "name": "Production",
		  "organization_id": "` + testListOrgID + `",
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

	mockHandler := helper.NewRequestHandlerMock(listInstancesPath, http.StatusOK, `{
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
}

func TestListInstancesMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(listInstancesPath, http.StatusOK, `{"data": []}`)

	helper.ExecuteCommand("instance list")

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestListInstancesMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(listInstancesPath, http.StatusOK, `{"data": []}`)

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

	helper.ExecuteCommand("instance list --organization-id " + testListOrgID + " --project-id unknown-project")

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

			mockHandler := helper.NewRequestHandlerMock(listInstancesPath, http.StatusOK, `{"data": []}`)

			helper.ExecuteCommand(tc.command)

			mockHandler.AssertCalledTimes(1)
			helper.AsssertOk()
		})
	}
}
