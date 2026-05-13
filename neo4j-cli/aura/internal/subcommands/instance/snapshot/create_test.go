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
	instanceId := "2f49c2b3"
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("POST /v1/instances/%s/snapshots", instanceId), http.StatusAccepted, `{
		"data": {
		  "snapshot_id": "snap123"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot create --instance-id %s --rw", instanceId))

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
	instanceId := "2f49c2b3"
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

	helper.ExecuteCommand(fmt.Sprintf("instance snapshot create --instance-id %s --wait --rw", instanceId))

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
