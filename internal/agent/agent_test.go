// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetect(t *testing.T) {
	for _, tc := range []struct {
		name      string
		env       map[string]string
		stdoutTTY bool
		want      bool
	}{
		{
			name: "no agent env vars",
			env:  map[string]string{},
			want: false,
		},
		{
			name: "Claude Code via CLAUDECODE",
			env:  map[string]string{"CLAUDECODE": "1"},
			want: true,
		},
		{
			name: "Claude Code via CLAUDE_CODE",
			env:  map[string]string{"CLAUDE_CODE": "1"},
			want: true,
		},
		{
			name: "Replit via REPL_ID",
			env:  map[string]string{"REPL_ID": "abc"},
			want: true,
		},
		{
			name: "Gemini CLI via GEMINI_CLI",
			env:  map[string]string{"GEMINI_CLI": "1"},
			want: true,
		},
		{
			name: "Codex via CODEX_SANDBOX",
			env:  map[string]string{"CODEX_SANDBOX": "1"},
			want: true,
		},
		{
			name: "Codex via CODEX_THREAD_ID",
			env:  map[string]string{"CODEX_THREAD_ID": "thread-1"},
			want: true,
		},
		{
			name: "OpenCode via OPENCODE",
			env:  map[string]string{"OPENCODE": "1"},
			want: true,
		},
		{
			name: "Auggie via AUGMENT_AGENT",
			env:  map[string]string{"AUGMENT_AGENT": "1"},
			want: true,
		},
		{
			name: "Goose via GOOSE_PROVIDER",
			env:  map[string]string{"GOOSE_PROVIDER": "openai"},
			want: true,
		},
		{
			name: "Cursor via CURSOR_AGENT",
			env:  map[string]string{"CURSOR_AGENT": "1"},
			want: true,
		},
		{
			name: "Devin via EDITOR substring",
			env:  map[string]string{"EDITOR": "/usr/local/bin/devin-editor"},
			want: true,
		},
		{
			name: "Devin via EDITOR substring (mixed case)",
			env:  map[string]string{"EDITOR": "Devin"},
			want: true,
		},
		{
			name: "Kiro via TERM_PROGRAM substring (non-tty stdout)",
			env:  map[string]string{"TERM_PROGRAM": "Kiro.app"},
			want: true,
		},
		{
			name:      "Kiro IDE terminal with a tty stdout is not an agent",
			env:       map[string]string{"TERM_PROGRAM": "Kiro.app"},
			stdoutTTY: true,
			want:      false,
		},
		{
			name: "Junie via JUNIE_DATA",
			env:  map[string]string{"JUNIE_DATA": "1"},
			want: true,
		},
		{
			name: "Junie via JUNIE_SHIM_PATH",
			env:  map[string]string{"JUNIE_SHIM_PATH": "/opt/junie/shim"},
			want: true,
		},
		{
			name: "Copilot via COPILOT_AGENT",
			env:  map[string]string{"COPILOT_AGENT": "1"},
			want: true,
		},
		{
			name: "Copilot via COPILOT_CLI",
			env:  map[string]string{"COPILOT_CLI": "1"},
			want: true,
		},
		{
			name: "Cline via CLINE_ACTIVE",
			env:  map[string]string{"CLINE_ACTIVE": "1"},
			want: true,
		},
		{
			name: "Antigravity via ANTIGRAVITY_AGENT",
			env:  map[string]string{"ANTIGRAVITY_AGENT": "1"},
			want: true,
		},
		{
			name: "AI_AGENT with a known value",
			env:  map[string]string{"AI_AGENT": "claude-code"},
			want: true,
		},
		{
			name: "AI_AGENT with an unknown value still marks an agent",
			env:  map[string]string{"AI_AGENT": "some-future-agent"},
			want: true,
		},
		{
			name: "whitespace-only AI_AGENT is not a match",
			env:  map[string]string{"AI_AGENT": "   "},
			want: false,
		},
		{
			name: "empty AI_AGENT is not a match",
			env:  map[string]string{"AI_AGENT": ""},
			want: false,
		},
		{
			name: "IDE-only signals are not agent signals",
			env: map[string]string{
				"CURSOR_TRACE_ID":      "trace",
				"COPILOT_GITHUB_TOKEN": "token",
				"COPILOT_MODEL":        "model",
				"COPILOT_ALLOW_ALL":    "true",
			},
			want: false,
		},
		{
			name: "pi via PATH substring",
			env:  map[string]string{"PATH": "/usr/bin:/home/u/.pi/agent/bin"},
			want: true,
		},
		{
			name: "presence var set to empty string is not a match",
			env:  map[string]string{"CLAUDECODE": ""},
			want: false,
		},
		{
			name: "EDITOR without devin substring is not a match",
			env:  map[string]string{"EDITOR": "vim"},
			want: false,
		},
		{
			name: "TERM_PROGRAM without kiro substring is not a match",
			env:  map[string]string{"TERM_PROGRAM": "iTerm.app"},
			want: false,
		},
		{
			name: "PATH without .pi/agent substring is not a match",
			env:  map[string]string{"PATH": "/usr/bin:/bin"},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origGetenv := getenv
			origTTY := stdoutIsTerminal
			getenv = func(key string) string {
				return tc.env[key]
			}
			stdoutIsTerminal = func() bool { return tc.stdoutTTY }
			t.Cleanup(func() {
				getenv = origGetenv
				stdoutIsTerminal = origTTY
			})

			assert.Equal(t, tc.want, Detect())
		})
	}
}

