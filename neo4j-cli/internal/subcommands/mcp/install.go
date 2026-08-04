// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/common/skill"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// installResult implements output.ResponseData for install/remove results.
type installResult struct {
	Agent       string `json:"agent"`
	DisplayName string `json:"display_name"`
	Method      string `json:"method"`
}

// installResults implements output.ResponseData for install/remove result lists.
type installResults []installResult

func (r installResults) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r))
	for _, row := range r {
		out = append(out, map[string]any{
			"agent":        row.Agent,
			"display_name": row.DisplayName,
			"method":       row.Method,
		})
	}
	return out
}

func (r installResults) MarshalJSON() ([]byte, error) {
	return json.Marshal([]installResult(r))
}

func newInstallCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		agentFilter string
		installAll  bool
		useBundle   bool
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install neo4j-cli as an MCP server",
		Long: "Installs neo4j-cli as an MCP server into supported agents. " +
			"By default writes mcpServers.\"neo4j-cli\" directly into the agent's " +
			"config file. Use --bundle to generate a .mcpb and open it instead " +
			"(opens the Claude Desktop install UI on macOS; falls back to " +
			"config write on other platforms when the open command is unavailable). " +
			"Use --agent <name> (case-insensitive) to scope to one agent, or omit " +
			"to install into every detected MCP-capable agent. " +
			"\n\nSupported agents: " + strings.Join(skill.MCPAgentNames(), ", "),
		Example: `# Install neo4j-cli into every detected MCP agent
	neo4j-cli mcp install --rw

	# Install into Claude Desktop by generating a .mcpb bundle and opening it
	neo4j-cli mcp install --agent claude-desktop --bundle --rw

	# Install into a single agent and emit the result as JSON
	neo4j-cli mcp install --agent claude-desktop --format json --rw`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return runInstallCmd(cfg, cmd, agentFilter, installAll, useBundle)
		},
	}

	cmd.Flags().StringVar(&agentFilter, "agent", "", "Restrict install to a single MCP agent (case-insensitive). See --help for supported agents.")
	cmd.Flags().BoolVar(&installAll, "all", false, "Install into every detected MCP agent.")
	cmd.Flags().BoolVar(&useBundle, "bundle", false, "Generate a .mcpb bundle and open it instead of writing the config directly.")
	return cmd
}

func runInstallCmd(cfg *clicfg.Config, cmd *cobra.Command, agentFilter string, installAll, useBundle bool) error {
	fs := cfg.Aura.Fs()

	if agentFilter != "" {
		a := skill.FindAgent(agentFilter)
		if a == nil {
			return clierr.NewUsageError("unknown MCP agent: %q\nvalid agents: %s", agentFilter, strings.Join(skill.MCPAgentNames(), ", "))
		}
		if !a.SupportsMCP() {
			return clierr.NewUsageError("%q is a skill-only agent; use 'skill install' instead", a.Name)
		}
		if !installAll {
			return runInstallAndRender(fs, cfg, cmd, a, useBundle)
		}
	}

	var targets []*skill.Agent
	if agentFilter != "" {
		targets = []*skill.Agent{skill.FindAgent(agentFilter)}
	} else {
		detected := skill.DetectMCPAgents(fs)
		if len(detected) == 0 {
			return clierr.NewUsageError("no MCP-capable agents detected\nvalid agents: %s", strings.Join(skill.MCPAgentNames(), ", "))
		}
		targets = detected
	}

	var rows installResults
	for _, a := range targets {
		if err := runInstallOne(fs, a, useBundle); err != nil {
			return err
		}
		method := "config"
		if useBundle {
			method = "mcpb"
		}
		rows = append(rows, installResult{
			Agent:       a.Name,
			DisplayName: a.DisplayName,
			Method:      method,
		})
	}

	renderInstallResults(cmd, cfg, rows)
	return nil
}

// runInstallAndRender installs into one agent and renders the result row.
func runInstallAndRender(fs afero.Fs, cfg *clicfg.Config, cmd *cobra.Command, a *skill.Agent, useBundle bool) error {
	if err := runInstallOne(fs, a, useBundle); err != nil {
		return err
	}
	method := "config"
	if useBundle {
		method = "mcpb"
	}
	rows := installResults{{Agent: a.Name, DisplayName: a.DisplayName, Method: method}}
	renderInstallResults(cmd, cfg, rows)
	return nil
}

func runInstallOne(fs afero.Fs, a *skill.Agent, useBundle bool) error {
	binPath, err := os.Executable()
	if err != nil {
		return clierr.NewFatalError("cannot resolve binary path: %s", err.Error())
	}
	if useBundle {
		return runInstallBundle(a, binPath)
	}
	return runInstallConfig(fs, a, binPath)
}

// runInstallBundle generates a .mcpb in a cache directory and opens it.
func runInstallBundle(a *skill.Agent, binPath string) error {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return clierr.NewFatalError("cannot resolve user cache directory: %s", err.Error())
	}
	bundleDir := filepath.Join(cacheDir, "neo4j-cli-mcp")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return clierr.NewFatalError("cannot create bundle directory: %s", err.Error())
	}
	bundlePath := filepath.Join(bundleDir, "neo4j-cli.mcpb")
	if err := GenerateBundle(bundlePath); err != nil {
		return clierr.NewFatalError("cannot generate MCP bundle: %s", err.Error())
	}
	if err := openFile(bundlePath); err != nil {
		return clierr.NewFatalError("cannot open bundle file: %s; it is at %s", err.Error(), bundlePath)
	}
	return nil
}

// runInstallConfig writes the neo4j-cli server entry into the agent's config.
func runInstallConfig(fs afero.Fs, a *skill.Agent, binPath string) error {
	return skill.InstallMCPConfig(fs, a, binPath)
}

// openFile opens a file with the system default handler (macOS open, Windows
// start, Linux xdg-open). Runs in the background (Start, not Run) so the CLI
// returns immediately rather than blocking on the desktop application.
func openFile(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

func renderInstallResults(cmd *cobra.Command, cfg *clicfg.Config, rows installResults) {
	if len(rows) == 0 && commonoutput.ResolveOutput(cmd, cfg) != "json" {
		cmd.Printf("No agents to install into.\n")
		return
	}
	commonoutput.PrintBodyMap(cmd, cfg, rows, []string{"agent", "display_name", "method"})
}
