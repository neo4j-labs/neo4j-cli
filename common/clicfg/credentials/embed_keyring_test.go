// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbedCredential_ZeroSensitiveFields(t *testing.T) {
	c := &EmbedCredential{Name: "openai", APIKey: "sk-key"}
	c.zeroSensitiveFields()
	assert.Equal(t, "", c.APIKey)
	assert.Equal(t, "openai", c.Name, "Name must be unchanged")
}

func TestEmbedCredential_WriteToKeyring_WritesNonEmpty(t *testing.T) {
	mock := newInternalMock()
	c := &EmbedCredential{Name: "openai", APIKey: "sk-key"}
	require.NoError(t, c.writeToKeyring(mock))

	v, err := mock.Get(ServiceName, KeyringKey("embed", "openai", "api-key"))
	require.NoError(t, err)
	assert.Equal(t, "sk-key", v)
}

func TestEmbedCredential_WriteToKeyring_SkipsEmpty(t *testing.T) {
	mock := newInternalMock()
	c := &EmbedCredential{Name: "openai", APIKey: ""}
	require.NoError(t, c.writeToKeyring(mock))

	_, err := mock.Get(ServiceName, KeyringKey("embed", "openai", "api-key"))
	assert.ErrorIs(t, err, ErrNotFound, "empty APIKey must not be written to keyring")
}

func TestEmbedCredential_WriteToKeyring_SetError(t *testing.T) {
	c := &EmbedCredential{Name: "openai", APIKey: "sk-key"}
	err := c.writeToKeyring(&errSetProvider{inner: newInternalMock()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyring set embed/openai/api-key")
}

func TestEmbedCredential_LoadFromKeyring_LoadsFromKeyring(t *testing.T) {
	mock := newInternalMock()
	require.NoError(t, mock.Set(ServiceName, KeyringKey("embed", "openai", "api-key"), "keyring-key"))

	c := &EmbedCredential{Name: "openai", APIKey: "json-key"}
	migrated := c.loadFromKeyring(mock, &bytes.Buffer{})
	assert.False(t, migrated)
	assert.Equal(t, "keyring-key", c.APIKey, "keyring value must overwrite JSON value")
}

func TestEmbedCredential_LoadFromKeyring_AutoMigrates(t *testing.T) {
	mock := newInternalMock()
	c := &EmbedCredential{Name: "openai", APIKey: "json-key"}
	migrated := c.loadFromKeyring(mock, &bytes.Buffer{})
	assert.True(t, migrated, "auto-migration must report migrated=true")
	assert.Equal(t, "json-key", c.APIKey, "in-memory value must remain")
	v, err := mock.Get(ServiceName, KeyringKey("embed", "openai", "api-key"))
	require.NoError(t, err)
	assert.Equal(t, "json-key", v)
}

func TestEmbedCredential_LoadFromKeyring_MissingOptional_NoWarn(t *testing.T) {
	mock := newInternalMock()
	c := &EmbedCredential{Name: "openai", APIKey: ""}
	var buf bytes.Buffer
	migrated := c.loadFromKeyring(mock, &buf)
	assert.False(t, migrated)
	assert.Empty(t, buf.String(), "missing optional APIKey must not produce a warning")
}

func TestEmbedCredential_LoadFromKeyring_AutoMigrate_SetFails_NoMigrated(t *testing.T) {
	c := &EmbedCredential{Name: "openai", APIKey: "json-key"}
	migrated := c.loadFromKeyring(&errSetProvider{inner: newInternalMock()}, &bytes.Buffer{})
	assert.False(t, migrated)
	assert.Equal(t, "json-key", c.APIKey, "in-memory value must remain")
}

func TestEmbedCredential_MigrateFromKeyring_HappyPath(t *testing.T) {
	mock := newInternalMock()
	require.NoError(t, mock.Set(ServiceName, KeyringKey("embed", "openai", "api-key"), "sk-key"))

	c := &EmbedCredential{Name: "openai"}
	var filled []migratedField
	require.NoError(t, c.migrateFromKeyring(mock, &filled))
	assert.Equal(t, "sk-key", c.APIKey)
	assert.Len(t, filled, 1)
}

func TestEmbedCredential_MigrateFromKeyring_OptionalMissing_Skipped(t *testing.T) {
	mock := newInternalMock()
	c := &EmbedCredential{Name: "openai"}
	var filled []migratedField
	require.NoError(t, c.migrateFromKeyring(mock, &filled), "missing optional APIKey must not error")
	assert.Equal(t, "", c.APIKey)
	assert.Empty(t, filled)
}

func TestEmbedCredential_MigrateFromKeyring_GetError_ReturnsError(t *testing.T) {
	c := &EmbedCredential{Name: "openai"}
	var filled []migratedField
	err := c.migrateFromKeyring(&errGetProvider{}, &filled)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyring get embed/openai/api-key")
}
