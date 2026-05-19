// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential_test

import (
	"fmt"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/stretchr/testify/assert"
	gokeyring "github.com/zalando/go-keyring"
)

func TestRemoveCredential(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetCredentialsValue("aura.credentials", []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}})

	helper.ExecuteCommand("credential remove test --rw")

	helper.AssertCredentialsValue("aura.credentials", "[]")
}

func TestRemoveCredentialWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetCredentialsValue("aura.credentials", []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}})

	helper.ExecuteCommand(fmt.Sprintf("credential remove %s\"\n\" --rw", "test"))

	helper.AssertCredentialsValue("aura.credentials", "[]")
}

// TestRemoveCredential_KeyringMode verifies that in keyring mode the
// client-secret and access-token keyring entries are deleted after credential
// removal. ErrNotFound on delete is silently ignored and does not block the
// removal.
func TestRemoveCredential_KeyringMode(t *testing.T) {
	t.Run("removes credential and deletes keyring entries", func(t *testing.T) {
		gokeyring.MockInit()
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		helper.SetCredentialsValue("aura.credentials", []map[string]string{
			{"name": "prod", "client-id": "id1", "client-secret": "s3cr3t"},
		})
		helper.SetConfigValue("credential-storage", "keyring")
		assert.Nil(t, gokeyring.Set("neo4j-cli", "aura/prod/client-secret", "s3cr3t"))
		assert.Nil(t, gokeyring.Set("neo4j-cli", "aura/prod/access-token", "tok"))

		helper.ExecuteCommand("credential remove prod --rw")

		helper.AssertCredentialsValue("aura.credentials", "[]")

		_, err := gokeyring.Get("neo4j-cli", "aura/prod/client-secret")
		assert.ErrorIs(t, err, gokeyring.ErrNotFound, "keyring client-secret must be deleted on remove")
		_, err = gokeyring.Get("neo4j-cli", "aura/prod/access-token")
		assert.ErrorIs(t, err, gokeyring.ErrNotFound, "keyring access-token must be deleted on remove")
	})

	t.Run("remove succeeds even when keyring entries are already absent", func(t *testing.T) {
		gokeyring.MockInit()
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		helper.SetCredentialsValue("aura.credentials", []map[string]string{
			{"name": "prod", "client-id": "id1", "client-secret": "s3cr3t"},
		})
		helper.SetConfigValue("credential-storage", "keyring")
		// No keyring entries seeded — ErrNotFound on delete must not block removal

		helper.ExecuteCommand("credential remove prod --rw")

		helper.AssertCredentialsValue("aura.credentials", "[]")
	})

	t.Run("remove in insecure mode does not touch keyring", func(t *testing.T) {
		gokeyring.MockInit()
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		helper.SetCredentialsValue("aura.credentials", []map[string]string{
			{"name": "prod", "client-id": "id1", "client-secret": "s3cr3t"},
		})
		// No credential-storage config key — defaults to insecure mode
		assert.Nil(t, gokeyring.Set("neo4j-cli", "aura/prod/client-secret", "s3cr3t"))

		helper.ExecuteCommand("credential remove prod --rw")

		helper.AssertCredentialsValue("aura.credentials", "[]")
		// Keyring entry must still be present (insecure mode does not clean up)
		val, err := gokeyring.Get("neo4j-cli", "aura/prod/client-secret")
		assert.NoError(t, err)
		assert.Equal(t, "s3cr3t", val)
	})
}
