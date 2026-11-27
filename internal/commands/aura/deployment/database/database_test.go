// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database_test

import (
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
)

const (
	orgID  = "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projID = "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	depID  = "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
	base   = "/v2beta1/organizations/" + orgID + "/projects/" + projID + "/fleet-manager/deployments"
)

func TestListDeploymentDatabases(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	body := `{"data":[{"access":"read-write","default":true,"id":"db1","name":"neo4j","node_count":3}]}`
	mock := helper.NewRequestHandlerMock(base+"/"+depID+"/databases", http.StatusOK, body)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand("deployment database list --deployment-id=" + depID + " --organization-id=" + orgID + " --project-id=" + projID)

	mock.AssertCalledTimes(1)
	mock.AssertCalledWithMethod(http.MethodGet)
	helper.AssertOutJson(body)
}
