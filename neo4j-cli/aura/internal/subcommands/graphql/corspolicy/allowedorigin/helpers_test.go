// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package allowedorigin_test

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

const (
	testOrgID     = "test-org-id"
	testProjectID = "YOUR_TENANT_ID"
)

func registerProjectsMock(helper *testutils.AuraTestHelper) {
	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": [{"id": "`+testProjectID+`", "name": "Test Project"}]}`,
	)
}

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
