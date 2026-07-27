// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListSessions(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusOK, `{ "data": [
					{
					  "id": "s-04de43fe-67ab-4",
					  "name": "people-and-fruits",
					  "memory": "8GB",
					  "instance_id": null,
					  "status": "Ready",
					  "created_at": "2025-04-04T09:32:35Z",
					  "host": "s-04de43fe-67ab-4-gds.ORCHESTRA.neo4j.io",
					  "expiry_date": "2025-04-11T09:32:35Z",
					  "ttl": "20m0s",
					  "user_id": "YOUR_USER_ID",
					  "tenant_id": "`+testProjectID+`",
					  "cloud_provider": "azure",
					  "region": "francecentral"
					},
					{
					  "id": "559c94c7-15de43fg",
					  "name": "people-and-fruits-with-db",
					  "memory": "4GB",
					  "instance_id": "559c94c7",
					  "status": "Creating",
					  "created_at": "2025-04-04T09:32:35Z",
					  "host": "559c94c7-15de43fg.ORCHESTRA.neo4j.io",
					  "expiry_date": null,
					  "ttl": null,
					  "user_id": "YOUR_USER_ID",
					  "tenant_id": "`+testProjectID+`",
					  "cloud_provider": "gcp",
					  "region": "europe-west1"
					}
			]
		}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session list --organization-id %s --project-id %s", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
	"data": [
		{
			"cloud_provider": "azure",
			"created_at": "2025-04-04T09:32:35Z",
			"expiry_date": "2025-04-11T09:32:35Z",
			"host": "s-04de43fe-67ab-4-gds.ORCHESTRA.neo4j.io",
			"id": "s-04de43fe-67ab-4",
			"instance_id": null,
			"memory": "8GB",
			"name": "people-and-fruits",
			"project_id": "YOUR_PROJECT_ID",
			"region": "francecentral",
			"status": "Ready",
			"ttl": "20m0s",
			"user_id": "YOUR_USER_ID"
		},
		{
			"cloud_provider": "gcp",
			"created_at": "2025-04-04T09:32:35Z",
			"expiry_date": null,
			"host": "559c94c7-15de43fg.ORCHESTRA.neo4j.io",
			"id": "559c94c7-15de43fg",
			"instance_id": "559c94c7",
			"memory": "4GB",
			"name": "people-and-fruits-with-db",
			"project_id": "YOUR_PROJECT_ID",
			"region": "europe-west1",
			"status": "Creating",
			"ttl": null,
			"user_id": "YOUR_USER_ID"
		}
	]
}`)
}

func TestListSessionsWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusOK, `{"data": []}`)

	helper.ExecuteCommand("graph-analytics session list")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{"data": []}`)
}

func TestListSessionsWithInstanceFilter(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusOK, `{"data": []}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session list --organization-id %s --project-id %s --instance-id my-instance-id", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	mockHandler.AssertCalledWithQueryParam("instanceId", "my-instance-id")

	helper.AssertOutJson(`{"data": []}`)
}

func TestListSessionsMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusOK, `{"data": []}`)

	helper.ExecuteCommand("graph-analytics session list")

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestListSessionsMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusOK, `{"data": []}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session list --organization-id %s", testOrgID))

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestListSessionsProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusOK, `{"data": []}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session list --organization-id %s --project-id unknown-project", testOrgID))

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: could not find project unknown-project in organization " + testOrgID)
}

func TestListSessionsWithInvalidOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("graph-analytics session list --format invalid")

	helper.AssertErr("Error: invalid format value specified: invalid")
}

func TestListSessionsWithCredentialFlag(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
	}{
		{name: "--credential flag", command: fmt.Sprintf("graph-analytics session list --organization-id %s --project-id %s --credential named-cred", testOrgID, testProjectID)},
		{name: "-c shorthand", command: fmt.Sprintf("graph-analytics session list --organization-id %s --project-id %s -c named-cred", testOrgID, testProjectID)},
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

			helper.NewRequestHandlerMock(
				"/v2beta1/organizations/"+testOrgID+"/projects",
				http.StatusOK,
				`{"data": [{"id": "`+testProjectID+`", "name": "Test Project"}]}`,
			)

			mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusOK, `{"data": []}`)

			helper.ExecuteCommand(tc.command)

			mockHandler.AssertCalledTimes(1)
			helper.AsssertOk()
		})
	}
}
