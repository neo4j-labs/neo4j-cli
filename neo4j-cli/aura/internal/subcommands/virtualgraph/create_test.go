// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

// createResponseBody is the 202 create response: the VirtualGraph shape plus
// the write-once plain_password.
const createResponseBody = `{
	"data": {
		"id": "ge82059a",
		"name": "sales-analytics",
		"status": "creating",
		"cloud_provider": "gcp",
		"region": "europe-west1",
		"memory": "4Gi",
		"bolt_url": "neo4j+s://ge82059a.graph-engine.neo4j.io",
		"data_source_id": "ds-abc123",
		"data_source_type": "databricks-pat",
		"error_detail": "",
		"created_at": "2026-06-09T10:15:00Z",
		"plain_password": "generated-password-only-shown-once"
	}
}`

const createFlags = "--rw --name sales-analytics --data-source-id ds-abc123 --import-model-id im-xyz789 --cloud-provider gcp --region europe-west1"

func TestCreateVirtualGraph(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusAccepted, createResponseBody)

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph create %s --organization-id %s --project-id %s", createFlags, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	// project_id is deliberately absent: the API derives the owning project from
	// the scoped path and the caller's token.
	mockHandler.AssertCalledWithBody(`{
		"name": "sales-analytics",
		"data_source_id": "ds-abc123",
		"import_model_id": "im-xyz789",
		"cloud_provider": "gcp",
		"region": "europe-west1"
	}`)

	helper.AssertOutJson(`{
  "data": {
    "bolt_url": "neo4j+s://ge82059a.graph-engine.neo4j.io",
    "cloud_provider": "gcp",
    "created_at": "2026-06-09T10:15:00Z",
    "data_source_id": "ds-abc123",
    "data_source_type": "databricks-pat",
    "error_detail": "",
    "id": "ge82059a",
    "memory": "4Gi",
    "name": "sales-analytics",
    "plain_password": "generated-password-only-shown-once",
    "region": "europe-west1",
    "status": "creating"
  }
}`)
}

// TestCreateVirtualGraphWithOptionalFlags covers the two optional body fields:
// memory and the BigQuery-only maximum_bytes_billed.
func TestCreateVirtualGraphWithOptionalFlags(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusAccepted, createResponseBody)

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph create %s --memory 8Gi --maximum-bytes-billed 1099511627776 --organization-id %s --project-id %s", createFlags, testOrgID, testProjectID))

	mockHandler.AssertCalledWithBody(`{
		"name": "sales-analytics",
		"data_source_id": "ds-abc123",
		"import_model_id": "im-xyz789",
		"cloud_provider": "gcp",
		"region": "europe-west1",
		"memory": "8Gi",
		"maximum_bytes_billed": 1099511627776
	}`)
}

// TestCreateVirtualGraphOmitsUnsetMaximumBytesBilled guards the Changed() check:
// an unset --maximum-bytes-billed must not be sent as 0, which the API would
// read as "cap every query at zero bytes" rather than "use the default".
func TestCreateVirtualGraphOmitsUnsetMaximumBytesBilled(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusAccepted, createResponseBody)

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph create %s --organization-id %s --project-id %s", createFlags, testOrgID, testProjectID))

	for _, call := range mockHandler.Calls {
		if _, ok := call.Body["maximum_bytes_billed"]; ok {
			t.Fatalf("maximum_bytes_billed must be omitted when the flag is unset; got body %v", call.Body)
		}
	}
}

// TestCreateVirtualGraphExplicitZeroMaximumBytesBilled is the counterpart: an
// explicit --maximum-bytes-billed 0 IS sent, so the caller can request the cap
// the API assigns to a literal zero rather than having it silently dropped.
func TestCreateVirtualGraphExplicitZeroMaximumBytesBilled(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusAccepted, createResponseBody)

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph create %s --maximum-bytes-billed 0 --organization-id %s --project-id %s", createFlags, testOrgID, testProjectID))

	mockHandler.AssertCalledWithBody(`{
		"name": "sales-analytics",
		"data_source_id": "ds-abc123",
		"import_model_id": "im-xyz789",
		"cloud_provider": "gcp",
		"region": "europe-west1",
		"maximum_bytes_billed": 0
	}`)
}

// TestCreateVirtualGraphWithWait covers the --wait path: create returns status
// "creating", and the command polls the scoped GET until the status moves off
// it.
func TestCreateVirtualGraphWithWait(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	createMock := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusAccepted, createResponseBody)
	pollMock := helper.NewRequestHandlerMock(virtualGraphPath(testVirtualGraphID), http.StatusOK, virtualGraphBody(testVirtualGraphID, "creating"))
	pollMock.AddResponse(http.StatusOK, virtualGraphBody(testVirtualGraphID, "running"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph create %s --wait --organization-id %s --project-id %s", createFlags, testOrgID, testProjectID))

	createMock.AssertCalledTimes(1)
	pollMock.AssertCalledTimes(2)

	helper.AssertErrContainsStrings([]string{
		"Waiting for virtual graph to be running...",
		"Virtual Graph Status: running",
	})
}

// TestCreateVirtualGraphWithWaitUppercaseStatus pins the case-insensitive poll
// comparison: the API's status casing is not guaranteed, and a case-sensitive
// compare against the lowercase constant would satisfy the poll condition on
// the very first request and return immediately, making --wait a silent no-op.
func TestCreateVirtualGraphWithWaitUppercaseStatus(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusAccepted, createResponseBody)
	pollMock := helper.NewRequestHandlerMock(virtualGraphPath(testVirtualGraphID), http.StatusOK, virtualGraphBody(testVirtualGraphID, "CREATING"))
	pollMock.AddResponse(http.StatusOK, virtualGraphBody(testVirtualGraphID, "CREATING"))
	pollMock.AddResponse(http.StatusOK, virtualGraphBody(testVirtualGraphID, "RUNNING"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph create %s --wait --organization-id %s --project-id %s", createFlags, testOrgID, testProjectID))

	// Three polls: the two CREATING responses must not satisfy the condition.
	pollMock.AssertCalledTimes(3)
	helper.AssertErrContainsStrings([]string{"Virtual Graph Status: RUNNING"})
}

func TestCreateVirtualGraphMissingRequiredFlags(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph create --rw --name sales-analytics --organization-id %s --project-id %s", testOrgID, testProjectID))

	helper.AssertErrContainsStrings([]string{`required flag(s) "cloud-provider", "data-source-id", "import-model-id", "region" not set`})
}

func TestCreateVirtualGraphRejectsUnknownCloudProvider(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph create --rw --name sales-analytics --data-source-id ds-abc123 --import-model-id im-xyz789 --cloud-provider oracle --region europe-west1 --organization-id %s --project-id %s", testOrgID, testProjectID))

	helper.AssertErrContainsStrings([]string{`must be one of "aws", "azure", or "gcp"`})
}
