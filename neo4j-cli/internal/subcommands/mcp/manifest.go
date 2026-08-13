// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"archive/zip"
	_ "embed"
	"encoding/json"
	"os"
	"strings"

	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp/server"
)

//go:embed assets/icon.png
var iconData []byte

// mcpbManifest is the manifest.json schema for an .mcpb desktop extension.
// JSON field names are the MCPB wire format; the repo's OUTPUT casing rule
// does not apply to spec-dictated keys.
type mcpbManifest struct {
	ManifestVersion string                     `json:"manifest_version"`
	Version         string                     `json:"version"`
	Name            string                     `json:"name"`
	DisplayName     string                     `json:"display_name"`
	Description     string                     `json:"description"`
	LongDescription string                     `json:"long_description,omitempty"`
	Author          mcpbAuthor                 `json:"author"`
	Homepage        string                     `json:"homepage,omitempty"`
	Documentation   string                     `json:"documentation,omitempty"`
	Support         string                     `json:"support,omitempty"`
	License         string                     `json:"license,omitempty"`
	Keywords        []string                   `json:"keywords,omitempty"`
	Icon            string                     `json:"icon,omitempty"`
	Server          mcpbServer                 `json:"server"`
	ToolsGenerated  bool                       `json:"tools_generated"`
	Tools           []map[string]string        `json:"tools"`
	UserConfig      map[string]mcpbConfigField `json:"user_config"`
	Compatibility   mcpbCompatibility          `json:"compatibility"`
	PrivacyPolicies []string                   `json:"privacy_policies"`
}

type mcpbServer struct {
	Type       string        `json:"type"`
	EntryPoint string        `json:"entry_point"`
	MCPConfig  mcpbMCPConfig `json:"mcp_config"`
}

type mcpbMCPConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

