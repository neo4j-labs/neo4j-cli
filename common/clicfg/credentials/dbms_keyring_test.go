// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- DbmsCredential.sensitiveFields tests ---

func TestDbmsCredential_SensitiveFields_Keys(t *testing.T) {
	c := &DbmsCredential{Name: "local", Password: "p4ss"}
	fields := c.sensitiveFields()
	require.Len(t, fields, 1)
	assert.Equal(t, KeyringKey("dbms", "local", "password"), fields[0].key)
	assert.True(t, fields[0].required)
}

func TestDbmsCredential_SensitiveFields_PtrPointsToField(t *testing.T) {
	c := &DbmsCredential{Name: "local", Password: "p4ss"}
	fields := c.sensitiveFields()
	assert.Equal(t, "p4ss", *fields[0].ptr)
}

// --- DbmsCredential.deleteFromKeyring tests ---

func TestDbmsCredential_DeleteFromKeyring_DeletesEntry(t *testing.T) {
	mock := newInternalMock()
	require.NoError(t, mock.Set(ServiceName, KeyringKey("dbms", "local", "password"), "p4ss"))

	c := &DbmsCredential{Name: "local"}
	require.NoError(t, c.deleteFromKeyring(mock))

	_, err := mock.Get(ServiceName, KeyringKey("dbms", "local", "password"))
	assert.ErrorIs(t, err, ErrNotFound, "password must be deleted")
}

func TestDbmsCredential_DeleteFromKeyring_ErrNotFound_Ignored(t *testing.T) {
	mock := newInternalMock()
	c := &DbmsCredential{Name: "local"}
	require.NoError(t, c.deleteFromKeyring(mock), "ErrNotFound must be silently ignored")
}

func TestDbmsCredential_DeleteFromKeyring_NonNotFoundError_Returned(t *testing.T) {
	c := &DbmsCredential{Name: "local"}
	err := c.deleteFromKeyring(&errDeleteProvider{inner: newInternalMock()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyring delete dbms/local/password")
}
