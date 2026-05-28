// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDbmsCredential_ZeroSensitiveFields(t *testing.T) {
	c := &DbmsCredential{Name: "local", Password: "p4ss"}
	c.zeroSensitiveFields()
	assert.Equal(t, "", c.Password)
	assert.Equal(t, "local", c.Name, "Name must be unchanged")
}

func TestDbmsCredential_WriteToKeyring_WritesNonEmpty(t *testing.T) {
	mock := newInternalMock()
	c := &DbmsCredential{Name: "local", Password: "p4ss"}
	require.NoError(t, c.writeToKeyring(mock, nil))

	v, err := mock.Get(ServiceName, KeyringKey("dbms", "local", "password"))
	require.NoError(t, err)
	assert.Equal(t, "p4ss", v)
}

func TestDbmsCredential_WriteToKeyring_SkipsEmpty(t *testing.T) {
	mock := newInternalMock()
	c := &DbmsCredential{Name: "local", Password: ""}
	require.NoError(t, c.writeToKeyring(mock, nil))

	_, err := mock.Get(ServiceName, KeyringKey("dbms", "local", "password"))
	assert.ErrorIs(t, err, ErrNotFound, "empty Password must not be written to keyring")
}

func TestDbmsCredential_WriteToKeyring_SetError(t *testing.T) {
	c := &DbmsCredential{Name: "local", Password: "p4ss"}
	err := c.writeToKeyring(&errSetProvider{inner: newInternalMock()}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyring set dbms/local/password")
}

func TestDbmsCredential_WriteToKeyring_TracksWritten(t *testing.T) {
	mock := newInternalMock()
	c := &DbmsCredential{Name: "local", Password: "p4ss"}
	var written []string
	require.NoError(t, c.writeToKeyring(mock, &written))
	assert.Equal(t, []string{KeyringKey("dbms", "local", "password")}, written)
}

func TestDbmsCredential_LoadFromKeyring_LoadsFromKeyring(t *testing.T) {
	mock := newInternalMock()
	require.NoError(t, mock.Set(ServiceName, KeyringKey("dbms", "local", "password"), "keyring-pass"))

	c := &DbmsCredential{Name: "local", Password: "json-pass"}
	migrated := c.loadFromKeyring(mock, &bytes.Buffer{})
	assert.False(t, migrated)
	assert.Equal(t, "keyring-pass", c.Password, "keyring value must overwrite JSON value")
}

func TestDbmsCredential_LoadFromKeyring_AutoMigrates(t *testing.T) {
	mock := newInternalMock()
	c := &DbmsCredential{Name: "local", Password: "json-pass"}
	migrated := c.loadFromKeyring(mock, &bytes.Buffer{})
	assert.True(t, migrated, "auto-migration must report migrated=true")
	assert.Equal(t, "json-pass", c.Password, "in-memory value must remain")
	v, err := mock.Get(ServiceName, KeyringKey("dbms", "local", "password"))
	require.NoError(t, err)
	assert.Equal(t, "json-pass", v)
}

func TestDbmsCredential_LoadFromKeyring_MissingRequired_Warns(t *testing.T) {
	mock := newInternalMock()
	c := &DbmsCredential{Name: "local", Password: ""}
	var buf bytes.Buffer
	c.loadFromKeyring(mock, &buf)
	assert.Contains(t, buf.String(), "local")
	assert.Contains(t, buf.String(), "dbms password")
}

func TestDbmsCredential_LoadFromKeyring_AutoMigrate_SetFails_NoMigrated(t *testing.T) {
	c := &DbmsCredential{Name: "local", Password: "json-pass"}
	migrated := c.loadFromKeyring(&errSetProvider{inner: newInternalMock()}, &bytes.Buffer{})
	assert.False(t, migrated)
	assert.Equal(t, "json-pass", c.Password, "in-memory value must remain")
}

func TestDbmsCredential_MigrateFromKeyring_HappyPath(t *testing.T) {
	mock := newInternalMock()
	require.NoError(t, mock.Set(ServiceName, KeyringKey("dbms", "local", "password"), "p4ss"))

	c := &DbmsCredential{Name: "local"}
	var filled []migratedField
	require.NoError(t, c.migrateFromKeyring(mock, &filled))
	assert.Equal(t, "p4ss", c.Password)
	assert.Len(t, filled, 1)
}

func TestDbmsCredential_MigrateFromKeyring_RequiredMissing_MemoryEmpty_Error(t *testing.T) {
	mock := newInternalMock()
	c := &DbmsCredential{Name: "local"}
	var filled []migratedField
	err := c.migrateFromKeyring(mock, &filled)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local")
	assert.Contains(t, err.Error(), "dbms password")
}

func TestDbmsCredential_MigrateFromKeyring_RequiredMissing_MemoryNonEmpty_NoOp(t *testing.T) {
	mock := newInternalMock()
	c := &DbmsCredential{Name: "local", Password: "json-fallback"}
	var filled []migratedField
	require.NoError(t, c.migrateFromKeyring(mock, &filled), "REQ-F-018: must succeed when in-memory value is present")
	assert.Empty(t, filled)
}

func TestDbmsCredential_MigrateFromKeyring_GetError_ReturnsError(t *testing.T) {
	c := &DbmsCredential{Name: "local"}
	var filled []migratedField
	err := c.migrateFromKeyring(&errGetProvider{}, &filled)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyring get dbms/local/password")
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
