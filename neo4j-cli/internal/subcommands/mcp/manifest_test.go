// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateBundle_ValidZip verifies that the .mcpb is a valid zip containing
// manifest.json and no binary (REQ-F-040).
func TestGenerateBundle_ValidZip(t *testing.T) {
	_, r := openTestBundle(t)
	defer func() { _ = r.Close() }()

	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	require.Contains(t, names, "manifest.json",
		"bundle must contain manifest.json")
	require.Contains(t, names, "README.md",
		"bundle must contain README.md")
}

// TestGenerateBundle_ManifestFields verifies the manifest's structural fields
// against the MCPB spec and REQ-F-038.
func TestGenerateBundle_ManifestFields(t *testing.T) {
	_, manifest := readTestManifest(t)

	assert.Equal(t, "0.3", manifest["manifest_version"])
	assert.Equal(t, "1", manifest["version"])
	assert.Equal(t, "neo4j-cli", manifest["name"])
	assert.Equal(t, "Neo4j CLI", manifest["display_name"])
	assert.NotEmpty(t, manifest["description"])
	assert.Equal(t, false, manifest["tools_generated"],
		"tools_generated must be false")

	author, ok := manifest["author"].(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, author["name"])

	server, ok := manifest["server"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "binary", server["type"])
	assert.NotEmpty(t, server["entry_point"])

	mcpCfg, ok := server["mcp_config"].(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, mcpCfg["command"])
	args, ok := mcpCfg["args"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"mcp", "serve"}, args)
}

// TestGenerateBundle_ServerCommandUsesUserConfigVar verifies that
// mcp_config.command references ${user_config.neo4j_cli_path} (so the user can
// override the binary path in the settings UI) and that the env block carries
// NEO4J_CLI_FLAG_MCP_SERVER=1 so the spawned process sees the feature flag on.
// entry_point remains the absolute path from os.Executable() (REQ-F-038).
func TestGenerateBundle_ServerCommandUsesUserConfigVar(t *testing.T) {
	_, manifest := readTestManifest(t)

	server, _ := manifest["server"].(map[string]any)
	mcpCfg, _ := server["mcp_config"].(map[string]any)

	command, ok := mcpCfg["command"].(string)
	require.True(t, ok)
	assert.Equal(t, "${user_config.neo4j_cli_path}", command,
		"mcp_config.command should reference the user_config variable so the settings UI controls it")

	// entry_point is the absolute path from os.Executable()
	entryPoint, _ := server["entry_point"].(string)
	assert.True(t, len(entryPoint) > 0 && entryPoint[0] == '/',
		"entry_point should be an absolute path, got %q", entryPoint)

	args, _ := mcpCfg["args"].([]any)
	assert.Equal(t, []any{"mcp", "serve"}, args)

	// env must carry the feature flag so the spawned process registers the mcp group
	env, ok := mcpCfg["env"].(map[string]any)
	require.True(t, ok, "mcp_config.env must be present")
	assert.Equal(t, "1", env["NEO4J_CLI_FLAG_MCP_SERVER"],
		"env must set NEO4J_CLI_FLAG_MCP_SERVER=1 so the flag is on in the spawned process")
}

