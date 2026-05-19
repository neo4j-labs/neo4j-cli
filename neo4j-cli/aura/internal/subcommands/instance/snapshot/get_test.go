// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package snapshot_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/stretchr/testify/require"
)

func TestGetSnapshot(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testInstID
	snapshotId := "afdb4e9d-6ba6-4d45-b951-f82843dcbca6"

	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s/snapshots/%s", instanceId, snapshotId), http.StatusOK, `{
			"data": {
				"exportable": true,
				"instance_id": "7261d20a",
				"profile": "AdHoc",
				"snapshot_id": "afdb4e9d-6ba6-4d45-b951-f82843dcbca6",
				"status": "Completed",
				"timestamp": "2024-09-12T13:51:45Z"
			}
		}`)

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot get --format json --instance-id %s --organization-id %s --project-id %s %s", instanceId, testOrgID, testProjectID, snapshotId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": {
			"exportable": true,
			"instance_id": "7261d20a",
			"profile": "AdHoc",
			"snapshot_id": "afdb4e9d-6ba6-4d45-b951-f82843dcbca6",
			"status": "Completed",
			"timestamp": "2024-09-12T13:51:45Z"
		}
	}`)
}

func TestGetSnapshotWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	registerProjectsMock(&helper)

	instanceId := testInstID
	snapshotId := "afdb4e9d-6ba6-4d45-b951-f82843dcbca6"

	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s/snapshots/%s", instanceId, snapshotId), http.StatusOK, `{
			"data": {
				"exportable": true,
				"instance_id": "7261d20a",
				"profile": "AdHoc",
				"snapshot_id": "afdb4e9d-6ba6-4d45-b951-f82843dcbca6",
				"status": "Completed",
				"timestamp": "2024-09-12T13:51:45Z"
			}
		}`)

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot get --instance-id %s %s", instanceId, snapshotId))

	helper.AsssertOk()
}

func TestGetSnapshotMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := testInstID
	snapshotId := "afdb4e9d-6ba6-4d45-b951-f82843dcbca6"
	helper.ExecuteCommand(fmt.Sprintf("instance snapshot get --instance-id %s %s", instanceId, snapshotId))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestGetSnapshotMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := testInstID
	snapshotId := "afdb4e9d-6ba6-4d45-b951-f82843dcbca6"
	helper.ExecuteCommand(fmt.Sprintf("instance snapshot get --instance-id %s --organization-id %s %s", instanceId, testOrgID, snapshotId))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestGetSnapshotProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	instanceId := testInstID
	snapshotId := "afdb4e9d-6ba6-4d45-b951-f82843dcbca6"
	helper.ExecuteCommand(fmt.Sprintf("instance snapshot get --instance-id %s --organization-id %s --project-id unknown-project %s", instanceId, testOrgID, snapshotId))

	helper.AssertErr("Error: could not find project unknown-project in organization " + testOrgID)
}

func TestGetSnapshotInstanceNotInProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testInstID
	snapshotId := "afdb4e9d-6ba6-4d45-b951-f82843dcbca6"

	// Instance belongs to a different project.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, "other-project-id"))

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot get --instance-id %s --organization-id %s --project-id %s %s", instanceId, testOrgID, testProjectID, snapshotId))

	helper.AssertErr(fmt.Sprintf("Error: could not find instance %s in project %s", instanceId, testProjectID))
}

// TestGetSnapshotNotFound_HasSuggestion locks the WithNotFoundContext
// rewrite at the snapshot-GET call site: when the API returns 404 for a
// snapshot path, the API layer's parseResourceFromRequest mis-tags the
// resource as "instance"; the caller rewrites ResourceType to "snapshot"
// and attaches the snapshot-list Suggestion (REQ-F-012).
func TestGetSnapshotNotFound_HasSuggestion(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testInstID
	snapshotId := "afdb4e9d-6ba6-4d45-b951-f82843dcbca6"

	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s/snapshots/%s", instanceId, snapshotId), http.StatusNotFound, `{
		"errors": [
			{"message": "snapshot not found", "reason": "not-found"}
		]
	}`)

	err := helper.ExecuteCommandE(fmt.Sprintf("instance snapshot get --instance-id %s --organization-id %s --project-id %s %s", instanceId, testOrgID, testProjectID, snapshotId))

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T: %v", err, err)
	require.Equal(t, 3, ce.Code)
	require.Equal(t, "snapshot", ce.ResourceType)
	require.Equal(t, snapshotId, ce.ResourceID)
	require.Equal(t, "Run 'neo4j-cli aura instance snapshot list --instance-id <id>' to see snapshots for this instance.", ce.Suggestion)
}

func TestGetSnapshotWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"
	snapshotId := "afdb4e9d-6ba6-4d45-b951-f82843dcbca6"

	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s/snapshots/%s", instanceId, snapshotId), http.StatusOK, `{
			"data": {
				"exportable": true,
				"instance_id": "7261d20a",
				"profile": "AdHoc",
				"snapshot_id": "afdb4e9d-6ba6-4d45-b951-f82843dcbca6",
				"status": "Completed",
				"timestamp": "2024-09-12T13:51:45Z"
			}
		}`)

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot get --format json --instance-id %s --organization-id %s --project-id %s %s\"\n\"", instanceId, testOrgID, testProjectID, snapshotId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
}
