// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"bytes"
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

func TestEndpointAllowed(t *testing.T) {
	for raw, want := range map[string]bool{
		"http://127.0.0.1:8765/v1/feedback": true,
		"http://localhost:1/x":              true,
		"http://[::1]:8765/x":               true,
		"https://127.0.0.1/":                false,
		"http://10.0.0.1/":                  false,
		"http://example.com/":               false,
		"http://localhost.example.com/":     false,
		"http://127.0.0.1.example.com/":     false,
		"":                                  false,
		"ftp://127.0.0.1/":                  false,
		"https://api.mixpanel.com":          true,
		"https://api.mixpanel.com/":         true,
		"https://api-eu.mixpanel.com":       true,
		"https://api-in.mixpanel.com":       true,
		"http://api.mixpanel.com":           false, // https only
		"https://api.mixpanel.com:8443":     false,
		"https://api.mixpanel.com/track":    false, // no path, so the region is the only choice
		"https://api.mixpanel.com/?x=1":     false,
		"https://u:p@api.mixpanel.com":      false,
		"https://api.mixpanel.com.evil.com": false,
		"https://evilapi.mixpanel.com":      false,
		"https://mixpanel.com":              false,
		"https://example.com":               false,
		"https://api.mixpanel.com#frag":     false, // would turn /track?ip=0 into a fragment
		"https://api.mixpanel.com?":         false, // an empty query, same effect
		"https://api.mixpanel.com/%2F":      false,
		"http://127.0.0.1:8765/x#frag":      false,
		"http://127.0.0.1:8765/x?y=1":       false,
	} {
		assert.Equal(t, want, endpointAllowed(raw), raw)
	}
}

func TestSettingsDefaults(t *testing.T) {
	cfg := newTestConfig(t)
	assert.Equal(t, "", endpoint(cfg))
}

func TestDefaultEndpoint_OnlyWithAToken(t *testing.T) {
	withToken(t, "")
	assert.Equal(t, "", defaultEndpoint(), "a build without a token has no destination of its own")
	withToken(t, "some-token")
	assert.Equal(t, mixpanelBase, defaultEndpoint())
	assert.True(t, endpointAllowed(defaultEndpoint()), "the compiled-in destination must be an allowed endpoint")
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
	assert.Equal(t, mixpanelBase, fallbackEndpoint())
}

// A long-running `feedback serve` must see a revoke, a decline or a new
// authorization written by another process, not the values it loaded at start.
func TestState_IsReadFromDiskNotFromStartup(t *testing.T) {
	server := newTestConfig(t)
	other := clicfg.NewConfig(server.Fs(), "test-version")

	require.NoError(t, authorize(other, loopback, now().Add(time.Hour)))
	assert.True(t, Authorized(server, now()), "an authorization written elsewhere is seen")

	_, _, err := runCmd(t, newDisableCmd(other))
	require.NoError(t, err)
	assert.False(t, Authorized(server, now()), "a revoke written elsewhere stops the running server")

	require.NoError(t, recordDecline(other, now()))
	assert.True(t, declinedRecently(server, now()), "a decline written elsewhere is seen")
}

// An unwritable config file is an error, never a panic: the hint stays silent
// and the consent writes report failure instead of crashing `feedback serve`.
func TestState_UnwritableConfigFailsCleanly(t *testing.T) {
	base, err := testfs.GetTestFs("{}", "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(afero.NewReadOnlyFs(base), "test-version")
	cfg.Flags.SetForTest("flag.feedback-command", true)
	withSeams(t, true, claudeCodeAgent)
	prev := lookupEnv
	t.Cleanup(func() { lookupEnv = prev })
	lookupEnv = func(k string) string {
		if k == endpointEnv {
			return loopback
		}
		return ""
	}

	assert.Error(t, authorize(cfg, loopback, now().Add(time.Hour)))
	assert.Error(t, recordDecline(cfg, now()))
	assert.False(t, Authorized(cfg, now()))

	var w bytes.Buffer
	assert.NotPanics(t, func() { MaybeRegisterHint(cfg, []string{"query"}, &w) })
	assert.Empty(t, w.String(), "no hint when the shown-once marker cannot be stored")

	_, _, err = runCmd(t, newDisableCmd(cfg))
	assert.Error(t, err, "disable says so when it cannot revoke")
}

func TestTrackURL_IsBuiltFromParts(t *testing.T) {
	for ep, want := range map[string]string{
		"https://api.mixpanel.com":          "https://api.mixpanel.com/track?ip=0&verbose=1",
		"https://api-eu.mixpanel.com/":      "https://api-eu.mixpanel.com/track?ip=0&verbose=1",
		"http://127.0.0.1:8765/v1/feedback": "http://127.0.0.1:8765/v1/feedback/track?ip=0&verbose=1",
	} {
		got, ok := trackURL(ep)
		assert.True(t, ok, ep)
		assert.Equal(t, want, got)
	}
	_, ok := trackURL("https://api.mixpanel.com#frag")
	assert.False(t, ok)
}