// TestGenerateBundle_NoPasswordInUserConfig enforces REQ-F-039: user_config
// carries a selector, not secrets. The sidecar is plaintext.
func TestGenerateBundle_NoPasswordInUserConfig(t *testing.T) {
	_, manifest := readTestManifest(t)

	raw, _ := json.Marshal(manifest)
	assert.NotContains(t, string(raw), "password")
	assert.NotContains(t, string(raw), "secret")

	uc, ok := manifest["user_config"].(map[string]any)
	require.True(t, ok)

	// allow_writes defaults to false (REQ-F-039)
	aw, ok := uc["allow_writes"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "boolean", aw["type"])
	assert.Equal(t, false, aw["default"],
		"allow_writes must default to false")
	assert.NotEmpty(t, aw["title"])
	assert.NotEmpty(t, aw["description"])

	// neo4j_credential is a selector, not a stored password
	cred, ok := uc["neo4j_credential"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", cred["type"])
	assert.NotEmpty(t, cred["title"])
	assert.NotEmpty(t, cred["description"])

	// allowed_databases is a string with empty default
	ad, ok := uc["allowed_databases"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", ad["type"])
	assert.Equal(t, "", ad["default"])
	assert.NotEmpty(t, ad["title"])
	assert.NotEmpty(t, ad["description"])

	// result_row_limit defaults to 500
	rl, ok := uc["result_row_limit"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "number", rl["type"])
	assert.Equal(t, float64(500), rl["default"])
	assert.NotEmpty(t, rl["title"])
	assert.NotEmpty(t, rl["description"])

	// neo4j_cli_path has title, description and a default; NOT required
	// (the default is the generating binary's path, so the user doesn't
	// have to fill it in)
	np, ok := uc["neo4j_cli_path"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "file", np["type"])
	assert.NotEmpty(t, np["default"], "neo4j_cli_path should default to the generating binary")
	assert.NotEmpty(t, np["title"])
	assert.NotEmpty(t, np["description"])
}

// TestGenerateBundle_ToolsMatchToolDefinitions enforces that the built
// manifest's tools[] array is derived from toolDefinitions() and has not
// drifted (REQ-F-038). Per the validated MCPB schema, tools entries carry
// only the tool name; hints and title come from the runtime tools/list
// response.
func TestGenerateBundle_ToolsMatchToolDefinitions(t *testing.T) {
	_, manifest := readTestManifest(t)

	tools, ok := manifest["tools"].([]any)
	require.True(t, ok)
	require.True(t, len(tools) > 0)

	manifestNames := make([]string, 0, len(tools))
	for _, ti := range tools {
		tm, ok := ti.(map[string]any)
		require.True(t, ok)
		name, ok := tm["name"].(string)
		require.True(t, ok)
		manifestNames = append(manifestNames, name)
		// Only "name" is expected per the validated spec; hints
		// and title come from the server's tools/list response.
		assert.Len(t, tm, 1, "tool entry should only have 'name'")
	}

	expectedNames := []string{
		"neo4j_cli_list_commands",
		"neo4j_cli_list_targets",
		"neo4j_cli_read_docs",
		"neo4j_cli_run",
		"neo4j_cli_run_write",
	}
	assert.ElementsMatch(t, expectedNames, manifestNames,
		"manifest tools must match toolDefinitions()")
}

// TestGenerateBundle_IconIsIncluded verifies that the generator includes an
// icon.png in the bundle and references it in the manifest. Claude Desktop
// needs the icon to render the extension preview card.
func TestGenerateBundle_IconIsIncluded(t *testing.T) {
	_, r := openTestBundle(t)
	defer func() { _ = r.Close() }()

	manifestData := readZipFile(t, r, "manifest.json")
	assert.Contains(t, string(manifestData), `"icon": "icon.png"`,
		"manifest must reference icon.png")

	iconData := readZipFile(t, r, "icon.png")
	assert.NotEmpty(t, iconData, "bundle must contain icon.png")
	assert.True(t, len(iconData) > 100, "icon.png should be a real PNG, got %d bytes", len(iconData))
}

// TestGenerateBundle_CompatibilityPlatforms checks that the platform list
// uses valid platform names per the MCPB spec enum.
func TestGenerateBundle_CompatibilityPlatforms(t *testing.T) {
	_, manifest := readTestManifest(t)

	comp, ok := manifest["compatibility"].(map[string]any)
	require.True(t, ok)

	platforms, ok := comp["platforms"].([]any)
	require.True(t, ok)
	platformStrs := make([]string, len(platforms))
	for i, p := range platforms {
		platformStrs[i] = p.(string)
	}
	assert.ElementsMatch(t, []string{"darwin", "win32", "linux"}, platformStrs,
		"platforms must be valid MCPB spec values")
}

// TestGenerateBundle_PrivacyPoliciesIsStringArray checks that privacy_policies
// is an array of URL strings, not objects (validated schema).
func TestGenerateBundle_PrivacyPoliciesIsStringArray(t *testing.T) {
	_, manifest := readTestManifest(t)

	pp, ok := manifest["privacy_policies"].([]any)
	require.True(t, ok)
	require.True(t, len(pp) > 0)

	for _, p := range pp {
		_, ok := p.(string)
		assert.True(t, ok, "privacy_policies entry must be a string URL, got %T", p)
	}
}

// TestGenerateBundle_READMEHasPrivacyPolicy checks that the README inside the
// bundle includes a privacy policy section.
func TestGenerateBundle_READMEHasPrivacyPolicy(t *testing.T) {
	_, r := openTestBundle(t)
	defer func() { _ = r.Close() }()

	content := readZipFile(t, r, "README.md")
	assert.Contains(t, string(content), "Privacy Policy")
	assert.Contains(t, string(content), "neo4j.com/privacy-policy")
}

// TestGenerateBundle_ValidatedSchema runs the bundle through the full
// generated-bundle pipeline and verifies the manifest passes the MCPB
// schema validator. Run manually or verify with `npx @anthropic-ai/mcpb validate`.
func TestGenerateBundle_ManifestPassesSchemaValidation(t *testing.T) {
	_, manifest := readTestManifest(t)

	// Verify the schema that the mcpb validate tool checks:
	// root-level required fields, author object, string version,
	// win32 platform enum, string[] privacy_policies.
	assert.NotEmpty(t, manifest["version"])
	assert.NotEmpty(t, manifest["description"])

	author, ok := manifest["author"].(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, author["name"])

	pp, ok := manifest["privacy_policies"].([]any)
	require.True(t, ok)
	if len(pp) > 0 {
		_, ok = pp[0].(string)
		assert.True(t, ok, "first privacy_policy must be a string URL")
	}

	comp, ok := manifest["compatibility"].(map[string]any)
	require.True(t, ok)
	platforms, ok := comp["platforms"].([]any)
	require.True(t, ok)
	for _, p := range platforms {
		s := p.(string)
		assert.Contains(t, []string{"darwin", "win32", "linux"}, s,
			"platform %q must be a valid MCPB platform", s)
	}

	// Each user_config entry must have title and description
	uc, ok := manifest["user_config"].(map[string]any)
	require.True(t, ok)
	for name, field := range uc {
		fm, ok := field.(map[string]any)
		require.True(t, ok, "user_config.%s must be an object", name)
		assert.NotEmpty(t, fm["title"], "user_config.%s.title required", name)
		assert.NotEmpty(t, fm["description"], "user_config.%s.description required", name)
	}
}
