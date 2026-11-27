// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	engine "github.com/neo4j/cli/internal/feedback"
	"testing"
	"time"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthorized(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		value      string // "" means key unset
		want       bool
		wantReason string
	}{
		{"unset", "", false, reasonNotEnabled},
		{"past", now.Add(-time.Hour).Format(time.RFC3339), false, reasonExpired},
		{"exactly now", now.Format(time.RFC3339), false, reasonExpired},
		{"future", now.Add(time.Hour).Format(time.RFC3339), true, ""},
		{"garbage", "not-a-date", false, reasonNotEnabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestConfig(t)
			if tc.value != "" {
				require.NoError(t, cfg.Global.Set(keyAuthorizedUntil, tc.value))
			}
			assert.Equal(t, tc.want, Authorized(cfg, now))
			assert.Equal(t, tc.wantReason, authState(cfg, now))
		})
	}
}

func TestSettingsDefaults(t *testing.T) {
	cfg := newTestConfig(t)
	assert.Equal(t, "", endpoint(cfg))
}

func TestFallbackEndpoint_EnvOverrideBeatsTheBuildDefault(t *testing.T) {
	withToken(t, "some-token")
	prev := lookupEnv
	t.Cleanup(func() { lookupEnv = prev })
	lookupEnv = func(k string) string {
		if k == endpointEnv {
			return "http://127.0.0.1:9"
		}
		return ""
	}
	assert.Equal(t, "http://127.0.0.1:9", fallbackEndpoint())
	lookupEnv = func(string) string { return "" }
	assert.Equal(t, engine.MixpanelBase(), fallbackEndpoint())
}

// A long-running process must see a revoke or a new authorization written by
// another process, not the values it loaded at start.
func TestState_IsReadFromDiskNotFromStartup(t *testing.T) {
	server := newTestConfig(t)
	other := clicfg.NewConfig(server.Fs(), "test-version")

	require.NoError(t, setValues(other,
		[2]string{keyEndpoint, loopback},
		[2]string{keyAuthorizedUntil, now().Add(time.Hour).Format(time.RFC3339)},
	))
	assert.True(t, Authorized(server, now()), "an authorization written elsewhere is seen")

	_, _, err := runCmd(t, newDisableCmd(other))
	require.NoError(t, err)
	assert.False(t, Authorized(server, now()), "a revoke written elsewhere is seen")
}

// An unwritable config file is an error, never a panic: the consent writes
// report failure instead of crashing.
func TestState_UnwritableConfigFailsCleanly(t *testing.T) {
	base, err := testfs.GetTestFs("{}", "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(afero.NewReadOnlyFs(base), "test-version")
	cfg.Flags.SetForTest("flag.feedback-command", true)
	withSeams(t, true, "claude-code")
	prev := lookupEnv
	t.Cleanup(func() { lookupEnv = prev })
	lookupEnv = func(k string) string {
		if k == endpointEnv {
			return loopback
		}
		return ""
	}

	assert.Error(t, setValues(cfg, [2]string{keyAuthorizedUntil, now().Add(time.Hour).Format(time.RFC3339)}))
	assert.False(t, Authorized(cfg, now()))

	_, _, err = runCmd(t, newDisableCmd(cfg))
	assert.Error(t, err, "disable says so when it cannot revoke")
}
