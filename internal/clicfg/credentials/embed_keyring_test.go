// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- EmbedCredential.sensitiveFields tests ---

func TestEmbedCredential_SensitiveFields_Keys(t *testing.T) {
	c := &EmbedCredential{Name: "openai", APIKey: "sk-key"}
	fields := c.sensitiveFields()
	require.Len(t, fields, 1)
	assert.Equal(t, KeyringKey("embed", "openai", "api-key"), fields[0].key)
	assert.False(t, fields[0].required)
}

func TestEmbedCredential_SensitiveFields_PtrPointsToField(t *testing.T) {
	c := &EmbedCredential{Name: "openai", APIKey: "sk-key"}
	fields := c.sensitiveFields()
	assert.Equal(t, "sk-key", *fields[0].ptr)
}

// --- EmbedCredential.deleteFromKeyring tests ---

func TestEmbedCredential_DeleteFromKeyring_DeletesEntry(t *testing.T) {
	mock := newInternalMock()
	require.NoError(t, mock.Set(ServiceName, KeyringKey("embed", "openai", "api-key"), "sk-key"))

	c := &EmbedCredential{Name: "openai"}
	require.NoError(t, c.deleteFromKeyring(mock))

	_, err := mock.Get(ServiceName, KeyringKey("embed", "openai", "api-key"))
	assert.ErrorIs(t, err, ErrNotFound, "api-key must be deleted")
}

func TestEmbedCredential_DeleteFromKeyring_ErrNotFound_Ignored(t *testing.T) {
	mock := newInternalMock()
	c := &EmbedCredential{Name: "openai"}
	require.NoError(t, c.deleteFromKeyring(mock), "ErrNotFound must be silently ignored")
}

func TestEmbedCredential_DeleteFromKeyring_NonNotFoundError_Returned(t *testing.T) {
	c := &EmbedCredential{Name: "openai"}
	err := c.deleteFromKeyring(&errDeleteProvider{inner: newInternalMock()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyring delete embed/openai/api-key")
}
