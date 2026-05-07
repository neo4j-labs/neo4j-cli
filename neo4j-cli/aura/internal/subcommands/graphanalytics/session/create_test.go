// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session_test

import (
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestCreateAttachedSession(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/graph-analytics/sessions", http.StatusAccepted, `{
  "data": {
    "id": "559c94c7-15de43fg",
    "name": "people-and-fruits-with-db",
    "memory": "4GB",
    "instance_id": "559c94c7",
    "status": "",
    "created_at": "2025-04-04T09:32:35Z",
    "host": "559c94c7-15de43fg.ORCHESTRA.neo4j.io",
    "expiry_date": "2025-04-11T09:32:35Z",
    "tenant_id": "YOUR_PROJECT_ID",
    "ttl": "8m",
    "user_id": "YOUR_USER_ID",
    "cloud_provider": "gcp",
    "region": "europe-west1"
  }
}`)

	helper.ExecuteCommand("graph-analytics session create --name session1 --memory 4GB --instance-id 559c94c7 --rw")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{"instance_id":"559c94c7","memory":"4GB","name":"session1"}`)

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
    "region": "europe-west1",
    "status": "",
    "tenant_id": "YOUR_PROJECT_ID",
    "ttl": "8m",
    "user_id": "YOUR_USER_ID"
  }
}`)
}

func TestCreateStandAloneSession(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/graph-analytics/sessions", http.StatusAccepted, `{
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
    "tenant_id": "YOUR_PROJECT_ID",
    "cloud_provider": "gcp",
    "region": "europe-west1"
  }
}`)

	helper.ExecuteCommand("graph-analytics session create --name session1 --memory 4GB --region europe-west1 --cloud-provider gcp --tenant-id YOUR_PROJECT_ID --rw")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{"cloud_provider":"gcp","memory":"4GB","name":"session1","region":"europe-west1","tenant_id":"YOUR_PROJECT_ID"}`)

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
    "region": "europe-west1",
    "status": "",
    "tenant_id": "YOUR_PROJECT_ID",
    "ttl": "8m",
    "user_id": "YOUR_USER_ID"
  }
}`)
}

func TestCreateSessionWithAwait(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	createMock := helper.NewRequestHandlerMock("POST /v1/graph-analytics/sessions", http.StatusAccepted, `{
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
    "tenant_id": "YOUR_PROJECT_ID",
    "cloud_provider": "gcp",
    "region": "europe-west1"
  }
}`)

	getMock := helper.NewRequestHandlerMock("GET /v1/graph-analytics/sessions/559c94c7-15de43fg", http.StatusOK, `{
			"data": {
				"id": "559c94c7-15de43fg",
				"status": "Creating"
			}
		}`).AddResponse(http.StatusOK, `{
			"data": {
				"id": "559c94c7-15de43fg",
				"status": "Ready"
			}
		}`)

	helper.ExecuteCommand("graph-analytics session create --name session1 --memory 4GB --instance-id 559c94c7 --await --rw")

	createMock.AssertCalledTimes(1)
	createMock.AssertCalledWithMethod(http.MethodPost)
	createMock.AssertCalledWithBody(`{"instance_id":"559c94c7","memory":"4GB","name":"session1"}`)

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
		"region": "europe-west1",
		"status": "",
		"tenant_id": "YOUR_PROJECT_ID",
		"ttl": "8m",
		"user_id": "YOUR_USER_ID"
	}
}
Waiting for session to be ready...
Session Status: Ready
	`)
}
