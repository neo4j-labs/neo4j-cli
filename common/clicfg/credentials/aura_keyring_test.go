// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
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

// errDeleteProvider always fails Delete with a non-ErrNotFound error.
type errDeleteProvider struct{ inner *internalMock }

func (e *errDeleteProvider) Get(s, u string) (string, error) { return e.inner.Get(s, u) }
func (e *errDeleteProvider) Set(s, u, p string) error        { return e.inner.Set(s, u, p) }
func (e *errDeleteProvider) Delete(_, _ string) error        { return errors.New("delete failed") }

// --- AuraCredential.sensitiveFields tests ---

func TestAuraCredential_SensitiveFields_Keys(t *testing.T) {
	c := &AuraCredential{Name: "prod", ClientSecret: "s3cr3t", AccessToken: "tok"}
	fields := c.sensitiveFields()
	require.Len(t, fields, 2)
	assert.Equal(t, KeyringKey("aura", "prod", "client-secret"), fields[0].key)
	assert.True(t, fields[0].required)
	assert.Equal(t, KeyringKey("aura", "prod", "access-token"), fields[1].key)
	assert.False(t, fields[1].required)
}

func TestAuraCredential_SensitiveFields_PtrsPointToFields(t *testing.T) {
	c := &AuraCredential{Name: "prod", ClientSecret: "s3cr3t", AccessToken: "tok"}
	fields := c.sensitiveFields()
	assert.Equal(t, "s3cr3t", *fields[0].ptr)
	assert.Equal(t, "tok", *fields[1].ptr)
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
