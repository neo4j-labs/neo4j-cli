// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"path/filepath"

	"github.com/spf13/afero"
)

// MCPConfigEntry describes the server config written into an agent's MCP
// config file under mcpServers."neo4j-cli".
type MCPConfigEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// DefaultMCPConfigArgs are the args written into the neo4j-cli server entry.
var DefaultMCPConfigArgs = []string{"mcp", "serve"}

// MCPList returns one AgentInstall per MCP-capable agent, ordered by AGENTS.
func MCPList(fs afero.Fs) []AgentInstall {
	agents := MCPAgents()
	out := make([]AgentInstall, 0, len(agents))
	for _, a := range agents {
		row := AgentInstall{Agent: a}
		row.Detected = agentDetected(fs, a)
		installed, _ := isMCPInstalled(fs, a)
		row.Installed = installed
		// For MCP, "installed version" is the command path extracted from the
		// config; we store it so check can compare against the current binary.
		if installed {
			row.InstalledVersion = readMCPCommandPath(fs, a)
			row.InstalledHasMCPManifest = readMCPConfigEnv(fs, a)
		}
		out = append(out, row)
	}
	return out
}

// isMCPInstalled reports whether the neo4j-cli server entry is present in the
// agent's MCP config file. An absent, unparseable, or inaccessible file is
// treated as not installed (no error).
func isMCPInstalled(fs afero.Fs, a *Agent) (bool, error) {
	_, found := readMCPConfigServerEntry(fs, a)
	return found, nil
}

// readMCPCommandPath reads the command path from the neo4j-cli entry in the
// agent's MCP config file. Returns "" when the entry is absent or broken.
func readMCPCommandPath(fs afero.Fs, a *Agent) string {
	entry, found := readMCPConfigServerEntry(fs, a)
	if !found {
		return ""
	}
	cmd, _ := entry["command"].(string)
	return cmd
}

// readMCPConfigEnv reports whether the neo4j-cli entry's env block has the
// manifest marker (NEO4J_CLI_MCP_MANIFEST=1). Returns false when the entry,
// the env block, or the config file is absent or unparseable.
func readMCPConfigEnv(fs afero.Fs, a *Agent) bool {
	entry, found := readMCPConfigServerEntry(fs, a)
	if !found {
		return false
	}
	env, _ := entry["env"].(map[string]any)
	if env == nil {
		return false
	}
	marker, _ := env[EnvMCPManifest].(string)
	return marker == "1"
}

// InstallMCPConfig surgically merges the neo4j-cli server entry into the
// agent's MCP config file. Reads the existing file (or starts from {}),
// sets only mcpServers."neo4j-cli", and writes atomically via temp-file +
// rename, preserving the original file mode. Returns nil.
func InstallMCPConfig(fs afero.Fs, a *Agent, binPath string, gates MCPGates) error {
	configPath, ok := a.MCPConfigPath()
	if !ok {
		return nil
	}

	entry := map[string]any{
		"command": binPath,
		"args":    DefaultMCPConfigArgs,
		"env":     MCPServerEnv(gates),
	}

	return writeMCPEntry(fs, configPath, "neo4j-cli", entry)
}

// RemoveMCPConfig removes the neo4j-cli server entry from the agent's MCP
// config file. Idempotent: returns nil when the file or entry is absent.
func RemoveMCPConfig(fs afero.Fs, a *Agent) error {
	configPath, ok := a.MCPConfigPath()
	if !ok {
		return nil
	}
	return removeMCPEntry(fs, configPath, "neo4j-cli")
}

// writeMCPEntry surgically merges an entry under mcpServers. Reads the
// existing file (if any), sets only the named key, and writes atomically.
func writeMCPEntry(fs afero.Fs, path, serverName string, entry map[string]any) error {
	cfg := readMCPConfig(fs, path)
	if cfg == nil {
		cfg = map[string]any{}
	}

	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		cfg["mcpServers"] = servers
	}
	servers[serverName] = entry

	return writeFileAtomically(fs, path, cfg)
}

// removeMCPEntry removes a server entry from mcpServers. When the entry or
// file is absent it is a no-op (idempotent). An empty mcpServers is preserved
// (its removal is cosmetic).
func removeMCPEntry(fs afero.Fs, path, serverName string) error {
	cfg := readMCPConfig(fs, path)
	if cfg == nil {
		return nil
	}

	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		return nil
	}
	if _, exists := servers[serverName]; !exists {
		return nil
	}

	delete(servers, serverName)

	return writeFileAtomically(fs, path, cfg)
}

// readMCPConfig reads and decodes the MCP config file. Returns nil on error —
// callers decide how to handle a nil return (start fresh or abort).
func readMCPConfig(fs afero.Fs, path string) map[string]any {
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return nil
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return cfg
}

// readMCPConfigServerEntry reads the agent's MCP config and extracts the
// neo4j-cli server entry under mcpServers. Returns found=false when the
// entry, config file, or MCP capability is absent or the config is
// unparseable — error-free so readers can treat absence as "not installed".
func readMCPConfigServerEntry(fs afero.Fs, a *Agent) (entry map[string]any, found bool) {
	configPath, ok := a.MCPConfigPath()
	if !ok {
		return nil, false
	}
	cfg := readMCPConfig(fs, configPath)
	if cfg == nil {
		return nil, false
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		return nil, false
	}
	entry, exists := servers["neo4j-cli"].(map[string]any)
	if !exists {
		return nil, false
	}
	return entry, true
}

// writeFileAtomically marshals data to path via temp-file + rename in the
// same directory, preserving the original file mode. Creates parent dirs.
func writeFileAtomically(fs afero.Fs, path string, data map[string]any) error {
	dir := filepath.Dir(path)
	if err := fs.MkdirAll(dir, 0755); err != nil {
		return err
	}

	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	// A trailing newline is conventional for JSON config files.
	payload = append(payload, '\n')

	af := afero.Afero{Fs: fs}
	tmp, err := af.TempFile(dir, ".mcp-config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = fs.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = fs.Remove(tmpPath)
		return err
	}

	// Preserve the original file mode.
	if info, serr := fs.Stat(path); serr == nil {
		_ = fs.Chmod(tmpPath, info.Mode())
	}

	if err := fs.Rename(tmpPath, path); err != nil {
		_ = fs.Remove(tmpPath)
		return err
	}

	return nil
}
