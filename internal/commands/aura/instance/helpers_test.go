// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance_test

import "fmt"

const (
	testListOrgID     = "test-org-id"
	testListProjectID = "YOUR_TENANT_ID"
)

// instanceGetBody returns a minimal GET /instances/{id} response body with the
// given tenant_id. Used to satisfy the v1 pre-flight ownership check in the
// non-migrated mutating command tests (pause/resume/update/overwrite).
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
