// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project_test

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg/projects"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestUseProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("aura-projects.projects", map[string]*projects.AuraProject{"test": {OrganizationId: "testorganizationid", ProjectId: "testprojectid"}})

	helper.ExecuteCommand("config project use test --rw")

	helper.AssertConfigValue("aura-projects.default", "test")
}

func TestUseProjectIfDoesNotExist(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.ExecuteCommand("config project use test --rw")

	helper.AssertErr("Error: could not find a project with the name test")
}
