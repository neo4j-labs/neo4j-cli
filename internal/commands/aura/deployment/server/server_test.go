// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server_test

import (
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
	"github.com/stretchr/testify/assert"
)

const (
	orgID  = "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projID = "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	depID  = "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
	base   = "/v2beta1/organizations/" + orgID + "/projects/" + projID + "/fleet-manager/deployments"
)

const (
	serverID = "srv-1"
	scope    = "--deployment-id=" + depID + " --organization-id=" + orgID + " --project-id=" + projID
)

func TestListDeploymentServers(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	body := `{"data":[{"address":"10.0.0.1","id":"srv-1","name":"s","status":"online","version":"5.26.0"}]}`
	mock := helper.NewRequestHandlerMock(base+"/"+depID+"/servers", http.StatusOK, body)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand("deployment server list " + scope)

	mock.AssertCalledTimes(1)
	mock.AssertCalledWithMethod(http.MethodGet)
	helper.AssertOutJson(body)
}

func TestListDeploymentServerDatabases(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	body := `{"data":[{"current_status":"online","id":"x","name":"neo4j","role":"primary","type":"standard","writer":true}]}`
	mock := helper.NewRequestHandlerMock(base+"/"+depID+"/servers/"+serverID+"/databases", http.StatusOK, body)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand("deployment server database list --server-id=" + serverID + " " + scope)

	mock.AssertCalledTimes(1)
	helper.AssertOutJson(body)
}

func TestListDeploymentServerDatabasesRequiresServerID(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	assert.Error(t, helper.ExecuteCommandE("deployment server database list "+scope))
}
