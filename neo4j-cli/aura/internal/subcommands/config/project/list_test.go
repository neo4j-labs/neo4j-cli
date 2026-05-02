// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project_test

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg/projects"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListProjects(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.SetConfigValue("aura-projects.projects", map[string]*projects.AuraProject{"test": {OrganizationId: "testorganizationid", ProjectId: "testprojectid"}})
	helper.SetConfigValue("aura-projects.default", "test")

	helper.ExecuteCommand("config project list")

	helper.AssertOutJson(`
		{
			"default": "test",
			"projects": {
				"test": {
					"organization-id": "testorganizationid",
					"project-id": "testprojectid"
				}
			}
		}
	`)
}

func TestListProjectWithNoData(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand("config project list")

	helper.AssertOutJson(`{
		"default": "",
		"projects": {}
	}`)
}

func TestListProjectsTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "table")
	helper.SetConfigValue("aura-projects.projects", map[string]*projects.AuraProject{"test": {OrganizationId: "testorganizationid", ProjectId: "testprojectid"}})
	helper.SetConfigValue("aura-projects.default", "test")

	helper.ExecuteCommand("config project list")

	helper.AssertOutContainsStrings([]string{"test", "testorganizationid", "testprojectid", "true"})
}
