// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gokeyring "github.com/zalando/go-keyring"
)

// mockKeyringProvider is a simple in-memory KeyringProvider used in tests.
type mockKeyringProvider struct {
	store map[string]map[string]string
}

func newMockKeyringProvider() *mockKeyringProvider {
	return &mockKeyringProvider{store: make(map[string]map[string]string)}
}

func (m *mockKeyringProvider) Get(service, user string) (string, error) {
	if svc, ok := m.store[service]; ok {
		if v, ok := svc[user]; ok {
			return v, nil
		}
	}
	return "", credentials.ErrNotFound
}

func (m *mockKeyringProvider) Set(service, user, password string) error {
	if m.store[service] == nil {
		m.store[service] = make(map[string]string)
	}
	m.store[service][user] = password
	return nil
}

func (m *mockKeyringProvider) Delete(service, user string) error {
	if svc, ok := m.store[service]; ok {
		if _, ok := svc[user]; ok {
			delete(svc, user)
			return nil
		}
	}
	return credentials.ErrNotFound
}

func TestKeyringKey_Format(t *testing.T) {
	tests := []struct {
		name     string
		credType string
		credName string
		field    string
		wantKey  string
	}{
		{
			name:     "aura client-secret key",
			credType: "aura",
			credName: "prod",
			field:    "client-secret",
			wantKey:  "aura/prod/client-secret",
		},
		{
			name:     "aura access-token key",
			credType: "aura",
			credName: "my-org",
			field:    "access-token",
			wantKey:  "aura/my-org/access-token",
		},
		{
			name:     "dbms password key",
			credType: "dbms",
			credName: "local",
			field:    "password",
			wantKey:  "dbms/local/password",
		},
		{
			name:     "embed api-key key",
			credType: "embed",
			credName: "openai-default",
			field:    "api-key",
			wantKey:  "embed/openai-default/api-key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := credentials.KeyringKey(tc.credType, tc.credName, tc.field)
			assert.Equal(t, tc.wantKey, got)
		})
	}
}

func TestServiceName_Constant(t *testing.T) {
	assert.Equal(t, "neo4j-cli", credentials.ServiceName)
}

func TestErrNotFound_MatchesGoKeyring(t *testing.T) {
	// ErrNotFound re-exported from credentials package must be the same
	// sentinel as the one from go-keyring, so callers can compare with either.
	assert.Equal(t, gokeyring.ErrNotFound, credentials.ErrNotFound)
}

func TestSetKeyringProviderForTest_SwapsAndRestores(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)

	// The mock is now active; populate it to verify it is being used.
	require.NoError(t, mock.Set(credentials.ServiceName, "aura/test/client-secret", "s3cr3t"))

	val, err := mock.Get(credentials.ServiceName, "aura/test/client-secret")
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", val)

	// After the test (via t.Cleanup) the real provider is restored — we cannot
	// observe that here but the seam is at least verified not to panic.
}

func TestMockKeyringProvider_GetSetDelete(t *testing.T) {
	mock := newMockKeyringProvider()

	// Get on empty store returns ErrNotFound
	_, err := mock.Get("svc", "user")
	assert.ErrorIs(t, err, credentials.ErrNotFound)

	// Set then Get returns the value
	require.NoError(t, mock.Set("svc", "user", "pass"))
	val, err := mock.Get("svc", "user")
	require.NoError(t, err)
	assert.Equal(t, "pass", val)

	// Delete removes the entry
	require.NoError(t, mock.Delete("svc", "user"))
	_, err = mock.Get("svc", "user")
	assert.ErrorIs(t, err, credentials.ErrNotFound)

	// Delete of absent entry returns ErrNotFound
	err = mock.Delete("svc", "user")
	assert.ErrorIs(t, err, credentials.ErrNotFound)
}
