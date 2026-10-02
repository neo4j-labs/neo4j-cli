// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"testing"
)

func TestMCPServerEnv_CoversAllGateCombinations(t *testing.T) {
	tests := []struct {
		name                string
		gates               MCPGates
		wantFeatureFlag     string
		wantManifest        string
		wantWrites          string
		wantAura            string
		wantCredentialWrite string
	}{
		{
			name:                "all false",
			gates:               MCPGates{},
			wantWrites:          "false",
			wantAura:            "false",
			wantCredentialWrite: "false",
		},
		{
			name: "all true",
			gates: MCPGates{
				AllowWrites:          true,
				AllowAura:            true,
				AllowCredentialWrite: true,
			},
			wantWrites:          "true",
			wantAura:            "true",
			wantCredentialWrite: "true",
		},
		{
			name: "writes only",
			gates: MCPGates{
				AllowWrites: true,
			},
			wantWrites:          "true",
			wantAura:            "false",
			wantCredentialWrite: "false",
		},
		{
			name: "aura only",
			gates: MCPGates{
				AllowAura: true,
			},
			wantWrites:          "false",
			wantAura:            "true",
			wantCredentialWrite: "false",
		},
		{
			name: "credential write only",
			gates: MCPGates{
				AllowCredentialWrite: true,
			},
			wantWrites:          "false",
			wantAura:            "false",
			wantCredentialWrite: "true",
		},
		{
			name: "writes and aura",
			gates: MCPGates{
				AllowWrites: true,
				AllowAura:   true,
			},
			wantWrites:          "true",
			wantAura:            "true",
			wantCredentialWrite: "false",
		},
		{
			name: "writes and credential write",
			gates: MCPGates{
				AllowWrites:          true,
				AllowCredentialWrite: true,
			},
			wantWrites:          "true",
			wantAura:            "false",
			wantCredentialWrite: "true",
		},
		{
			name: "aura and credential write",
			gates: MCPGates{
				AllowAura:            true,
				AllowCredentialWrite: true,
			},
			wantWrites:          "false",
			wantAura:            "true",
			wantCredentialWrite: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MCPServerEnv(tt.gates)

			if len(got) != 5 {
				t.Errorf("len(got) = %d, want 5", len(got))
			}

			if got[EnvMCPFeatureFlag] != "1" {
				t.Errorf("EnvMCPFeatureFlag = %q, want \"1\"", got[EnvMCPFeatureFlag])
			}
			if got[EnvMCPManifest] != "1" {
				t.Errorf("EnvMCPManifest = %q, want \"1\"", got[EnvMCPManifest])
			}

			if got[EnvMCPAllowWrites] != tt.wantWrites {
				t.Errorf("EnvMCPAllowWrites = %q, want %q", got[EnvMCPAllowWrites], tt.wantWrites)
			}
			if got[EnvMCPAllowAura] != tt.wantAura {
				t.Errorf("EnvMCPAllowAura = %q, want %q", got[EnvMCPAllowAura], tt.wantAura)
			}
			if got[EnvMCPAllowCredentialWrite] != tt.wantCredentialWrite {
				t.Errorf("EnvMCPAllowCredentialWrite = %q, want %q", got[EnvMCPAllowCredentialWrite], tt.wantCredentialWrite)
			}
		})
	}
}
