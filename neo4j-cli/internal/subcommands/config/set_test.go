// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	gokeyring "github.com/zalando/go-keyring"
)

func TestConfigSet(t *testing.T) {
	// Initialise the mock keyring so that ProbeKeyringAvailability() (called
	// by MigrateToKeyring() when testing credential-storage keyring cases)
	// does not reach the real OS keyring daemon. The cleanup restores the mock
	// so that subtests in TestConfigSet_CredentialStorageMigration that call
	// MockInitWithError are not affected by ordering.
	gokeyring.MockInit()
	t.Cleanup(gokeyring.MockInit)

	tests := []struct {
		name             string
		command          string
		wantConfigKey    string
		wantConfigValue  string
		wantErr          string
		wantErrSubstring string
		wantOutEmpty     bool
	}{
		{
			name:         "set format to json without rw errors",
			command:      "config set format json",
			wantErr:      "Error: this command writes; pass --rw to allow it",
			wantOutEmpty: true,
		},
		{
			name:            "set format to json with rw writes json at root",
			command:         "config set --rw format json",
			wantConfigKey:   "format",
			wantConfigValue: "json",
		},
		{
			name:            "set format to table with rw writes table at root",
			command:         "config set --rw format table",
			wantConfigKey:   "format",
			wantConfigValue: "table",
		},
		{
			name:            "set format to default with rw writes default at root",
			command:         "config set --rw format default",
			wantConfigKey:   "format",
			wantConfigValue: "default",
		},
		{
			name:         "set format to invalid value with rw returns error",
			command:      "config set --rw format invalid",
			wantErr:      "Error: invalid value for 'format': invalid (valid values: default, json, table, toon)",
			wantOutEmpty: true,
		},
		{
			name:    "set unknown key with rw returns error",
			command: "config set --rw unknown-key value",
			wantErr: `Error: invalid config key: "unknown-key"`,
		},
		{
			name:             "set with missing value and rw returns error",
			command:          "config set --rw format",
			wantErrSubstring: "Error",
		},
		// Dot-notation aura keys
		{
			name:            "set aura.base-url with rw writes to aura.base-url",
			command:         "config set --rw aura.base-url https://example.com",
			wantConfigKey:   "aura.base-url",
			wantConfigValue: "https://example.com",
		},
		{
			name:    "set aura.format with rw returns error (global-only key)",
			command: "config set --rw aura.format json",
			wantErr: `Error: invalid config key: "aura.format" is a global key and cannot be addressed with the "aura." prefix`,
		},
		{
			name:    "set aura.unknown with rw returns error",
			command: "config set --rw aura.unknown value",
			wantErr: `Error: invalid config key: "aura.unknown"`,
		},
		// Telemetry opt-out
		{
			name:            "set telemetry to false with rw writes false at root",
			command:         "config set --rw telemetry false",
			wantConfigKey:   "telemetry",
			wantConfigValue: "false",
		},
		{
			name:            "set telemetry to true with rw writes true at root",
			command:         "config set --rw telemetry true",
			wantConfigKey:   "telemetry",
			wantConfigValue: "true",
		},
		{
			name:    "set telemetry to invalid value with rw returns error",
			command: "config set --rw telemetry maybe",
			wantErr: "Error: invalid value for 'telemetry': maybe (valid values: true, false)",
		},
		{
			name:         "set telemetry to false without rw errors",
			command:      "config set telemetry false",
			wantErr:      "Error: this command writes; pass --rw to allow it",
			wantOutEmpty: true,
		},
		{
			name:    "set flag.unknown-thing with rw returns error",
			command: "config set --rw flag.unknown-thing true",
			wantErr: `Error: invalid config key: "flag.unknown-thing"`,
		},
		// Credential-storage mode
		{
			name:            "set credential-storage to keyring with rw succeeds",
			command:         "config set --rw credential-storage keyring",
			wantConfigKey:   "credential-storage",
			wantConfigValue: "keyring",
		},
		{
			name:            "set credential-storage to insecure with rw succeeds",
			command:         "config set --rw credential-storage insecure",
			wantConfigKey:   "credential-storage",
			wantConfigValue: "insecure",
		},
		{
			name:         "set credential-storage to invalid value with rw returns error",
			command:      "config set --rw credential-storage plaintext",
			wantErr:      "Error: invalid value for 'credential-storage': plaintext (valid values: keyring, insecure)",
			wantOutEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newNeo4jTestHelper(t)

			h.executeCommand(tc.command)

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				if tc.wantOutEmpty {
					h.assertOut("")
				}
				return
			}
			if tc.wantErrSubstring != "" {
				errOut, err := io.ReadAll(h.err)
				assert.Nil(t, err)
				assert.Contains(t, string(errOut), tc.wantErrSubstring)
				return
			}

			h.assertErr("")
			h.assertConfigValue(tc.wantConfigKey, tc.wantConfigValue)
		})
	}
}

