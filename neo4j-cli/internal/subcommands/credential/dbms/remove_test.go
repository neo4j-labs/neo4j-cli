// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms_test

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
	gokeyring "github.com/zalando/go-keyring"
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
			wantErr:        "could not find credential with name nonexistent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

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

// TestDbmsCredentialRemove_KeyringMode verifies that in keyring mode the
// password keyring entry is deleted after credential removal. ErrNotFound on
// delete is silently ignored and does not block the removal.
func TestDbmsCredentialRemove_KeyringMode(t *testing.T) {
	const keyringConfig = `{"credential-storage":"keyring"}`

	t.Run("removes credential and deletes password keyring entry", func(t *testing.T) {
		gokeyring.MockInit()
		h := newDbmsTestHelper(t)
		h.setCredentialsValue("dbms.credentials", []map[string]interface{}{
			{"name": "local", "username": "neo4j", "password": "p4ss", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
		})
		assert.Nil(t, gokeyring.Set("neo4j-cli", "dbms/local/password", "p4ss"))

		h.executeCommandWithConfig("remove local --yes --force", keyringConfig) //nolint:errcheck

		h.assertErr("")
		h.assertCredentialsValue("dbms.credentials", "[]")

		_, err := gokeyring.Get("neo4j-cli", "dbms/local/password")
		assert.ErrorIs(t, err, gokeyring.ErrNotFound, "keyring password entry must be deleted on remove")
	})

	t.Run("remove succeeds even when password keyring entry is absent", func(t *testing.T) {
		gokeyring.MockInit()
		h := newDbmsTestHelper(t)
		h.setCredentialsValue("dbms.credentials", []map[string]interface{}{
			{"name": "local", "username": "neo4j", "password": "p4ss", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
		})
		// No keyring entry seeded — ErrNotFound on delete must not block removal

		h.executeCommandWithConfig("remove local --yes --force", keyringConfig) //nolint:errcheck

		h.assertErr("")
		h.assertCredentialsValue("dbms.credentials", "[]")
	})

	t.Run("remove in insecure mode does not touch keyring", func(t *testing.T) {
		gokeyring.MockInit()
		h := newDbmsTestHelper(t)
		h.setCredentialsValue("dbms.credentials", []map[string]interface{}{
			{"name": "local", "username": "neo4j", "password": "p4ss", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
		})
		assert.Nil(t, gokeyring.Set("neo4j-cli", "dbms/local/password", "p4ss"))

		h.executeCommand("remove local --yes --force") //nolint:errcheck

		h.assertErr("")
		// Keyring entry must still be present (insecure mode does not clean up)
		val, err := gokeyring.Get("neo4j-cli", "dbms/local/password")
		assert.NoError(t, err)
		assert.Equal(t, "p4ss", val)
	})
}

func TestDbmsCredentialRemove_ConfirmGate(t *testing.T) {
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "credential dbms remove",
		NoFlagsArgs:   "remove mydb",
		BothFlagsArgs: "remove mydb --yes --force",
		ResourceLabel: "dbms",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			h := newDbmsTestHelper(t)
			h.setCredentialsValue("dbms.credentials", []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
			})
			h.setStdin(stdin)
			err := h.executeCommand(args)
			file, ferr := h.fs.Open(filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json"))
			if ferr != nil {
				t.Fatalf("open credentials: %v", ferr)
			}
			defer file.Close() //nolint:errcheck // in-memory FS close error is not actionable in a defer
			contents, rerr := io.ReadAll(file)
			if rerr != nil {
				t.Fatalf("read credentials: %v", rerr)
			}
			invoked := gjson.Get(string(contents), "dbms.credentials").String() == "[]"
			return confirmtest.GateRunResult{Err: err, Stderr: h.err.String(), Invoked: invoked}
		},
	})
}
