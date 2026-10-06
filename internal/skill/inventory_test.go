// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregateCatalog(t *testing.T) {
	// Stable, named agent pointers — values irrelevant beyond Name, which
	// the aggregator reads to populate InstalledAgents.
	agentA := &Agent{Name: "agent-a"}
	agentB := &Agent{Name: "agent-b"}
	agentC := &Agent{Name: "agent-c"}

	row := func(agent *Agent, installed bool, installedVersion, availableVersion string) InventoryRow {
		return InventoryRow{
			Skill:            "neo4j-cypher-skill",
			Source:           sourceCatalog,
			Agent:            agent,
			Installed:        installed,
			InstalledVersion: installedVersion,
			AvailableVersion: availableVersion,
			Status:           statusFor(installed, installedVersion, availableVersion),
		}
	}

	tests := []struct {
		name          string
		rows          []InventoryRow
		wantStatus    string
		wantCount     int
		wantAgents    []string
		wantSkill     string
		wantAvailable string
	}{
		{
			name:       "empty rows → zero-value summary, not-installed status",
			rows:       nil,
			wantStatus: statusNotInstalled,
			wantCount:  0,
			wantAgents: nil,
			wantSkill:  "",
		},
		{
			name:          "single row installed matching version → installed",
			rows:          []InventoryRow{row(agentA, true, "1.0.0", "1.0.0")},
			wantStatus:    statusInstalled,
			wantCount:     1,
			wantAgents:    []string{"agent-a"},
			wantSkill:     "neo4j-cypher-skill",
			wantAvailable: "1.0.0",
		},
		{
			name:          "single row installed mismatched version → drift",
			rows:          []InventoryRow{row(agentA, true, "0.9.0", "1.0.0")},
			wantStatus:    statusDrift,
			wantCount:     1,
			wantAgents:    []string{"agent-a"},
			wantSkill:     "neo4j-cypher-skill",
			wantAvailable: "1.0.0",
		},
		{
			name:          "single row installed empty version → unknown-version",
			rows:          []InventoryRow{row(agentA, true, "", "1.0.0")},
			wantStatus:    statusUnknownVersion,
			wantCount:     1,
			wantAgents:    []string{"agent-a"},
			wantSkill:     "neo4j-cypher-skill",
			wantAvailable: "1.0.0",
		},
		{
			name:          "single row not installed → not-installed",
			rows:          []InventoryRow{row(agentA, false, "", "1.0.0")},
			wantStatus:    statusNotInstalled,
			wantCount:     0,
			wantAgents:    nil,
			wantSkill:     "neo4j-cypher-skill",
			wantAvailable: "1.0.0",
		},
		{
			name: "all installed all matching → installed",
			rows: []InventoryRow{
				row(agentA, true, "1.0.0", "1.0.0"),
				row(agentB, true, "1.0.0", "1.0.0"),
				row(agentC, true, "1.0.0", "1.0.0"),
			},
			wantStatus:    statusInstalled,
			wantCount:     3,
			wantAgents:    []string{"agent-a", "agent-b", "agent-c"},
			wantSkill:     "neo4j-cypher-skill",
			wantAvailable: "1.0.0",
		},
		{
			name: "all installed one drift → drift",
			rows: []InventoryRow{
				row(agentA, true, "1.0.0", "1.0.0"),
				row(agentB, true, "0.9.0", "1.0.0"),
				row(agentC, true, "1.0.0", "1.0.0"),
			},
			wantStatus:    statusDrift,
			wantCount:     3,
			wantAgents:    []string{"agent-a", "agent-b", "agent-c"},
			wantSkill:     "neo4j-cypher-skill",
			wantAvailable: "1.0.0",
		},
		{
			name: "all installed one unknown no drift → unknown-version",
			rows: []InventoryRow{
				row(agentA, true, "1.0.0", "1.0.0"),
				row(agentB, true, "", "1.0.0"),
				row(agentC, true, "1.0.0", "1.0.0"),
			},
			wantStatus:    statusUnknownVersion,
			wantCount:     3,
			wantAgents:    []string{"agent-a", "agent-b", "agent-c"},
			wantSkill:     "neo4j-cypher-skill",
			wantAvailable: "1.0.0",
		},
		{
			name: "mixed installed and not-installed no drift no unknown → partial",
			rows: []InventoryRow{
				row(agentA, true, "1.0.0", "1.0.0"),
				row(agentB, false, "", "1.0.0"),
				row(agentC, true, "1.0.0", "1.0.0"),
			},
			wantStatus:    statusPartial,
			wantCount:     2,
			wantAgents:    []string{"agent-a", "agent-c"},
			wantSkill:     "neo4j-cypher-skill",
			wantAvailable: "1.0.0",
		},
		{
			name: "drift + unknown-version mix → drift wins",
			rows: []InventoryRow{
				row(agentA, true, "0.9.0", "1.0.0"),
				row(agentB, true, "", "1.0.0"),
				row(agentC, false, "", "1.0.0"),
			},
			wantStatus:    statusDrift,
			wantCount:     2,
			wantAgents:    []string{"agent-a", "agent-b"},
			wantSkill:     "neo4j-cypher-skill",
			wantAvailable: "1.0.0",
		},
		{
			name: "unknown-version + partial mix → unknown-version wins",
			rows: []InventoryRow{
				row(agentA, true, "", "1.0.0"),
				row(agentB, false, "", "1.0.0"),
				row(agentC, true, "1.0.0", "1.0.0"),
			},
			wantStatus:    statusUnknownVersion,
			wantCount:     2,
			wantAgents:    []string{"agent-a", "agent-c"},
			wantSkill:     "neo4j-cypher-skill",
			wantAvailable: "1.0.0",
		},
		{
			name: "InstalledAgents preserves input row order",
			rows: []InventoryRow{
				row(agentC, true, "1.0.0", "1.0.0"),
				row(agentA, false, "", "1.0.0"),
				row(agentB, true, "1.0.0", "1.0.0"),
			},
			wantStatus:    statusPartial,
			wantCount:     2,
			wantAgents:    []string{"agent-c", "agent-b"},
			wantSkill:     "neo4j-cypher-skill",
			wantAvailable: "1.0.0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := aggregateCatalog(tc.rows)
			assert.Equal(t, tc.wantStatus, got.Status, "status")
			assert.Equal(t, tc.wantCount, got.InstalledCount, "installed count")
			assert.Equal(t, tc.wantAgents, got.InstalledAgents, "installed agents")
			assert.Equal(t, tc.wantSkill, got.Skill, "skill")
			assert.Equal(t, tc.wantAvailable, got.AvailableVersion, "available version")
		})
	}
}

// TestBuildInventorySkipsMCPOnlyAgents keeps `skill list` and `skill check`
// row counts pinned to the skill-capable subset, so an MCP-only catalog
// entry cannot appear as a bogus row.
func TestBuildInventorySkipsMCPOnlyAgents(t *testing.T) {
	setGOOSForTest(t, "darwin")
	memFs := setupHomeWithAgents(t, filepath.FromSlash("/Users/alice"), "claude-code", "claude-desktop")

	rows := BuildInventory(memFs, skillNameForTests, "1.7.0", nil)
	require.Len(t, rows, len(SkillAgents()))
	for _, r := range rows {
		assert.True(t, r.Agent.SupportsSkills(), "row for non-skill agent %q", r.Agent.Name)
	}
}
