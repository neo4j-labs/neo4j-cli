// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential_test

import (
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestAddFirstCredential(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetCredentialsValue("aura.credentials", []map[string]string{})

	helper.ExecuteCommand("credential add --name test --client-id testclientid --client-secret testclientsecret --rw")

	helper.AssertCredentialsValue("aura.credentials", `[{"name":"test","client-id":"testclientid","client-secret":"testclientsecret","access-token":"","token-expiry":0}]`)
	helper.AssertCredentialsValue("aura.default-credential", "test")
}

func TestAddCredentialIfAlreadyExists(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetCredentialsValue("aura.credentials", []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}})

	helper.ExecuteCommand("credential add --name test --client-id testclientid --client-secret testclientsecret --rw")

	helper.AssertErr("Error: already have credential with name test")
}
func TestAddAditionalCredentials(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetCredentialsValue("aura.credentials", []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}})
	helper.SetCredentialsValue("aura.default-credential", "test")

	helper.ExecuteCommand("credential add --name test-new --client-id testclientid2 --client-secret testclientsecret2 --rw")

	helper.AssertCredentialsValue("aura.credentials", `[{"name":"test","client-id":"testclientid","client-secret":"testclientsecret","access-token":"","token-expiry":0},{"name":"test-new","client-id":"testclientid2","client-secret":"testclientsecret2","access-token":"","token-expiry":0}]`)
	helper.AssertCredentialsValue("aura.default-credential", "test")
}
