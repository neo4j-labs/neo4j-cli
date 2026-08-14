// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

// MCP env var names. Single-sourced in common/ so both the mcp/serve runtime
// and the manifest builder reference the same constants — the internal-package
// rule forbids common/* importing neo4j-cli/internal/*.
const (
	EnvMCPFeatureFlag          = "NEO4J_CLI_FLAG_MCP_SERVER"
	EnvMCPManifest             = "NEO4J_CLI_MCP_MANIFEST"
	EnvMCPAllowWrites          = "NEO4J_CLI_MCP_ALLOW_WRITES"
	EnvMCPAllowAura            = "NEO4J_CLI_MCP_ALLOW_AURA"
	EnvMCPAllowCredentialWrite = "NEO4J_CLI_MCP_ALLOW_CREDENTIAL_WRITE"
)

// MCPGates records which capability gates are enabled for a server instance.
type MCPGates struct {
	AllowWrites          bool
	AllowAura            bool
	AllowCredentialWrite bool
}

// MCPServerEnv builds the env map that a spawned neo4j-cli mcp serve process
// needs to honour the requested gate set.
//
// ALWAYS emits all five keys, including gates set to "false". This is
// load-bearing, not stylistic: the config env block overrides the environment
// Claude Desktop inherited from its parent process, so an explicit "false"
// neutralises a stale NEO4J_CLI_MCP_ALLOW_WRITES=true left in a login shell
// rc — exposure that setting NEO4J_CLI_MCP_MANIFEST=1 would otherwise newly
// create. Without it a future reader would be tempted to "tidy" the "false"
// entries into omitempty, silently reopening the hole.
func MCPServerEnv(g MCPGates) map[string]string {
	return map[string]string{
		EnvMCPFeatureFlag:          "1",
		EnvMCPManifest:             "1",
		EnvMCPAllowWrites:          envBoolStr(g.AllowWrites),
		EnvMCPAllowAura:            envBoolStr(g.AllowAura),
		EnvMCPAllowCredentialWrite: envBoolStr(g.AllowCredentialWrite),
	}
}

func envBoolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
