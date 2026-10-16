// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationrun_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
	"github.com/neo4j/cli/internal/confirm/confirmtest"
)

func TestDeleteEvaluationRun(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunPath(testRunID), http.StatusOK, `{}`)

	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run delete %s --organization-id=%s --project-id=%s --rw --yes --force", testRunID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOut(fmt.Sprintf("Evaluation run deleted successfully %s", testRunID))
}

func TestDeleteEvaluationRunWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunPath(testRunID), http.StatusOK, `{}`)

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run delete %s --rw --yes --force", testRunID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOut(fmt.Sprintf("Evaluation run deleted successfully %s", testRunID))
}

func TestDeleteEvaluationRunNotFound(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunPath("non-existent-run-id"), http.StatusNotFound, `{
		"errors": [{"message": "Evaluation run not found"}]
	}`)

	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run delete non-existent-run-id --organization-id=%s --project-id=%s --rw --yes --force", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertErr(`Error: [
	Evaluation run not found
]`)
}

func TestDeleteEvaluationRunConfirmGate(t *testing.T) {
	base := fmt.Sprintf("agent evaluation-run delete %s --organization-id=%s --project-id=%s --rw", testRunID, testOrgID, testProjectID)
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "aura agent evaluation-run delete",
		NoFlagsArgs:   base,
		BothFlagsArgs: base + " --yes --force",
		ResourceLabel: "evaluation-run",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			helper := testutils.NewAuraTestHelper(t)
			t.Cleanup(helper.Close)
			mock := helper.NewRequestHandlerMock(evalRunPath(testRunID), http.StatusOK, `{}`)
			helper.SetStdin(stdin)
			err := helper.ExecuteCommandE(args)
			return confirmtest.GateRunResult{Err: err, Stderr: helper.PrintErr(), Invoked: mock.CalledWithMethod(http.MethodDelete)}
		},
	})
}
