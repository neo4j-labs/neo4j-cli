// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package deployment_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
	"github.com/neo4j/cli/internal/confirm/confirmtest"
	"github.com/stretchr/testify/assert"
)

const (
	orgID  = "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projID = "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	depID  = "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
	base   = "/v2beta1/organizations/" + orgID + "/projects/" + projID + "/fleet-manager/deployments"
)

const scope = "--organization-id=" + orgID + " --project-id=" + projID

func TestListDeployments(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock(base, http.StatusOK, `{"data":[{"connection_url":"neo4j://x","created_by":"me","id":"d1","name":"one","status":"healthy"}]}`)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand("deployment list " + scope)

	mock.AssertCalledTimes(1)
	mock.AssertCalledWithMethod(http.MethodGet)
	helper.AssertOutJson(`{"data":[{"connection_url":"neo4j://x","created_by":"me","id":"d1","name":"one","status":"healthy"}]}`)
}

func TestListDeploymentsDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock(base, http.StatusOK, `{"data":[]}`)
	helper.SetDefaultProjectInConfig(orgID, projID)
	helper.ExecuteCommand("deployment list")

	mock.AssertCalledTimes(1)
}

func TestGetDeployment(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	body := `{"data":{"dbms":{"edition":"enterprise","packaging":"docker"},"id":"` + depID + `","name":"one","token":{"auto_rotated":true}}}`
	mock := helper.NewRequestHandlerMock(base+"/"+depID, http.StatusOK, body)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand("deployment get " + depID + " " + scope)

	mock.AssertCalledTimes(1)
	mock.AssertCalledWithMethod(http.MethodGet)
	helper.AssertOutJson(body)
}

func TestGetDeploymentRejectsMalformedID(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock(base, http.StatusOK, `{}`)
	err := helper.ExecuteCommandE("deployment get a%2Fb " + scope)

	assert.Error(t, err)
	mock.AssertCalledTimes(0)
}

func TestCreateDeployment(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock(base, http.StatusOK, `{"data":{"id":"`+depID+`"}}`)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand("deployment create --name my-dep --connection-url neo4j://x " + scope + " --rw")

	mock.AssertCalledTimes(1)
	mock.AssertCalledWithMethod(http.MethodPost)
	mock.AssertCalledWithBody(`{"connection_url":"neo4j://x","name":"my-dep"}`)
	helper.AssertOutJson(`{"data":{"id":"` + depID + `"}}`)
}

func TestCreateDeploymentOmitsEmptyConnectionURL(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock(base, http.StatusOK, `{"data":{"id":"`+depID+`"}}`)
	helper.ExecuteCommand("deployment create --name my-dep " + scope + " --rw")

	mock.AssertCalledWithBody(`{"name":"my-dep"}`)
}

func TestCreateDeploymentRequiresName(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock(base, http.StatusOK, `{}`)
	assert.Error(t, helper.ExecuteCommandE("deployment create "+scope+" --rw"))
	mock.AssertCalledTimes(0)
}

func TestCreateDeploymentRequiresRW(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock(base, http.StatusOK, `{}`)
	assert.Error(t, helper.ExecuteCommandE("deployment create --name x "+scope))
	mock.AssertCalledTimes(0)
}

func TestDeleteDeployment(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock(base+"/"+depID, http.StatusNoContent, "")
	helper.ExecuteCommand("deployment delete " + depID + " " + scope + " --rw --yes --force")

	mock.AssertCalledTimes(1)
	mock.AssertCalledWithMethod(http.MethodDelete)
	helper.AssertOut("Deployment deleted successfully " + depID)
}

func TestDeleteDeploymentNotFound(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(base+"/"+depID, http.StatusForbidden, `{"errors":[{"message":"Deployment not found"}]}`)
	assert.Error(t, helper.ExecuteCommandE("deployment delete "+depID+" "+scope+" --rw --yes --force"))
}

func TestDeleteDeploymentConfirmGate(t *testing.T) {
	args := fmt.Sprintf("deployment delete %s %s --rw", depID, scope)
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "aura deployment delete",
		NoFlagsArgs:   args,
		BothFlagsArgs: args + " --yes --force",
		ResourceLabel: "deployment",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			helper := testutils.NewAuraTestHelper(t)
			t.Cleanup(helper.Close)
			mock := helper.NewRequestHandlerMock(base+"/"+depID, http.StatusNoContent, "")
			helper.SetStdin(stdin)
			err := helper.ExecuteCommandE(args)
			return confirmtest.GateRunResult{Err: err, Stderr: helper.PrintErr(), Invoked: mock.CalledWithMethod(http.MethodDelete)}
		},
	})
}
