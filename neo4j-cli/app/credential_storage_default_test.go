// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package app

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gokeyring "github.com/zalando/go-keyring"
)

// credentialsJSONWithAura is a minimal credentials.json with one Aura credential.
const credentialsJSONWithAura = `{
	"aura": {
		"default-credential": "prod",
		"credentials": [{"name": "prod", "client-id": "abc", "client-secret": "s3cr3t"}]
	},
	"dbms": {"credentials": []},
	"embed": {"credentials": []}
}`

// TestInitCredentialStorageDefault covers the primary behaviours of
// initCredentialStorageDefault: fresh install (no credentials), first run with
// existing credentials (upgrade notice), and no-op when the setting is already
// present in config.
func TestInitCredentialStorageDefault(t *testing.T) {
	const migrationCmd = "neo4j-cli config set credential-storage keyring --rw"

	tests := []struct {
		name               string
		configJSON         string
		credentialsJSON    string
		wantEmptyStderr    bool
		wantStderrContains []string
		wantStderrLine     string // if non-empty, must appear on a complete stderr line
		wantStorageMode    string
		wantCredStorage    string
	}{
		{
			name:            "no credentials: silently writes keyring",
			configJSON:      "{}",
			credentialsJSON: "{}",
			wantEmptyStderr: true,
			wantStorageMode: credentials.StorageModeKeyring,
			wantCredStorage: credentials.StorageModeKeyring,
		},
		{
			name:               "existing credentials: writes insecure and emits upgrade notice",
			configJSON:         "{}",
			credentialsJSON:    credentialsJSONWithAura,
			wantEmptyStderr:    false,
			wantStderrContains: []string{"plaintext", migrationCmd},
			wantStderrLine:     migrationCmd,
			wantStorageMode:    credentials.StorageModeInsecure,
			wantCredStorage:    credentials.StorageModeInsecure,
		},
		{
			name:            "already set: is a complete no-op",
			configJSON:      `{"credential-storage":"keyring"}`,
			credentialsJSON: credentialsJSONWithAura,
			wantEmptyStderr: true,
			wantStorageMode: credentials.StorageModeKeyring,
			wantCredStorage: credentials.StorageModeKeyring,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gokeyring.MockInit()

			fs, err := testfs.GetTestFs(tc.configJSON, tc.credentialsJSON)
			require.NoError(t, err)

			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

			var stderr bytes.Buffer
			initCredentialStorageDefault(cfg, &stderr)

			if tc.wantEmptyStderr {
				assert.Empty(t, stderr.String())
			}
			for _, s := range tc.wantStderrContains {
				assert.Contains(t, stderr.String(), s)
			}
			if tc.wantStderrLine != "" {
				lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
				found := false
				for _, line := range lines {
					if strings.Contains(line, tc.wantStderrLine) {
						found = true
						break
					}
				}
				assert.True(t, found, "stderr must contain %q on a complete line; got:\n%s", tc.wantStderrLine, stderr.String())
			}

			assert.Equal(t, tc.wantStorageMode, cfg.Credentials.StorageMode())
			assert.True(t, cfg.Global.CredentialStorageIsSet())
			assert.Equal(t, tc.wantCredStorage, cfg.Global.CredentialStorage())
		})
	}
}

// TestInitCredentialStorageDefault_NeverSelectsEnv verifies that the first-run
// default logic never auto-selects env mode, regardless of whether credentials
// exist. env is explicitly settable only (REQ-F: non-goal of auto-selection).
func TestInitCredentialStorageDefault_NeverSelectsEnv(t *testing.T) {
	tests := []struct {
		name            string
		credentialsJSON string
	}{
		{name: "no credentials", credentialsJSON: "{}"},
		{name: "existing credentials", credentialsJSON: credentialsJSONWithAura},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gokeyring.MockInit()

			fs, err := testfs.GetTestFs("{}", tc.credentialsJSON)
			require.NoError(t, err)

			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

			var stderr bytes.Buffer
			initCredentialStorageDefault(cfg, &stderr)

			assert.NotEqual(t, credentials.StorageModeEnv, cfg.Credentials.StorageMode())
			assert.NotEqual(t, credentials.StorageModeEnv, cfg.Global.CredentialStorage())
		})
	}
}

// TestInitCredentialStorageDefault_NoCreds_KeyringUnavailable verifies that
// when no credentials exist but the OS keyring daemon is unavailable, the
// function falls back to insecure mode and emits a warning to stderr.
func TestInitCredentialStorageDefault_NoCreds_KeyringUnavailable(t *testing.T) {
	someErr := errors.New("dbus connection refused")
	gokeyring.MockInitWithError(someErr)

	fs, err := testfs.GetTestFs("{}", "{}")
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	var stderr bytes.Buffer
	initCredentialStorageDefault(cfg, &stderr)

	// Must have written insecure (not keyring) because the probe failed.
	assert.Equal(t, credentials.StorageModeInsecure, cfg.Credentials.StorageMode())
	assert.True(t, cfg.Global.CredentialStorageIsSet())
	assert.Equal(t, credentials.StorageModeInsecure, cfg.Global.CredentialStorage())

	// Must have emitted a warning mentioning keyring unavailability.
	stderrOut := stderr.String()
	assert.Contains(t, stderrOut, "keyring")
	assert.Contains(t, stderrOut, "unavailable")
}

// TestInitCredentialStorageDefault_SubsequentRuns_NoNotice verifies that a
// second call (simulating a subsequent CLI invocation) is a no-op even when
// the config was written by the first call.
func TestInitCredentialStorageDefault_SubsequentRuns_NoNotice(t *testing.T) {
	gokeyring.MockInit()

	fs, err := testfs.GetTestFs("{}", "{}")
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	var stderr bytes.Buffer
	// First call: writes keyring silently
	initCredentialStorageDefault(cfg, &stderr)
	assert.Empty(t, stderr.String())

	// Second call on same cfg must also be a no-op (key is now set in viper)
	var stderr2 bytes.Buffer
	initCredentialStorageDefault(cfg, &stderr2)
	assert.Empty(t, stderr2.String())
}
