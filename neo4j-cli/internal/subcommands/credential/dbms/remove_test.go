// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
)

func TestDbmsCredentialRemove(t *testing.T) {
	tests := []struct {
		name            string
		initialCreds    []map[string]interface{}
		initialDefault  string
		command         string
		wantErr         string
		wantCredentials string
		wantDefault     string
	}{
		{
			name: "removes a non-default credential",
			initialCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
				{"name": "otherdb", "username": "neo4j", "password": "secret2", "database-name": "neo4j", "uri": "bolt://localhost:7688"},
			},
			initialDefault:  "mydb",
			command:         "remove otherdb --yes --force",
			wantCredentials: `[{"name":"mydb","username":"neo4j","password":"secret","database-name":"neo4j","uri":"bolt://localhost:7687"}]`,
			wantDefault:     "mydb",
		},
		{
			name: "removing the default credential clears default-credential",
			initialCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
				{"name": "otherdb", "username": "neo4j", "password": "secret2", "database-name": "neo4j", "uri": "bolt://localhost:7688"},
			},
			initialDefault:  "mydb",
			command:         "remove mydb --yes --force",
			wantCredentials: `[{"name":"otherdb","username":"neo4j","password":"secret2","database-name":"neo4j","uri":"bolt://localhost:7688"}]`,
			wantDefault:     "",
		},
		{
			name: "unknown name returns descriptive error",
			initialCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
			},
			initialDefault: "mydb",
			command:        "remove nonexistent --yes --force",
			wantErr:        "could not find credential with name nonexistent to remove",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

			h := newDbmsTestHelper(t)
			h.setCredentialsValue("dbms.credentials", tc.initialCreds)
			if tc.initialDefault != "" {
				h.setCredentialsValue("dbms.default-credential", tc.initialDefault)
			}

			h.executeCommand(tc.command) //nolint:errcheck // error checked via assertErr

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			h.assertCredentialsValue("dbms.credentials", tc.wantCredentials)
			h.assertCredentialsValue("dbms.default-credential", tc.wantDefault)
		})
	}
}

func TestDbmsCredentialRemoveConfirmGate_NonTTYWithoutFlags_Exit2(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	h := newDbmsTestHelper(t)
	h.setCredentialsValue("dbms.credentials", []map[string]interface{}{
		{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
	})

	err := h.executeCommand("remove mydb")

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
	// Credential must still be present (no mutation).
	h.assertCredentialsValue("dbms.credentials", `[{"database-name":"neo4j","name":"mydb","password":"secret","uri":"bolt://localhost:7687","username":"neo4j"}]`)
}

func TestDbmsCredentialRemoveConfirmGate_NonTTYWithBothFlags_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	h := newDbmsTestHelper(t)
	h.setCredentialsValue("dbms.credentials", []map[string]interface{}{
		{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
	})

	if err := h.executeCommand("remove mydb --yes --force"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	h.assertCredentialsValue("dbms.credentials", `[]`)
}

func TestDbmsCredentialRemoveConfirmGate_TTYAnswerY_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	h := newDbmsTestHelper(t)
	h.setCredentialsValue("dbms.credentials", []map[string]interface{}{
		{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
	})
	h.setStdin("y\n")

	if err := h.executeCommand("remove mydb"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	h.assertCredentialsValue("dbms.credentials", `[]`)
	h.assertErr("Delete dbms")
}

func TestDbmsCredentialRemoveConfirmGate_TTYAnswerN_Cancels(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	h := newDbmsTestHelper(t)
	h.setCredentialsValue("dbms.credentials", []map[string]interface{}{
		{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
	})
	h.setStdin("N\n")

	if err := h.executeCommand("remove mydb"); !errors.Is(err, confirm.ErrCancelled) {
		t.Fatalf("expected confirm.ErrCancelled on cancel, got %v", err)
	}
	// Credential is unchanged on cancellation.
	h.assertCredentialsValue("dbms.credentials", `[{"database-name":"neo4j","name":"mydb","password":"secret","uri":"bolt://localhost:7687","username":"neo4j"}]`)
	h.assertErr("cancelled.")
}
