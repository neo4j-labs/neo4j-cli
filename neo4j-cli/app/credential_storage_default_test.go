// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package app

import (
	"bytes"
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

// TestInitCredentialStorageDefault_NoCredentials_WritesKeyring verifies that on a
// fresh install (no credentials), the function silently writes "keyring" and
// emits no notice to stderr.
func TestInitCredentialStorageDefault_NoCredentials_WritesKeyring(t *testing.T) {
	gokeyring.MockInit()

	fs, err := testfs.GetTestFs("{}", "{}")
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	var stderr bytes.Buffer
	initCredentialStorageDefault(cfg, &stderr)

	// No notice emitted
	assert.Empty(t, stderr.String())
	// Storage mode set to keyring in-memory
	assert.Equal(t, credentials.StorageModeKeyring, cfg.Credentials.StorageMode())
	// Key written to config
	assert.True(t, cfg.Global.CredentialStorageIsSet())
	assert.Equal(t, credentials.StorageModeKeyring, cfg.Global.CredentialStorage())
}

// TestInitCredentialStorageDefault_WithCredentials_WritesInsecureAndNotice verifies
// that when existing credentials are present, "insecure" is written and the
// one-time upgrade notice is printed to stderr.
func TestInitCredentialStorageDefault_WithCredentials_WritesInsecureAndNotice(t *testing.T) {
	// Use an insecure mock so the credentials can load without touching the real keyring.
	gokeyring.MockInit()

	fs, err := testfs.GetTestFs("{}", credentialsJSONWithAura)
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	var stderr bytes.Buffer
	initCredentialStorageDefault(cfg, &stderr)

	// Notice must be emitted
	notice := stderr.String()
	assert.Contains(t, notice, "plaintext")
	assert.Contains(t, notice, "neo4j-cli config set credential-storage keyring --rw")

	// Storage mode set to insecure in-memory
	assert.Equal(t, credentials.StorageModeInsecure, cfg.Credentials.StorageMode())
	// Key written to config
	assert.True(t, cfg.Global.CredentialStorageIsSet())
	assert.Equal(t, credentials.StorageModeInsecure, cfg.Global.CredentialStorage())
}

// TestInitCredentialStorageDefault_AlreadySet_IsNoop verifies that when
// "credential-storage" is already present in config.json, the function is a
// complete no-op (no notice, no mode change).
func TestInitCredentialStorageDefault_AlreadySet_IsNoop(t *testing.T) {
	gokeyring.MockInit()

	// Pre-seed the config with credential-storage already set to keyring.
	fs, err := testfs.GetTestFs(`{"credential-storage":"keyring"}`, credentialsJSONWithAura)
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	// In keyring mode with existing credentials the loader tries the keyring;
	// but the test credential JSON has client-secret present as JSON fallback.

	var stderr bytes.Buffer
	initCredentialStorageDefault(cfg, &stderr)

	// No notice, no change
	assert.Empty(t, stderr.String())
	// Mode unchanged
	assert.Equal(t, credentials.StorageModeKeyring, cfg.Credentials.StorageMode())
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

// TestInitCredentialStorageDefault_UpgradeNoticeContainsMigrationCommand checks the
// exact migration command appears in the upgrade notice.
func TestInitCredentialStorageDefault_UpgradeNoticeContainsMigrationCommand(t *testing.T) {
	gokeyring.MockInit()

	fs, err := testfs.GetTestFs("{}", credentialsJSONWithAura)
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	var stderr bytes.Buffer
	initCredentialStorageDefault(cfg, &stderr)

	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	// Must contain the exact migration command on one of the lines
	found := false
	for _, line := range lines {
		if strings.Contains(line, "neo4j-cli config set credential-storage keyring --rw") {
			found = true
			break
		}
	}
	assert.True(t, found, "upgrade notice must name the migration command exactly; got:\n%s", stderr.String())
}
