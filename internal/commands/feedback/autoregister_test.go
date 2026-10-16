// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"bytes"
	"testing"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// autoRig pins the agent seams to Claude Code, a known loopback endpoint and a
// fake `claude`, so MaybeAutoRegister can be driven directly.
func autoRig(t *testing.T, harness string, registered bool) (*clicfg.Config, *[]string) {
	t.Helper()
	withSeams(t, true, harness)
	calls := fakeClaude(t, registered, false)
	prev := lookupEnv
	t.Cleanup(func() { lookupEnv = prev })
	lookupEnv = func(k string) string {
		if k == endpointEnv {
			return loopback
		}
		return ""
	}
	return newTestConfig(t), calls
}

func added(calls []string) int {
	n := 0
	for _, c := range calls {
		if len(c) >= 7 && c[:7] == "mcp add" {
			n++
		}
	}
	return n
}

func TestRegisterHint_HintsOnceAndChangesNothing(t *testing.T) {
	cfg, calls := autoRig(t, "claude-code", false)
	var w bytes.Buffer
	MaybeRegisterHint(cfg, []string{"--version"}, &w)
	assert.Empty(t, *calls, "a read-only command must not touch the agent config, not even to look")
	assert.Contains(t, w.String(), "agent feedback is not set up")
	assert.Contains(t, w.String(), "neo4j-cli feedback enable")
	assert.Contains(t, w.String(), "neo4j-cli feedback register")
	assert.Equal(t, autoHinted, getString(cfg, keyAutoRegistered))
	assert.False(t, Authorized(cfg, now()))
	assert.Empty(t, endpoint(cfg), "nothing is stored either")

	w.Reset()
	MaybeRegisterHint(cfg, []string{"query", "RETURN 2"}, &w)
	assert.Empty(t, w.String(), "shown once")
}

func TestRegisterHint_StaysQuiet(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T) (*clicfg.Config, []string)
	}{
		{"unsupported agent", func(t *testing.T) (*clicfg.Config, []string) {
			cfg, _ := autoRig(t, "gemini-cli", false)
			return cfg, []string{"query"}
		}},
		{"no agent at all", func(t *testing.T) (*clicfg.Config, []string) {
			cfg, _ := autoRig(t, "", false)
			detectAgent = func() bool { return false }
			return cfg, []string{"query"}
		}},
		{"feedback command itself", func(t *testing.T) (*clicfg.Config, []string) {
			cfg, _ := autoRig(t, "claude-code", false)
			return cfg, []string{"feedback", "status"}
		}},
		{"flag off", func(t *testing.T) (*clicfg.Config, []string) {
			cfg, _ := autoRig(t, "claude-code", false)
			cfg.Flags.SetForTest("flag.feedback-command", false)
			return cfg, []string{"query"}
		}},
		{"DO_NOT_TRACK=1", func(t *testing.T) (*clicfg.Config, []string) {
			cfg, _ := autoRig(t, "claude-code", false)
			lookupEnv = func(k string) string {
				if k == "DO_NOT_TRACK" {
					return "1"
				}
				return loopback
			}
			return cfg, []string{"query"}
		}},
		{"person ran disable", func(t *testing.T) (*clicfg.Config, []string) {
			cfg, _ := autoRig(t, "claude-code", false)
			require.NoError(t, cfg.Global.Set(keyAutoRegistered, autoDisabled))
			return cfg, []string{"query"}
		}},
		{"no endpoint known", func(t *testing.T) (*clicfg.Config, []string) {
			cfg, _ := autoRig(t, "claude-code", false)
			lookupEnv = func(string) string { return "" }
			return cfg, []string{"query"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, args := tc.setup(t)
			calls := fakeClaude(t, false, false)
			var w bytes.Buffer
			MaybeRegisterHint(cfg, args, &w)
			assert.Equal(t, 0, added(*calls))
			assert.Empty(t, w.String())
		})
	}
}

func TestDisable_StopsTheHintForGood(t *testing.T) {
	cfg, calls := autoRig(t, "claude-code", false)
	_, _, err := runCmd(t, newTestApp(cfg), "feedback", "disable")
	require.NoError(t, err)
	assert.Equal(t, autoDisabled, getString(cfg, keyAutoRegistered))
	var w bytes.Buffer
	MaybeRegisterHint(cfg, []string{"query"}, &w)
	assert.Empty(t, w.String())
	assert.Equal(t, 0, added(*calls))
}

func TestRegisterAndEnable_MarkAutoRegistered(t *testing.T) {
	cfg, _ := autoRig(t, "claude-code", true)
	withSeams(t, false, "")
	_, _, err := runCmd(t, newTestApp(cfg), "feedback", "register", "--rw")
	require.NoError(t, err)
	assert.Equal(t, autoDone, getString(cfg, keyAutoRegistered))
}

func TestRegisterHint_AlsoForCodex(t *testing.T) {
	cfg, calls := autoRig(t, "codex", false)
	var w bytes.Buffer
	MaybeRegisterHint(cfg, []string{"--version"}, &w)
	assert.Contains(t, w.String(), "agent feedback is not set up")
	assert.Empty(t, *calls, "still changes nothing")
}
