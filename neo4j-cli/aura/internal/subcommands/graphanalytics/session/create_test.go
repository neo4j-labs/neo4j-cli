// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestCreateAttachedSession(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusAccepted, `{
  "data": {
    "id": "559c94c7-15de43fg",
    "name": "people-and-fruits-with-db",
    "memory": "4GB",
    "instance_id": "559c94c7",
    "status": "",
    "created_at": "2025-04-04T09:32:35Z",
    "host": "559c94c7-15de43fg.ORCHESTRA.neo4j.io",
    "expiry_date": "2025-04-11T09:32:35Z",
    "tenant_id": "`+testProjectID+`",
    "ttl": "8m",
    "user_id": "YOUR_USER_ID",
    "cloud_provider": "gcp",
    "region": "europe-west1"
  }
}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session create --name session1 --memory 4GB --instance-id 559c94c7 --organization-id %s --project-id %s --rw", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(fmt.Sprintf(`{"instance_id":"559c94c7","memory":"4GB","name":"session1","tenant_id":"%s"}`, testProjectID))

	helper.AssertErr("")
	helper.AssertOutJson(`{
  "data": {
	"cloud_provider": "gcp",
    "created_at": "2025-04-04T09:32:35Z",
    "expiry_date": "2025-04-11T09:32:35Z",
    "host": "559c94c7-15de43fg.ORCHESTRA.neo4j.io",
    "id": "559c94c7-15de43fg",
    "instance_id": "559c94c7",
    "memory": "4GB",
    "name": "people-and-fruits-with-db",
    "project_id": "YOUR_PROJECT_ID",
    "region": "europe-west1",
    "status": "",
    "ttl": "8m",
    "user_id": "YOUR_USER_ID"
  }
}`)
}

func TestCreateStandAloneSession(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusAccepted, `{
  "data": {
    "id": "s-15de43fg",
    "name": "people-and-fruits-with-db",
    "memory": "4GB",
    "instance_id": "",
    "status": "",
    "created_at": "2025-04-04T09:32:35Z",
    "host": "s-15de43fg.ORCHESTRA.neo4j.io",
    "expiry_date": "2025-04-11T09:32:35Z",
    "ttl": "8m",
    "user_id": "YOUR_USER_ID",
    "tenant_id": "`+testProjectID+`",
    "cloud_provider": "gcp",
    "region": "europe-west1"
  }
}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session create --name session1 --memory 4GB --region europe-west1 --cloud-provider gcp --organization-id %s --project-id %s --rw", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(fmt.Sprintf(`{"cloud_provider":"gcp","memory":"4GB","name":"session1","region":"europe-west1","tenant_id":"%s"}`, testProjectID))

	helper.AssertOutJson(`{
  "data": {
	"cloud_provider": "gcp",
    "created_at": "2025-04-04T09:32:35Z",
    "expiry_date": "2025-04-11T09:32:35Z",
    "host": "s-15de43fg.ORCHESTRA.neo4j.io",
    "id": "s-15de43fg",
    "instance_id": "",
    "memory": "4GB",
    "name": "people-and-fruits-with-db",
    "project_id": "YOUR_PROJECT_ID",
    "region": "europe-west1",
    "status": "",
    "ttl": "8m",
    "user_id": "YOUR_USER_ID"
  }
}`)
}

func TestCreateSessionWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusAccepted, `{
  "data": {
    "id": "s-15de43fg",
    "name": "ws-session",
    "memory": "4GB",
    "instance_id": "",
    "status": "",
    "created_at": "2025-04-04T09:32:35Z",
    "host": "s-15de43fg.ORCHESTRA.neo4j.io",
    "expiry_date": "2025-04-11T09:32:35Z",
    "ttl": "8m",
    "user_id": "YOUR_USER_ID",
    "tenant_id": "`+testProjectID+`",
    "cloud_provider": "gcp",
    "region": "europe-west1"
  }
}`)

	helper.ExecuteCommand("graph-analytics session create --name ws-session --memory 4GB --region europe-west1 --cloud-provider gcp --rw")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
}

func TestCreateSessionMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusAccepted, `{"data": {}}`)

	helper.ExecuteCommand("graph-analytics session create --name session1 --memory 4GB --region europe-west1 --cloud-provider gcp --rw")

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestCreateSessionMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusAccepted, `{"data": {}}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session create --name session1 --memory 4GB --region europe-west1 --cloud-provider gcp --organization-id %s --rw", testOrgID))

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestCreateSessionProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	mockHandler := helper.NewRequestHandlerMock("/v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusAccepted, `{"data": {}}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session create --name session1 --memory 4GB --region europe-west1 --cloud-provider gcp --organization-id %s --project-id unknown-project --rw", testOrgID))

	mockHandler.AssertCalledTimes(0)
	helper.AssertErr("Error: could not find project unknown-project in organization " + testOrgID)
}

func TestCreateSessionWithWait(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	createMock := helper.NewRequestHandlerMock("POST /v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions", http.StatusAccepted, `{
  "data": {
    "id": "559c94c7-15de43fg",
    "name": "people-and-fruits-with-db",
    "memory": "4GB",
    "instance_id": "559c94c7",
    "status": "",
    "created_at": "2025-04-04T09:32:35Z",
    "host": "559c94c7-15de43fg.ORCHESTRA.neo4j.io",
    "expiry_date": "2025-04-11T09:32:35Z",
    "ttl": "8m",
    "user_id": "YOUR_USER_ID",
    "tenant_id": "`+testProjectID+`",
    "cloud_provider": "gcp",
    "region": "europe-west1"
  }
}`)

	getMock := helper.NewRequestHandlerMock("GET /v2beta1/organizations/"+testOrgID+"/projects/"+testProjectID+"/graph-analytics/sessions/559c94c7-15de43fg", http.StatusOK, `{
			"data": {
				"id": "559c94c7-15de43fg",
				"legacy_status": "Creating"
			}
		}`).AddResponse(http.StatusOK, `{
			"data": {
				"id": "559c94c7-15de43fg",
				"legacy_status": "Ready"
			}
		}`)

	helper.ExecuteCommand(fmt.Sprintf("graph-analytics session create --name session1 --memory 4GB --instance-id 559c94c7 --organization-id %s --project-id %s --wait --rw", testOrgID, testProjectID))

	createMock.AssertCalledTimes(1)
	createMock.AssertCalledWithMethod(http.MethodPost)
	createMock.AssertCalledWithBody(fmt.Sprintf(`{"instance_id":"559c94c7","memory":"4GB","name":"session1","tenant_id":"%s"}`, testProjectID))

	getMock.AssertCalledTimes(2)
	getMock.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
{
	"data": {
		"cloud_provider": "gcp",
		"created_at": "2025-04-04T09:32:35Z",
		"expiry_date": "2025-04-11T09:32:35Z",
		"host": "559c94c7-15de43fg.ORCHESTRA.neo4j.io",
		"id": "559c94c7-15de43fg",
		"instance_id": "559c94c7",
		"memory": "4GB",
		"name": "people-and-fruits-with-db",
		"project_id": "YOUR_PROJECT_ID",
		"region": "europe-west1",
		"status": "",
		"ttl": "8m",
		"user_id": "YOUR_USER_ID"
	}
}
	`)
	helper.AssertErrContainsStrings([]string{
		"Waiting for session to be ready...",
		"Session Status: Ready",
	})
}
