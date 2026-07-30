// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

// TestDeleteVirtualGraph covers the empty-202 contract: the API returns no
// body, so the command echoes the id it accepted for deletion rather than
// printing nothing.
func TestDeleteVirtualGraph(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphPath(testVirtualGraphID), http.StatusAccepted, "")

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph delete %s --rw --yes --force --organization-id %s --project-id %s", testVirtualGraphID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOutJson(`{
  "data": {
    "id": "ge82059a"
  }
}`)
}

func TestDeleteVirtualGraphRequiresConfirmation(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphPath(testVirtualGraphID), http.StatusAccepted, "")

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph delete %s --rw --organization-id %s --project-id %s", testVirtualGraphID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(0)
	helper.AssertErrContainsStrings([]string{"--yes", "--force"})
}

func TestDeleteVirtualGraphRequiresRw(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph delete %s --yes --force --organization-id %s --project-id %s", testVirtualGraphID, testOrgID, testProjectID))

	helper.AssertErrContainsStrings([]string{"this command writes; pass --rw to allow it"})
}

func TestDeleteVirtualGraphRejectsTraversalID(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph delete .. --rw --yes --force --organization-id %s --project-id %s", testOrgID, testProjectID))

	helper.AssertErr(`Error: invalid virtual-graph id ".."`)
}
