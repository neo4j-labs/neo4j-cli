// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
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

func TestRemoveCredentialConfirmGate_NonTTYWithoutFlags_Exit2(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetCredentialsValue("aura.credentials", []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}})

	err := helper.ExecuteCommandE("credential remove test --rw")

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("error %q missing 'pass both --yes and --force'", err.Error())
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != 2 {
		t.Fatalf("err = %v, want *clierr.CLIError with exit 2", err)
	}
	helper.AssertCredentialsValue("aura.credentials", `[{"client-id":"testclientid","client-secret":"testclientsecret","name":"test"}]`)
}

func TestRemoveCredentialConfirmGate_NonTTYWithBothFlags_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetCredentialsValue("aura.credentials", []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}})

	helper.ExecuteCommand("credential remove test --rw --yes --force")

	helper.AssertCredentialsValue("aura.credentials", "[]")
}

func TestRemoveCredentialConfirmGate_TTYAnswerY_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetStdin("y\n")
	helper.SetCredentialsValue("aura.credentials", []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}})

	helper.ExecuteCommand("credential remove test --rw")

	helper.AssertCredentialsValue("aura.credentials", "[]")
	helper.AssertErrContainsStrings([]string{"Delete credential"})
}

func TestRemoveCredentialConfirmGate_TTYAnswerN_Cancels(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetStdin("N\n")
	helper.SetCredentialsValue("aura.credentials", []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}})

	err := helper.ExecuteCommandE("credential remove test --rw")
	if !errors.Is(err, confirm.ErrCancelled) {
		t.Fatalf("expected confirm.ErrCancelled on cancel, got %v", err)
	}

	helper.AssertCredentialsValue("aura.credentials", `[{"client-id":"testclientid","client-secret":"testclientsecret","name":"test"}]`)
	helper.AssertErrContainsStrings([]string{"cancelled."})
}
