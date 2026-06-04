// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetect(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want bool
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
			name: "Kiro via TERM_PROGRAM substring",
			env:  map[string]string{"TERM_PROGRAM": "Kiro.app"},
			want: true,
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
			orig := getenv
			getenv = func(key string) string {
				return tc.env[key]
			}
			t.Cleanup(func() { getenv = orig })

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
		{"interactive terminal, no harness", false, true, "interactive"},
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
