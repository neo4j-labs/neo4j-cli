// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

// TestUpdateVirtualGraphName covers the PATCH-then-GET shape: the API
// acknowledges with an empty 202, so the command re-reads the resource to have
// something to print.
func TestUpdateVirtualGraphName(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphPath(testVirtualGraphID), http.StatusAccepted, "")
	mockHandler.AddResponse(http.StatusOK, virtualGraphBody(testVirtualGraphID, "updating"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph update %s --rw --name renamed-analytics --organization-id %s --project-id %s", testVirtualGraphID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodPatch)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	mockHandler.AssertCalledWithBody(`{"name": "renamed-analytics"}`)

	helper.AssertOutContainsStrings([]string{`"status": "updating"`})
}

func TestUpdateVirtualGraphMemoryAndModel(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphPath(testVirtualGraphID), http.StatusAccepted, "")
	mockHandler.AddResponse(http.StatusOK, virtualGraphBody(testVirtualGraphID, "updating"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph update %s --rw --memory 8Gi --import-model-id im-xyz789 --organization-id %s --project-id %s", testVirtualGraphID, testOrgID, testProjectID))

	mockHandler.AssertCalledWithBody(`{"memory": "8Gi", "import_model_id": "im-xyz789"}`)
}

func TestUpdateVirtualGraphRequiresOneFlag(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph update %s --rw --organization-id %s --project-id %s", testVirtualGraphID, testOrgID, testProjectID))

	helper.AssertErrContainsStrings([]string{`at least one of the flags in the group [name memory import-model-id] is required`})
}

func TestUpdateVirtualGraphRequiresRw(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph update %s --name renamed-analytics --organization-id %s --project-id %s", testVirtualGraphID, testOrgID, testProjectID))

	helper.AssertErrContainsStrings([]string{"this command writes; pass --rw to allow it"})
}
