// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationdataset_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
	"github.com/neo4j/cli/internal/confirm/confirmtest"
)

func TestDeleteEvaluationDataset(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetPath(testDatasetID), http.StatusOK, `{}`)

	helper.ExecuteCommand(fmt.Sprintf(
		"agent evaluation-dataset delete %s --organization-id=%s --project-id=%s --rw --yes --force",
		testDatasetID, testOrgID, testProjectID,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOut("Evaluation dataset deleted successfully f47ac10b-58cc-4372-a567-0e02b2c3d479")
}

func TestDeleteEvaluationDatasetWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetPath(testDatasetID), http.StatusOK, `{}`)

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-dataset delete %s --rw --yes --force", testDatasetID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOut("Evaluation dataset deleted successfully f47ac10b-58cc-4372-a567-0e02b2c3d479")
}

func TestDeleteEvaluationDatasetConfirmGate(t *testing.T) {
	base := fmt.Sprintf("agent evaluation-dataset delete %s --organization-id=%s --project-id=%s --rw", testDatasetID, testOrgID, testProjectID)
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "aura agent evaluation-dataset delete",
		NoFlagsArgs:   base,
		BothFlagsArgs: base + " --yes --force",
		ResourceLabel: "evaluation-dataset",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			helper := testutils.NewAuraTestHelper(t)
			t.Cleanup(helper.Close)
			mock := helper.NewRequestHandlerMock(datasetPath(testDatasetID), http.StatusOK, `{}`)
			helper.SetStdin(stdin)
			err := helper.ExecuteCommandE(args)
			return confirmtest.GateRunResult{Err: err, Stderr: helper.PrintErr(), Invoked: mock.CalledWithMethod(http.MethodDelete)}
		},
	})
}