func TestInvoker(t *testing.T) {
	for _, tc := range []struct {
		name     string
		detected bool
		tty      bool
		want     string
	}{
		{"interactive terminal, no harness", false, true, "human"},
		{"non-tty, no harness", false, false, "script"},
		{"harness with tty", true, true, "agent"},
		{"harness without tty", true, false, "agent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origGetenv := getenv
			origTTY := stdinIsTerminal
			if tc.detected {
				getenv = func(key string) string {
					if key == "CLAUDECODE" {
						return "1"
					}
					return ""
				}
			} else {
				getenv = func(string) string { return "" }
			}
			stdinIsTerminal = func() bool { return tc.tty }
			t.Cleanup(func() {
				getenv = origGetenv
				stdinIsTerminal = origTTY
			})

			assert.Equal(t, tc.want, Invoker())
		})
	}
}

func TestHarness(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"none", map[string]string{}, ""},
		{"claude via CLAUDECODE", map[string]string{"CLAUDECODE": "1"}, "claude-code"},
		{"claude via CLAUDE_CODE", map[string]string{"CLAUDE_CODE": "1"}, "claude-code"},
		{"codex via CODEX_SANDBOX", map[string]string{"CODEX_SANDBOX": "1"}, "codex"},
		{"codex via CODEX_THREAD_ID", map[string]string{"CODEX_THREAD_ID": "t"}, "codex"},
		{"gemini", map[string]string{"GEMINI_CLI": "1"}, "gemini-cli"},
		{"cursor", map[string]string{"CURSOR_AGENT": "1"}, "cursor"},
		{"opencode", map[string]string{"OPENCODE": "1"}, "opencode"},
		{"junie via JUNIE_DATA", map[string]string{"JUNIE_DATA": "1"}, "junie"},
		{"junie via JUNIE_SHIM_PATH", map[string]string{"JUNIE_SHIM_PATH": "/s"}, "junie"},
		{"copilot via COPILOT_AGENT", map[string]string{"COPILOT_AGENT": "1"}, "copilot"},
		{"copilot via COPILOT_CLI", map[string]string{"COPILOT_CLI": "1"}, "copilot"},
		{"cline via CLINE_ACTIVE", map[string]string{"CLINE_ACTIVE": "1"}, "cline"},
		{"antigravity via ANTIGRAVITY_AGENT", map[string]string{"ANTIGRAVITY_AGENT": "1"}, "antigravity"},
		{"AI_AGENT known value", map[string]string{"AI_AGENT": "codex"}, "codex"},
		{"AI_AGENT is lowercased and trimmed", map[string]string{"AI_AGENT": "  OpenCode "}, "opencode"},
		{"AI_AGENT copilot vscode alias", map[string]string{"AI_AGENT": "github_copilot_vscode_agent"}, "copilot"},
		{"AI_AGENT copilot cloud alias", map[string]string{"AI_AGENT": "github_copilot_cloud_agent"}, "copilot"},
		{"AI_AGENT unknown value is never forwarded", map[string]string{"AI_AGENT": "some-future-agent"}, ""},
		{"AI_AGENT whitespace-only", map[string]string{"AI_AGENT": "  "}, ""},
		{"AI_AGENT unknown falls back to a labelled presence var", map[string]string{"AI_AGENT": "nope", "CLAUDECODE": "1"}, "claude-code"},
		{"detected but unlabelled harness (Replit)", map[string]string{"REPL_ID": "x"}, ""},
		{"empty value is not a match", map[string]string{"CLAUDECODE": ""}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orig := getenv
			getenv = func(key string) string { return tc.env[key] }
			t.Cleanup(func() { getenv = orig })

			assert.Equal(t, tc.want, Harness())
		})
	}
}

// TestHarnessLabels_CoverPresenceVars guards the closed label set against
// drift: every label a presence var reports must be one Harness may emit.
func TestHarnessLabels_CoverPresenceVars(t *testing.T) {
	for _, v := range presenceVars {
		if v.label == "" {
			continue
		}
		assert.True(t, harnessLabels[v.label], "presence var %s labels %q, which is missing from harnessLabels", v.name, v.label)
	}
}
