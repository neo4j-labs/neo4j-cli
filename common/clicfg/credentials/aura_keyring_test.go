// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time checks: all three concrete types must satisfy keyringCredential.
var _ keyringCredential = (*AuraCredential)(nil)
var _ keyringCredential = (*DbmsCredential)(nil)
var _ keyringCredential = (*EmbedCredential)(nil)

// internalMock is a simple in-memory KeyringProvider for internal (white-box) tests.
type internalMock struct {
	store map[string]map[string]string
}

func newInternalMock() *internalMock {
	return &internalMock{store: make(map[string]map[string]string)}
}

func (m *internalMock) Get(service, user string) (string, error) {
	if svc, ok := m.store[service]; ok {
		if v, ok := svc[user]; ok {
			return v, nil
		}
	}
	return "", ErrNotFound
}

func (m *internalMock) Set(service, user, password string) error {
	if m.store[service] == nil {
		m.store[service] = make(map[string]string)
	}
	m.store[service][user] = password
	return nil
}

func (m *internalMock) Delete(service, user string) error {
	if svc, ok := m.store[service]; ok {
		if _, ok := svc[user]; ok {
			delete(svc, user)
			return nil
		}
	}
	return ErrNotFound
}

// errSetProvider always fails Set with a sentinel error.
type errSetProvider struct{ inner *internalMock }

func (e *errSetProvider) Get(s, u string) (string, error) { return e.inner.Get(s, u) }
func (e *errSetProvider) Set(_, _, _ string) error        { return errors.New("set failed") }
func (e *errSetProvider) Delete(s, u string) error        { return e.inner.Delete(s, u) }

// errGetProvider always fails Get with a non-ErrNotFound error.
type errGetProvider struct{}

func (e *errGetProvider) Get(_, _ string) (string, error) { return "", errors.New("daemon down") }
func (e *errGetProvider) Set(_, _, _ string) error        { return nil }
func (e *errGetProvider) Delete(_, _ string) error        { return nil }

// errDeleteProvider always fails Delete with a non-ErrNotFound error.
type errDeleteProvider struct{ inner *internalMock }

func (e *errDeleteProvider) Get(s, u string) (string, error) { return e.inner.Get(s, u) }
func (e *errDeleteProvider) Set(s, u, p string) error        { return e.inner.Set(s, u, p) }
func (e *errDeleteProvider) Delete(_, _ string) error        { return errors.New("delete failed") }

// --- AuraCredential tests ---

func TestAuraCredential_ZeroSensitiveFields(t *testing.T) {
	c := &AuraCredential{Name: "prod", ClientSecret: "s3cr3t", AccessToken: "tok"}
	c.zeroSensitiveFields()
	assert.Equal(t, "", c.ClientSecret)
	assert.Equal(t, "", c.AccessToken)
	assert.Equal(t, "prod", c.Name, "Name must be unchanged")
}

func TestAuraCredential_WriteToKeyring_WritesNonEmpty(t *testing.T) {
	mock := newInternalMock()
	c := &AuraCredential{Name: "prod", ClientSecret: "s3cr3t", AccessToken: "tok"}
	require.NoError(t, c.writeToKeyring(mock, nil))

	v, err := mock.Get(ServiceName, KeyringKey("aura", "prod", "client-secret"))
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", v)

	v, err = mock.Get(ServiceName, KeyringKey("aura", "prod", "access-token"))
	require.NoError(t, err)
	assert.Equal(t, "tok", v)
}

func TestAuraCredential_WriteToKeyring_SkipsEmpty(t *testing.T) {
	mock := newInternalMock()
	c := &AuraCredential{Name: "prod", ClientSecret: "s3cr3t", AccessToken: ""}
	require.NoError(t, c.writeToKeyring(mock, nil))

	_, err := mock.Get(ServiceName, KeyringKey("aura", "prod", "access-token"))
	assert.ErrorIs(t, err, ErrNotFound, "empty AccessToken must not be written to keyring")
}

