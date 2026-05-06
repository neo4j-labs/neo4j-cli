// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project_test

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg/projects"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestAddFirstProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	helper.ExecuteCommand("config project add --name test --organization-id testorganizationid --project-id testprojectid --rw")

	helper.AssertConfigValue("aura-projects.projects", `
	{
		"test": {
			"organization-id": "testorganizationid",
			"project-id": "testprojectid"
		}
	}`)
	helper.AssertConfigValue("aura-projects.default", "test")
}

func TestAddProjectIfAlreadyExists(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("aura-projects.projects", map[string]*projects.AuraProject{"test": {OrganizationId: "testorganizationid", ProjectId: "testprojectid"}})

	helper.ExecuteCommand("config project add --name test --organization-id testorganizationid --project-id testprojectid --rw")

	helper.AssertErr("Error: already have a project with the name test")
}
func TestAddAditionalProjects(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("aura-projects.projects", map[string]*projects.AuraProject{"test": {OrganizationId: "testorganizationid", ProjectId: "testprojectid"}})
	helper.SetConfigValue("aura-projects.default", "test")

	helper.ExecuteCommand("config project add --name test-new --organization-id newtestorganizationid --project-id newtestprojectid --rw")

	helper.AssertConfigValue("aura-projects.projects", `
	{
		"test": {
			"organization-id":"testorganizationid",
			"project-id":"testprojectid"
		},
		"test-new" :{
			"organization-id":"newtestorganizationid",
			"project-id":"newtestprojectid"
		}
	}`)
	helper.AssertConfigValue("aura-projects.default", "test")
}