type mcpbAuthor struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// mcpbConfigField describes one user_config field in the manifest. The Claude
// Extensions Settings sidecar is plaintext (verified), so this carries a
// selector only — never a password.
type mcpbConfigField struct {
	Type        string `json:"type"`
	Default     any    `json:"default"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

type mcpbCompatibility struct {
	Platforms []string `json:"platforms"`
}

// GenerateBundle writes an .mcpb bundle (manifest.json + README.md, no binary)
// to path. The manifest embeds this binary's absolute path so the connector
// points at the generating CLI. task-016 reuses this function to write into the
// user cache dir rather than duplicating manifest construction.
func GenerateBundle(path string) error {
	binPath, err := os.Executable()
	if err != nil {
		return err
	}

	manifest := newManifest(binPath)
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	readme := newReadme(binPath)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)

	for _, entry := range []struct {
		name string
		data []byte
	}{
		{"manifest.json", manifestJSON},
		{"README.md", []byte(readme)},
		{"icon.png", iconData},
	} {
		w, err := zw.Create(entry.name)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if _, err := w.Write(entry.data); err != nil {
			_ = zw.Close()
			return err
		}
	}

	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

// newManifest builds the manifest.json content. The icon field is deliberately
// omitted — the repo ships no image assets and third-party logo mirrors are
// not acceptable provenance for a committed brand asset (REQ-F-038a).
func newManifest(binPath string) *mcpbManifest {
	tools := make([]map[string]string, 0, len(server.ToolDefinitions()))
	for _, t := range server.ToolDefinitions() {
		tools = append(tools, map[string]string{"name": t.Name})
	}

	// Platform names follow the MCPB spec enum: darwin, win32, linux.
	// privacy_policies is a string array of URLs per the validated schema.
	return &mcpbManifest{
		ManifestVersion: "0.3",
		Version:         "1",
		Name:            "neo4j-cli",
		DisplayName:     "Neo4j CLI",
		Description:     "Neo4j CLI connector — manage your Neo4j databases, run Cypher queries, and administer Aura instances through natural language.",
		LongDescription: "Expose the neo4j-cli command-line tool to MCP clients such as Claude Desktop. Discover local and cloud Neo4j databases, read the CLI's own documentation, and execute read-only or write commands — manage Docker containers, Neo4j Desktop instances, credentials, and Aura resources. All operations run locally on your machine.",
		Author:          mcpbAuthor{Name: "Neo4j", URL: "https://neo4j.com"},
		Homepage:        "https://neo4j.sh",
		Documentation:   "https://github.com/neo4j-labs/neo4j-cli#neo4j-cli",
		Support:         "https://github.com/neo4j-labs/neo4j-cli/issues",
		License:         "Apache-2.0",
		Keywords:        []string{"neo4j", "graph", "database", "cypher", "aura", "docker", "mcp"},
		Icon:            "icon.png",
		Server: mcpbServer{
			Type:       "binary",
			EntryPoint: binPath,
			MCPConfig: mcpbMCPConfig{
				Command: "${user_config.neo4j_cli_path}",
				Args:    []string{"mcp", "serve"},
				Env: map[string]string{
					"NEO4J_CLI_FLAG_MCP_SERVER":            "1",
					"NEO4J_CLI_MCP_MANIFEST":               "1",
					"NEO4J_CLI_MCP_ALLOW_WRITES":           "${user_config.allow_writes}",
					"NEO4J_CLI_MCP_ALLOW_AURA":             "${user_config.allow_aura}",
					"NEO4J_CLI_MCP_ALLOW_CREDENTIAL_WRITE": "${user_config.allow_credential_write}",
				},
			},
		},
		ToolsGenerated: false,
		Tools:          tools,
		UserConfig: map[string]mcpbConfigField{
			"neo4j_cli_path": {
				Type:        "file",
				Default:     binPath,
				Title:       "Neo4j CLI path",
				Description: "Path to the neo4j-cli binary. Auto-set when generated.",
			},
			"allow_writes": {
				Type:        "boolean",
				Default:     false,
				Title:       "Allow writes",
				Description: "Let the assistant modify your databases. Off by default.",
			},
			"allow_aura": {
				Type:        "boolean",
				Default:     false,
				Title:       "Allow Aura provisioning",
				Description: "Let the assistant create, modify and delete Aura instances. Off by default.",
			},
			"allow_credential_write": {
				Type:        "boolean",
				Default:     false,
				Title:       "Allow credential management",
				Description: "Let the assistant add and remove stored credentials. Off by default.",
			},
			"neo4j_credential": {
				Type:        "string",
				Default:     "",
				Title:       "Credential name",
				Description: "Name of a stored dbms credential. Empty = auto-detect.",
			},
			"allowed_databases": {
				Type:        "string",
				Default:     "",
				Title:       "Allowed databases",
				Description: "Comma-separated database names the assistant may access. Empty = all.",
			},
			"result_row_limit": {
				Type:        "number",
				Default:     float64(500),
				Title:       "Result row limit",
				Description: "Maximum rows returned per query. Default 500.",
			},
		},
		Compatibility: mcpbCompatibility{
			Platforms: []string{"darwin", "win32", "linux"},
		},
		PrivacyPolicies: []string{"https://neo4j.com/privacy-policy"},
	}
}

// newReadme builds the README.md content for the bundle. The Connectors
// Directory rejects a bundle with no privacy policy, so a policy section is
// included even though this connector runs entirely locally.
func newReadme(binPath string) string {
	var b strings.Builder
	b.WriteString("# Neo4j CLI MCP Connector\n\n")
	b.WriteString("This connector exposes the `neo4j-cli` command-line tool to MCP clients such as Claude Desktop.\n\n")
	b.WriteString("## Features\n\n")
	b.WriteString("- Discover local and cloud Neo4j databases\n")
	b.WriteString("- Read CLI documentation for any command\n")
	b.WriteString("- Execute read-only and write commands against your databases\n")
	b.WriteString("- Manage Docker containers, Neo4j Desktop instances, and Aura resources\n\n")
	b.WriteString("## Privacy Policy\n\n")
	b.WriteString("This connector runs locally on your machine. It does not send any data to external servers beyond what the Neo4j CLI itself communicates with (e.g., the Neo4j Aura API for cloud operations).\n\n")
	b.WriteString("No telemetry, usage data, or personally identifiable information is collected by this connector.\n\n")
	b.WriteString("For Neo4j's privacy practices, see https://neo4j.com/privacy-policy/.\n")
	return b.String()
}
