// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package snapshot_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

const (
	testOrgID     = "test-org-id"
	testProjectID = "YOUR_TENANT_ID"
	testInstID    = "2f49c2b3"
)

// registerProjectsMock registers a mock for the v2beta1 list-projects endpoint
// that returns testProjectID as a valid project.
func registerProjectsMock(helper *testutils.AuraTestHelper) {
	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": [{"id": "`+testProjectID+`", "name": "Test Project"}]}`,
	)
}

// instanceGetBody returns a minimal GET /instances/{id} response body.
func instanceGetBody(id, tenantID string) string {
	return fmt.Sprintf(`{
		"data": {
			"id": %q,
			"name": "Production",
			"status": "running",
			"tenant_id": %q,
			"cloud_provider": "gcp",
			"connection_url": "YOUR_CONNECTION_URL",
			"region": "europe-west1",
			"type": "enterprise-db",
			"memory": "8GB"
		}
	}`, id, tenantID)
}

func TestListSnapshot(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testInstID
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s/snapshots", instanceId), http.StatusOK, `{
		"data": [
			{
				"exportable": true,
				"instance_id": "7261d20a",
				"profile": "AdHoc",
				"snapshot_id": "afdb4e9d-6ba6-4d45-b951-f82843dcbca6",
				"status": "Completed",
				"timestamp": "2024-09-12T13:51:45Z"
			}
		]
		}`)

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot list --instance-id %s --organization-id %s --project-id %s", instanceId, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": [
			{
				"exportable": true,
				"instance_id": "7261d20a",
				"profile": "AdHoc",
				"snapshot_id": "afdb4e9d-6ba6-4d45-b951-f82843dcbca6",
				"status": "Completed",
				"timestamp": "2024-09-12T13:51:45Z"
			}
		]
	}
	`)
}

func TestListSnapshotWithDate(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testInstID
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s/snapshots", instanceId), http.StatusOK, `{
		"data": [
			{
				"exportable": true,
				"instance_id": "7261d20a",
				"profile": "AdHoc",
				"snapshot_id": "afdb4e9d-6ba6-4d45-b951-f82843dcbca6",
				"status": "Completed",
				"timestamp": "2024-09-12T13:51:45Z"
			}
		]
		}`)

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot list --instance-id %s --date 2024-02-13 --organization-id %s --project-id %s", instanceId, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	mockHandler.AssertCalledWithQueryParam("date", "2024-02-13")

	helper.AssertOutJson(`{
		"data": [
			{
				"exportable": true,
				"instance_id": "7261d20a",
				"profile": "AdHoc",
				"snapshot_id": "afdb4e9d-6ba6-4d45-b951-f82843dcbca6",
				"status": "Completed",
				"timestamp": "2024-09-12T13:51:45Z"
			}
		]
	}
	`)
}

func TestListSnapshotWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	registerProjectsMock(&helper)

	instanceId := testInstID
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s/snapshots", instanceId), http.StatusOK, `{"data": []}`)

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot list --instance-id %s", instanceId))

	helper.AsssertOk()
}

func TestListSnapshotMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := testInstID
	helper.ExecuteCommand(fmt.Sprintf("instance snapshot list --instance-id %s", instanceId))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestListSnapshotMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := testInstID
	helper.ExecuteCommand(fmt.Sprintf("instance snapshot list --instance-id %s --organization-id %s", instanceId, testOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestListSnapshotProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	instanceId := testInstID
	helper.ExecuteCommand(fmt.Sprintf("instance snapshot list --instance-id %s --organization-id %s --project-id unknown-project", instanceId, testOrgID))

	helper.AssertErr("Error: could not find project unknown-project in organization " + testOrgID)
}

func TestListSnapshotInstanceNotInProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testInstID
	// Instance belongs to a different project.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, "other-project-id"))

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot list --instance-id %s --organization-id %s --project-id %s", instanceId, testOrgID, testProjectID))

	helper.AssertErr(fmt.Sprintf("Error: could not find instance %s in project %s", instanceId, testProjectID))
}