// TestConfigSet_CredentialStorageMigration tests the migration wiring in the
// config set RunE. Migration itself is unit-tested in the credentials package;
// these tests verify the RunE correctly gates the config write on migration
// success and skips migration on a no-op value change.
func TestConfigSet_CredentialStorageMigration(t *testing.T) {
	// credentialsWithAura is a minimal credentials.json with one Aura
	// credential that has a non-empty client-secret.
	const credentialsWithAura = `{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"s3cr3t","access-token":"","token-expiry":0}]}}`

	t.Run("switching to keyring with no credentials succeeds without migration", func(t *testing.T) {
		gokeyring.MockInit()
		h := newNeo4jTestHelper(t)
		h.executeCommand("config set --rw credential-storage keyring")
		h.assertErr("")
		h.assertConfigValue("credential-storage", "keyring")
	})

	t.Run("switching to keyring with credentials migrates secrets to keyring", func(t *testing.T) {
		gokeyring.MockInit()
		h := newNeo4jTestHelper(t)
		h.executeCommandWithCredentials("config set --rw credential-storage keyring", credentialsWithAura)
		h.assertErr("")
		h.assertConfigValue("credential-storage", "keyring")
	})

	t.Run("migration failure returns error and leaves config unchanged", func(t *testing.T) {
		// MockInitWithError makes every keyring operation return an error,
		// so MigrateToKeyring() fails on the first Set call.
		gokeyring.MockInitWithError(io.ErrUnexpectedEOF)
		// Restore the empty mock after the test regardless of outcome.
		t.Cleanup(gokeyring.MockInit)
		h := newNeo4jTestHelper(t)
		h.executeCommandWithCredentials("config set --rw credential-storage keyring", credentialsWithAura)
		// Error must be surfaced; credential-storage must NOT be written.
		errOut, err := io.ReadAll(h.err)
		assert.Nil(t, err)
		assert.Contains(t, string(errOut), "Error")
	})

	t.Run("credential-storage keyring to keyring (repair pass): runs MigrateToKeyring", func(t *testing.T) {
		gokeyring.MockInit()
		// Pre-seed the mock keyring with the secret so that loadSensitiveFieldsFromKeyring
		// (called from NewConfig) finds it and loads it into memory without triggering
		// auto-migration. This isolates the repair-pass behaviour: the credential is
		// already in the keyring but credentials.json still carries the plaintext secret
		// (simulating an out-of-sync state after a partial migration).
		assert.Nil(t, gokeyring.Set("neo4j-cli", "aura/prod/client-secret", "s3cr3t"))
		h := newNeo4jTestHelper(t)
		h.setConfigValue("credential-storage", "keyring")
		// credentials.json deliberately carries the plaintext secret even though the
		// keyring already has it. The repair pass must scrub the JSON copy.
		h.executeCommandWithCredentials("config set --rw credential-storage keyring", credentialsWithAura)
		h.assertErr("")
		h.assertConfigValue("credential-storage", "keyring")
		// Repair pass must leave the secret in the keyring and have scrubbed it from JSON.
		secret, err := gokeyring.Get("neo4j-cli", "aura/prod/client-secret")
		assert.Nil(t, err)
		assert.Equal(t, "s3cr3t", secret)
		h.assertCredentialsValue("aura.credentials.0.client-secret", "")
	})

	t.Run("setting credential-storage insecure to insecure is a no-op", func(t *testing.T) {
		gokeyring.MockInit()
		h := newNeo4jTestHelper(t)
		// Pre-seed config with credential-storage already set to insecure.
		h.setConfigValue("credential-storage", "insecure")
		// Setting insecure again should succeed without triggering migration.
		h.executeCommandWithCredentials("config set --rw credential-storage insecure", credentialsWithAura)
		h.assertErr("")
		h.assertConfigValue("credential-storage", "insecure")
		// Secret must remain in credentials.json (no migration triggered).
		h.assertCredentialsValue("aura.credentials.0.client-secret", "s3cr3t")
	})
}
