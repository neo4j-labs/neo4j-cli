// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package snapshot_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestCreateSnapshot(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testInstID

	// Pre-flight GET for ownership check.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("POST /v1/instances/%s/snapshots", instanceId), http.StatusAccepted, `{
		"data": {
		  "snapshot_id": "snap123"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot create --instance-id %s --organization-id %s --project-id %s --rw", instanceId, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertOutJson(`{
		"data": {
		  "snapshot_id": "snap123"
		}
	  }`)
}

func TestCreateSnapshotWithWait(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testInstID

	// Pre-flight GET for ownership check.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))

	createMock := helper.NewRequestHandlerMock(fmt.Sprintf("POST /v1/instances/%s/snapshots", instanceId), http.StatusAccepted, `{
		"data": {
		  "snapshot_id": "snap123"
		}
	  }`)

	getMock := helper.NewRequestHandlerMock(fmt.Sprintf("GET /v1/instances/%s/snapshots/snap123", instanceId), http.StatusOK, `{
			"data": {
				"id": "db1d1234",
				"status": "Pending"
			}
		}`).AddResponse(http.StatusOK, `{
			"data": {
				"id": "db1d1234",
				"status": "InProgress"
			}
		}`).AddResponse(http.StatusOK, `{
			"data": {
				"id": "db1d1234",
				"status": "Completed"
			}
		}`)

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot create --instance-id %s --organization-id %s --project-id %s --wait --rw", instanceId, testOrgID, testProjectID))

	createMock.AssertCalledTimes(1)
	createMock.AssertCalledWithMethod(http.MethodPost)

	getMock.AssertCalledTimes(3)
	getMock.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
{
	"data": {
		"snapshot_id": "snap123"
	}
}
	`)
	helper.AssertErrContainsStrings([]string{
		"Waiting for snapshot to be ready...",
		"Snapshot Status: Completed",
	})
}

func TestCreateSnapshotWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	registerProjectsMock(&helper)

	instanceId := testInstID
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
	helper.NewRequestHandlerMock(fmt.Sprintf("POST /v1/instances/%s/snapshots", instanceId), http.StatusAccepted, `{"data": {"snapshot_id": "snap123"}}`)

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot create --instance-id %s --rw", instanceId))

	helper.AsssertOk()
}

func TestCreateSnapshotMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := testInstID
	helper.ExecuteCommand(fmt.Sprintf("instance snapshot create --instance-id %s --rw", instanceId))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestCreateSnapshotMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := testInstID
	helper.ExecuteCommand(fmt.Sprintf("instance snapshot create --instance-id %s --organization-id %s --rw", instanceId, testOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestCreateSnapshotProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	instanceId := testInstID
	helper.ExecuteCommand(fmt.Sprintf("instance snapshot create --instance-id %s --organization-id %s --project-id unknown-project --rw", instanceId, testOrgID))

	helper.AssertErr("Error: could not find project unknown-project in organization " + testOrgID)
}

func TestCreateSnapshotInstanceNotInProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testInstID
	// Instance belongs to a different project.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, "other-project-id"))

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot create --instance-id %s --organization-id %s --project-id %s --rw", instanceId, testOrgID, testProjectID))

	helper.AssertErr(fmt.Sprintf("Error: could not find instance %s in project %s", instanceId, testProjectID))
}
