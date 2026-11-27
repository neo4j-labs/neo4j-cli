// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package token_test

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

const (
	tokenPath = base + "/" + depID + "/token"
	scope     = "--deployment-id=" + depID + " --organization-id=" + orgID + " --project-id=" + projID
)

func TestTokenCreateAndUpdate(t *testing.T) {
	for _, tc := range []struct {
		verb, method string
	}{
		{"create", http.MethodPost},
		{"update", http.MethodPatch},
	} {
		t.Run(tc.verb, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			mock := helper.NewRequestHandlerMock(tokenPath, http.StatusOK, `{"data":{"token":"FM_API_TOKEN"}}`)
			helper.SetConfigValue("format", "json")
			helper.ExecuteCommand("deployment token " + tc.verb + " " + scope + " --rw")

			mock.AssertCalledTimes(1)
			mock.AssertCalledWithMethod(tc.method)
			mock.AssertCalledWithBody("{}")
			helper.AssertOutJson(`{"data":{"token":"FM_API_TOKEN"}}`)
		})
	}
}

func TestTokenRequiresDeploymentID(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock(tokenPath, http.StatusOK, `{}`)
	assert.Error(t, helper.ExecuteCommandE("deployment token create --organization-id="+orgID+" --project-id="+projID+" --rw"))
	mock.AssertCalledTimes(0)
}

func TestTokenCreateWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock(tokenPath, http.StatusOK, `{"data":{"token":"T"}}`)
	helper.SetDefaultProjectInConfig(orgID, projID)
	helper.ExecuteCommand("deployment token create --deployment-id=" + depID + " --rw")

	mock.AssertCalledTimes(1)
}

func TestTokenDelete(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock(tokenPath, http.StatusNoContent, "")
	helper.ExecuteCommand("deployment token delete " + scope + " --rw --yes --force")

	mock.AssertCalledTimes(1)
	mock.AssertCalledWithMethod(http.MethodDelete)
	helper.AssertOut("Deployment token deleted successfully for deployment " + depID)
}

func TestTokenDeleteConfirmGate(t *testing.T) {
	args := fmt.Sprintf("deployment token delete %s --rw", scope)
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "aura deployment token delete",
		NoFlagsArgs:   args,
		BothFlagsArgs: args + " --yes --force",
		ResourceLabel: "token",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			helper := testutils.NewAuraTestHelper(t)
			t.Cleanup(helper.Close)
			mock := helper.NewRequestHandlerMock(tokenPath, http.StatusNoContent, "")
			helper.SetStdin(stdin)
			err := helper.ExecuteCommandE(args)
			return confirmtest.GateRunResult{Err: err, Stderr: helper.PrintErr(), Invoked: mock.CalledWithMethod(http.MethodDelete)}
		},
	})
}