func TestAuraCredential_WriteToKeyring_SetError(t *testing.T) {
	c := &AuraCredential{Name: "prod", ClientSecret: "s3cr3t"}
	err := c.writeToKeyring(&errSetProvider{inner: newInternalMock()}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyring set aura/prod/client-secret")
}

func TestAuraCredential_WriteToKeyring_TracksWritten(t *testing.T) {
	mock := newInternalMock()
	c := &AuraCredential{Name: "prod", ClientSecret: "s3cr3t", AccessToken: "tok"}
	var written []string
	require.NoError(t, c.writeToKeyring(mock, &written))
	assert.Equal(t, []string{
		KeyringKey("aura", "prod", "client-secret"),
		KeyringKey("aura", "prod", "access-token"),
	}, written)
}

func TestAuraCredential_LoadFromKeyring_LoadsFromKeyring(t *testing.T) {
	mock := newInternalMock()
	require.NoError(t, mock.Set(ServiceName, KeyringKey("aura", "prod", "client-secret"), "keyring-secret"))
	require.NoError(t, mock.Set(ServiceName, KeyringKey("aura", "prod", "access-token"), "keyring-tok"))

	c := &AuraCredential{Name: "prod", ClientSecret: "json-secret", AccessToken: "json-tok"}
	migrated := c.loadFromKeyring(mock, &bytes.Buffer{})
	assert.False(t, migrated)
	assert.Equal(t, "keyring-secret", c.ClientSecret, "keyring value must overwrite JSON value")
	assert.Equal(t, "keyring-tok", c.AccessToken)
}

func TestAuraCredential_LoadFromKeyring_AutoMigrates_ClientSecret(t *testing.T) {
	mock := newInternalMock()
	c := &AuraCredential{Name: "prod", ClientSecret: "json-secret"}
	migrated := c.loadFromKeyring(mock, &bytes.Buffer{})
	assert.True(t, migrated, "auto-migration must report migrated=true")
	assert.Equal(t, "json-secret", c.ClientSecret, "in-memory value must remain")
	v, err := mock.Get(ServiceName, KeyringKey("aura", "prod", "client-secret"))
	require.NoError(t, err)
	assert.Equal(t, "json-secret", v, "value must be written to keyring")
}

func TestAuraCredential_LoadFromKeyring_AutoMigrates_AccessToken(t *testing.T) {
	mock := newInternalMock()
	// client-secret is in keyring; access-token is only in JSON
	require.NoError(t, mock.Set(ServiceName, KeyringKey("aura", "prod", "client-secret"), "ks"))
	c := &AuraCredential{Name: "prod", AccessToken: "json-tok"}
	migrated := c.loadFromKeyring(mock, &bytes.Buffer{})
	assert.True(t, migrated)
	v, err := mock.Get(ServiceName, KeyringKey("aura", "prod", "access-token"))
	require.NoError(t, err)
	assert.Equal(t, "json-tok", v)
}

func TestAuraCredential_LoadFromKeyring_MissingRequired_Warns(t *testing.T) {
	mock := newInternalMock()
	c := &AuraCredential{Name: "prod", ClientSecret: ""}
	var buf bytes.Buffer
	c.loadFromKeyring(mock, &buf)
	assert.Contains(t, buf.String(), "prod", "warning must name the credential")
	assert.Contains(t, buf.String(), "aura client-secret")
}

func TestAuraCredential_LoadFromKeyring_AutoMigrate_SetFails_NoMigrated(t *testing.T) {
	c := &AuraCredential{Name: "prod", ClientSecret: "json-secret"}
	migrated := c.loadFromKeyring(&errSetProvider{inner: newInternalMock()}, &bytes.Buffer{})
	assert.False(t, migrated, "failed Set must not report migrated")
	assert.Equal(t, "json-secret", c.ClientSecret, "in-memory value must remain")
}

func TestAuraCredential_LoadFromKeyring_GetError_ReturnsFalse(t *testing.T) {
	c := &AuraCredential{Name: "prod", ClientSecret: "json-secret"}
	migrated := c.loadFromKeyring(&errGetProvider{}, &bytes.Buffer{})
	assert.False(t, migrated)
}

func TestAuraCredential_MigrateFromKeyring_HappyPath(t *testing.T) {
	mock := newInternalMock()
	require.NoError(t, mock.Set(ServiceName, KeyringKey("aura", "prod", "client-secret"), "s3cr3t"))
	require.NoError(t, mock.Set(ServiceName, KeyringKey("aura", "prod", "access-token"), "tok"))

	c := &AuraCredential{Name: "prod"}
	var filled []migratedField
	require.NoError(t, c.migrateFromKeyring(mock, &filled))
	assert.Equal(t, "s3cr3t", c.ClientSecret)
	assert.Equal(t, "tok", c.AccessToken)
	assert.Len(t, filled, 2)
}

func TestAuraCredential_MigrateFromKeyring_RequiredMissing_MemoryEmpty_Error(t *testing.T) {
	mock := newInternalMock()
	c := &AuraCredential{Name: "prod"}
	var filled []migratedField
	err := c.migrateFromKeyring(mock, &filled)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prod")
	assert.Contains(t, err.Error(), "aura client-secret")
}

func TestAuraCredential_MigrateFromKeyring_RequiredMissing_MemoryNonEmpty_NoOp(t *testing.T) {
	mock := newInternalMock()
	c := &AuraCredential{Name: "prod", ClientSecret: "json-fallback"}
	var filled []migratedField
	require.NoError(t, c.migrateFromKeyring(mock, &filled), "REQ-F-018: must succeed when in-memory value is present")
	assert.Empty(t, filled, "no keyring entries were read so filled must be empty")
}

func TestAuraCredential_MigrateFromKeyring_OptionalMissing_Skipped(t *testing.T) {
	mock := newInternalMock()
	require.NoError(t, mock.Set(ServiceName, KeyringKey("aura", "prod", "client-secret"), "s3cr3t"))
	// access-token absent from keyring

	c := &AuraCredential{Name: "prod"}
	var filled []migratedField
	require.NoError(t, c.migrateFromKeyring(mock, &filled))
	assert.Equal(t, "", c.AccessToken, "missing optional access-token must be empty")
	assert.Len(t, filled, 1, "only client-secret was read from keyring")
}

func TestAuraCredential_MigrateFromKeyring_GetError_ReturnsError(t *testing.T) {
	c := &AuraCredential{Name: "prod"}
	var filled []migratedField
	err := c.migrateFromKeyring(&errGetProvider{}, &filled)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyring get aura/prod/client-secret")
}

// --- AuraCredential.deleteFromKeyring tests ---

func TestAuraCredential_DeleteFromKeyring_DeletesEntries(t *testing.T) {
	mock := newInternalMock()
	require.NoError(t, mock.Set(ServiceName, KeyringKey("aura", "prod", "client-secret"), "s3cr3t"))
	require.NoError(t, mock.Set(ServiceName, KeyringKey("aura", "prod", "access-token"), "tok"))

	c := &AuraCredential{Name: "prod"}
	require.NoError(t, c.deleteFromKeyring(mock))

	_, err := mock.Get(ServiceName, KeyringKey("aura", "prod", "client-secret"))
	assert.ErrorIs(t, err, ErrNotFound, "client-secret must be deleted")
	_, err = mock.Get(ServiceName, KeyringKey("aura", "prod", "access-token"))
	assert.ErrorIs(t, err, ErrNotFound, "access-token must be deleted")
}

func TestAuraCredential_DeleteFromKeyring_ErrNotFound_Ignored(t *testing.T) {
	mock := newInternalMock()
	c := &AuraCredential{Name: "prod"}
	require.NoError(t, c.deleteFromKeyring(mock), "ErrNotFound must be silently ignored")
}

func TestAuraCredential_DeleteFromKeyring_NonNotFoundError_Returned(t *testing.T) {
	c := &AuraCredential{Name: "prod"}
	err := c.deleteFromKeyring(&errDeleteProvider{inner: newInternalMock()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keyring delete aura/prod/client-secret")
}

func TestAuraCredential_DeleteFromKeyring_PartialFailure_BothAttempted(t *testing.T) {
	mock := newInternalMock()
	require.NoError(t, mock.Set(ServiceName, KeyringKey("aura", "prod", "access-token"), "tok"))

	// Only client-secret deletion fails; access-token deletion should still be attempted.
	failClientSecret := &failOneDeleteProvider{
		inner:   mock,
		failKey: KeyringKey("aura", "prod", "client-secret"),
	}

	c := &AuraCredential{Name: "prod"}
	err := c.deleteFromKeyring(failClientSecret)
	require.Error(t, err, "client-secret failure must be reported")

	// access-token must have been deleted despite the client-secret failure
	_, getErr := mock.Get(ServiceName, KeyringKey("aura", "prod", "access-token"))
	assert.ErrorIs(t, getErr, ErrNotFound, "access-token must be deleted even when client-secret deletion fails")
}

// failOneDeleteProvider wraps internalMock and fails Delete for a specific key.
type failOneDeleteProvider struct {
	inner   *internalMock
	failKey string
}

func (f *failOneDeleteProvider) Get(s, u string) (string, error) { return f.inner.Get(s, u) }
func (f *failOneDeleteProvider) Set(s, u, p string) error        { return f.inner.Set(s, u, p) }
func (f *failOneDeleteProvider) Delete(s, u string) error {
	if u == f.failKey {
		return errors.New("delete failed")
	}
	return f.inner.Delete(s, u)
}
