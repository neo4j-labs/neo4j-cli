// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential_test

import (
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestRemoveCredential(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetCredentialsValue("aura.credentials", []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}})

	helper.ExecuteCommand("credential remove test --rw")

	helper.AssertCredentialsValue("aura.credentials", "[]")
}
