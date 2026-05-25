// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential_test

import (
	"fmt"
	"testing"

	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestRemoveCredential(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetCredentialsValue("aura.credentials", []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}})

	helper.ExecuteCommand("credential remove test --rw --yes --force")

	helper.AssertCredentialsValue("aura.credentials", "[]")
}

func TestRemoveCredentialWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetCredentialsValue("aura.credentials", []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}})

	helper.ExecuteCommand(fmt.Sprintf("credential remove %s\"\n\" --rw --yes --force", "test"))

	helper.AssertCredentialsValue("aura.credentials", "[]")
}

func TestRemoveCredentialConfirmGate(t *testing.T) {
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "aura credential remove",
		NoFlagsArgs:   "credential remove test --rw",
		BothFlagsArgs: "credential remove test --rw --yes --force",
		ResourceLabel: "credential",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			helper := testutils.NewAuraTestHelper(t)
			t.Cleanup(helper.Close)
			helper.SetCredentialsValue("aura.credentials", []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}})
			helper.SetStdin(stdin)
			err := helper.ExecuteCommandE(args)
			return confirmtest.GateRunResult{Err: err, Stderr: helper.PrintErr(), Invoked: helper.CredentialsValue("aura.credentials") == "[]"}
		},
	})
}
