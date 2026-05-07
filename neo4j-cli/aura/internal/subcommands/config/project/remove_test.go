// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project_test

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg/projects"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestRemoveProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("aura-projects.projects", map[string]*projects.AuraProject{"test": {OrganizationId: "testorganizationid", ProjectId: "testprojectid"}})

	helper.ExecuteCommand("config project remove test --rw")

	helper.AssertConfigValue("aura-projects.projects", "{}")
	helper.AssertConfigValue("aura-projects.default", "")
}

func TestRemoveProjectWhenProjectDoesNotExist(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	helper.ExecuteCommand("config project remove test --rw")

	helper.AssertErr("Error: could not find a project with the name test to remove")
}

func TestRemoveProjectWhenMultipleProjectsExist(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("aura-projects.projects", map[string]*projects.AuraProject{
		"first-project":  {OrganizationId: "testorganizationid", ProjectId: "testprojectid"},
		"second-project": {OrganizationId: "testorganizationid", ProjectId: "testprojectid"},
	})
	helper.SetConfigValue("aura-projects.default", "first-project")

	helper.ExecuteCommand("config project remove first-project --rw")

	helper.AssertConfigValue("aura-projects.projects", `
	{
		"second-project": {
			"organization-id": "testorganizationid",
			"project-id": "testprojectid"
		}
	}`)
	helper.AssertConfigValue("aura-projects.default", "second-project")
}

func TestRemoveProjectWhenProjectDoesNotExistWithMultipleProjects(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("aura-projects.projects", map[string]*projects.AuraProject{
		"first-project":  {OrganizationId: "testorganizationid", ProjectId: "testprojectid"},
		"second-project": {OrganizationId: "testorganizationid", ProjectId: "testprojectid"},
	})
	helper.SetConfigValue("aura-projects.default", "first-project")

	helper.ExecuteCommand("config project remove non-existing --rw")

	helper.AssertErr("Error: could not find a project with the name non-existing to remove")
	helper.AssertConfigValue("aura-projects.projects", `
	{
		"first-project": {
			"organization-id": "testorganizationid",
			"project-id": "testprojectid"
		},
		"second-project": {
			"organization-id": "testorganizationid",
			"project-id": "testprojectid"
		}
	}`)
	helper.AssertConfigValue("aura-projects.default", "first-project")
}
