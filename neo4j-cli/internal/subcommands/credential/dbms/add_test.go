// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms_test

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/credential/dbms"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/neo4j/cli/test/utils/testjson"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// dbmsTestHelper wires dbms.NewCmd with an in-memory filesystem.
type dbmsTestHelper struct {
	out         *bytes.Buffer
	err         *bytes.Buffer
	credentials string
	fs          afero.Fs
	t           *testing.T
}

func newDbmsTestHelper(t *testing.T) dbmsTestHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	return dbmsTestHelper{
		out: bytes.NewBufferString(""),
		err: bytes.NewBufferString(""),
		credentials: `{
			"dbms": {
				"credentials": [],
				"default-credential": ""
			}
		}`,
		t: t,
	}
}

func (h *dbmsTestHelper) setCredentialsValue(key string, value interface{}) {
	creds, err := sjson.Set(h.credentials, key, value)
	assert.Nil(h.t, err)
	h.credentials = creds
}

func (h *dbmsTestHelper) executeCommand(command string) error {
	args, err := shlex.Split(command)
	assert.Nil(h.t, err)

	fs, err := testfs.GetTestFs("{}", h.credentials)
	assert.Nil(h.t, err)
	h.fs = fs

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	cmd := dbms.NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetArgs(args)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)

	return cmd.Execute()
}

func (h *dbmsTestHelper) assertCredentialsValue(key string, expected string) {
	file, err := h.fs.Open(filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json"))
	assert.Nil(h.t, err)
	defer file.Close() //nolint:errcheck // in-memory FS close error is not actionable in a defer

	out, err := io.ReadAll(file)
	assert.Nil(h.t, err)

	actual := gjson.Get(string(out), key).String()

	formattedExpected, err := testjson.FormatJson(expected, "\t")
	if err != nil {
		formattedExpected = expected
	}
	formattedActual, err := testjson.FormatJson(actual, "\t")
	if err != nil {
		formattedActual = actual
	}

	assert.Equal(h.t, formattedExpected, formattedActual)
}

func (h *dbmsTestHelper) assertErr(expected string) {
	out, err := io.ReadAll(h.err)
	assert.Nil(h.t, err)
	assert.Contains(h.t, string(out), expected)
}

// --- add tests ---

func TestDbmsCredentialAdd(t *testing.T) {
	tests := []struct {
		name            string
		initialCreds    []map[string]interface{}
		initialDefault  string
		command         string
		wantErr         string
		wantCredentials string
		wantDefaultCred string
	}{
		{
			name:            "first credential is stored and set as default",
			initialCreds:    []map[string]interface{}{},
			initialDefault:  "",
			command:         "add --name mydb --username neo4j --password secret --uri bolt://localhost:7687",
			wantCredentials: `[{"name":"mydb","username":"neo4j","password":"secret","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false}]`,
			wantDefaultCred: "mydb",
		},
		{
			name: "duplicate name returns an error",
			initialCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687", "insecure": false},
			},
			initialDefault: "mydb",
			command:        "add --name mydb --username neo4j --password secret --uri bolt://localhost:7687",
			wantErr:        "already have credential with name mydb",
		},
		{
			name:            "custom database-name is stored",
			initialCreds:    []map[string]interface{}{},
			initialDefault:  "",
			command:         "add --name mydb --username neo4j --password secret --uri bolt://localhost:7687 --database-name mydb",
			wantCredentials: `[{"name":"mydb","username":"neo4j","password":"secret","database-name":"mydb","uri":"bolt://localhost:7687","insecure":false}]`,
			wantDefaultCred: "mydb",
		},
		{
			name:            "insecure flag stored as true",
			initialCreds:    []map[string]interface{}{},
			initialDefault:  "",
			command:         "add --name mydb --username neo4j --password secret --uri http://localhost:7474 --insecure",
			wantCredentials: `[{"name":"mydb","username":"neo4j","password":"secret","database-name":"neo4j","uri":"http://localhost:7474","insecure":true}]`,
			wantDefaultCred: "mydb",
		},
		{
			name: "second credential does not override existing default",
			initialCreds: []map[string]interface{}{
				{"name": "first", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687", "insecure": false},
			},
			initialDefault:  "first",
			command:         "add --name second --username neo4j --password secret2 --uri bolt://localhost:7688",
			wantCredentials: `[{"name":"first","username":"neo4j","password":"secret","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false},{"name":"second","username":"neo4j","password":"secret2","database-name":"neo4j","uri":"bolt://localhost:7688","insecure":false}]`,
			wantDefaultCred: "first",
		},
		{
			name:         "missing --name produces usage error",
			initialCreds: []map[string]interface{}{},
			command:      "add --username neo4j --password secret --uri bolt://localhost:7687",
			wantErr:      `required flag(s) "name" not set`,
		},
		{
			name:         "missing --username produces usage error",
			initialCreds: []map[string]interface{}{},
			command:      "add --name mydb --password secret --uri bolt://localhost:7687",
			wantErr:      `required flag(s) "username" not set`,
		},
		{
			name:         "missing --password produces usage error",
			initialCreds: []map[string]interface{}{},
			command:      "add --name mydb --username neo4j --uri bolt://localhost:7687",
			wantErr:      `required flag(s) "password" not set`,
		},
		{
			name:         "missing --uri produces usage error",
			initialCreds: []map[string]interface{}{},
			command:      "add --name mydb --username neo4j --password secret",
			wantErr:      `required flag(s) "uri" not set`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
			h.assertCredentialsValue("dbms.default-credential", tc.wantDefaultCred)
		})
	}
}
