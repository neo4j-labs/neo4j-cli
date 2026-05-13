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
	// files is seeded into the in-memory FS at executeCommand time. Keys are
	// paths (e.g. "/tmp/creds.txt"), values are the file contents. Use this
	// to drive --file flag tests without touching the real filesystem.
	files map[string]string
	fs    afero.Fs
	t     *testing.T
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
			},
			"embed": {
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

	for path, contents := range h.files {
		assert.Nil(h.t, fs.MkdirAll(filepath.Dir(path), 0755))
		assert.Nil(h.t, afero.WriteFile(fs, path, []byte(contents), 0600))
	}

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
		name             string
		initialCreds     []map[string]interface{}
		initialDefault   string
		initialEmbed     []map[string]interface{}
		command          string
		wantErr          string
		wantCredentials  string
		wantDefaultCred  string
		assertNoCredsAdd bool
	}{
		{
			name:            "first credential is stored and set as default",
			initialCreds:    []map[string]interface{}{},
			initialDefault:  "",
			command:         "add --name mydb --username neo4j --password secret --uri bolt://localhost:7687",
			wantCredentials: `[{"name":"mydb","username":"neo4j","password":"secret","database-name":"neo4j","uri":"bolt://localhost:7687"}]`,
			wantDefaultCred: "mydb",
		},
		{
			name: "duplicate name returns an error",
			initialCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
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
			wantCredentials: `[{"name":"mydb","username":"neo4j","password":"secret","database-name":"mydb","uri":"bolt://localhost:7687"}]`,
			wantDefaultCred: "mydb",
		},
		{
			name: "second credential does not override existing default",
			initialCreds: []map[string]interface{}{
				{"name": "first", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
			},
			initialDefault:  "first",
			command:         "add --name second --username neo4j --password secret2 --uri bolt://localhost:7688",
			wantCredentials: `[{"name":"first","username":"neo4j","password":"secret","database-name":"neo4j","uri":"bolt://localhost:7687"},{"name":"second","username":"neo4j","password":"secret2","database-name":"neo4j","uri":"bolt://localhost:7688"}]`,
			wantDefaultCred: "first",
		},
		{
			name:         "missing --name produces usage error",
			initialCreds: []map[string]interface{}{},
			command:      "add --username neo4j --password secret --uri bolt://localhost:7687",
			wantErr:      `--name is required (provide via --file as AURA_INSTANCENAME, or pass --name)`,
		},
		{
			name:         "missing --username produces usage error",
			initialCreds: []map[string]interface{}{},
			command:      "add --name mydb --password secret --uri bolt://localhost:7687",
			wantErr:      `--username is required (provide via --file as NEO4J_USERNAME, or pass --username)`,
		},
		{
			name:         "missing --password produces usage error",
			initialCreds: []map[string]interface{}{},
			command:      "add --name mydb --username neo4j --uri bolt://localhost:7687",
			wantErr:      `--password is required (provide via --file as NEO4J_PASSWORD, or pass --password)`,
		},
		{
			name:         "missing --uri produces usage error",
			initialCreds: []map[string]interface{}{},
			command:      "add --name mydb --username neo4j --password secret",
			wantErr:      `--uri is required (provide via --file as NEO4J_URI, or pass --uri)`,
		},
		{
			name:             "--embed-credential pointing at missing embed cred errors before persisting",
			initialCreds:     []map[string]interface{}{},
			initialEmbed:     []map[string]interface{}{},
			command:          "add --name mydb --username neo4j --password secret --uri bolt://localhost:7687 --embed-credential nope",
			wantErr:          `invalid --embed-credential "nope"`,
			assertNoCredsAdd: true,
		},
		{
			name:         "--embed-credential matching an existing embed cred persists link",
			initialCreds: []map[string]interface{}{},
			initialEmbed: []map[string]interface{}{
				{"name": "myembed", "provider": "openai", "model": "text-embedding-3-small", "base-url": "", "dimensions": 0, "api-key": "k"},
			},
			command:         "add --name mydb --username neo4j --password secret --uri bolt://localhost:7687 --embed-credential myembed",
			wantCredentials: `[{"name":"mydb","username":"neo4j","password":"secret","database-name":"neo4j","uri":"bolt://localhost:7687","embed-credential":"myembed"}]`,
			wantDefaultCred: "mydb",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newDbmsTestHelper(t)
			h.setCredentialsValue("dbms.credentials", tc.initialCreds)
			if tc.initialDefault != "" {
				h.setCredentialsValue("dbms.default-credential", tc.initialDefault)
			}
			if tc.initialEmbed != nil {
				h.setCredentialsValue("embed.credentials", tc.initialEmbed)
			}

			h.executeCommand(tc.command) //nolint:errcheck // error checked via assertErr

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				if tc.assertNoCredsAdd {
					// Confirm no half-creation: dbms.credentials remains the empty slice it started as.
					h.assertCredentialsValue("dbms.credentials", `[]`)
				}
				return
			}

			h.assertErr("")
			h.assertCredentialsValue("dbms.credentials", tc.wantCredentials)
			h.assertCredentialsValue("dbms.default-credential", tc.wantDefaultCred)
		})
	}
}

// auraFileContent is the verbatim Aura-exported credentials file shape from
// the CLI-75 issue: comments, blank lines, AURA_INSTANCEID (ignored), and the
// four NEO4J_* keys plus AURA_INSTANCENAME.
const auraFileContent = `# Wait 60 seconds before connecting using these details, or login to https://console.neo4j.io to validate the Aura Instance is available
NEO4J_URI=neo4j+s://abc12345.databases.neo4j.io
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=supersecret
AURA_INSTANCEID=abc12345
AURA_INSTANCENAME=Instance01
`

func TestDbmsCredentialAddFromFile(t *testing.T) {
	// Use forward-slash literal paths: shlex.Split treats backslashes as escapes
	// (POSIX-style), so a Windows-native path like `\tmp\x` would be mangled by
	// the command parser. afero.MemMapFs normalises paths via filepath.Clean,
	// so `/tmp/x` is equivalent to `\tmp\x` on Windows for storage lookup.
	credsPath := "/tmp/aura-creds.txt"
	missingPath := "/tmp/missing.txt"

	tests := []struct {
		name             string
		initialCreds     []map[string]interface{}
		initialEmbed     []map[string]interface{}
		files            map[string]string
		command          string
		wantErr          string
		wantCredentials  string
		wantDefaultCred  string
		assertNoCredsAdd bool
	}{
		{
			name:         "happy path: --file alone populates all fields from Aura export",
			initialCreds: []map[string]interface{}{},
			files:        map[string]string{credsPath: auraFileContent},
			command:      "add --file " + credsPath,
			wantCredentials: `[{"name":"Instance01","username":"neo4j","password":"supersecret",` +
				`"database-name":"neo4j","uri":"neo4j+s://abc12345.databases.neo4j.io"}]`,
			wantDefaultCred: "Instance01",
		},
		{
			name:         "--name overrides AURA_INSTANCENAME",
			initialCreds: []map[string]interface{}{},
			files:        map[string]string{credsPath: auraFileContent},
			command:      "add --file " + credsPath + " --name custom",
			wantCredentials: `[{"name":"custom","username":"neo4j","password":"supersecret",` +
				`"database-name":"neo4j","uri":"neo4j+s://abc12345.databases.neo4j.io"}]`,
			wantDefaultCred: "custom",
		},
		{
			name:         "--password overrides NEO4J_PASSWORD from file",
			initialCreds: []map[string]interface{}{},
			files:        map[string]string{credsPath: auraFileContent},
			command:      "add --file " + credsPath + " --password override",
			wantCredentials: `[{"name":"Instance01","username":"neo4j","password":"override",` +
				`"database-name":"neo4j","uri":"neo4j+s://abc12345.databases.neo4j.io"}]`,
			wantDefaultCred: "Instance01",
		},
		{
			name:         "NEO4J_DATABASE from file is honoured when --database-name not passed",
			initialCreds: []map[string]interface{}{},
			files:        map[string]string{credsPath: "NEO4J_URI=bolt://x:7687\nNEO4J_USERNAME=neo4j\nNEO4J_PASSWORD=pw\nAURA_INSTANCENAME=Inst\nNEO4J_DATABASE=mydb\n"},
			command:      "add --file " + credsPath,
			wantCredentials: `[{"name":"Inst","username":"neo4j","password":"pw",` +
				`"database-name":"mydb","uri":"bolt://x:7687"}]`,
			wantDefaultCred: "Inst",
		},
		{
			name:         "comments and blank lines parse without error",
			initialCreds: []map[string]interface{}{},
			files: map[string]string{credsPath: `# leading comment

NEO4J_URI=bolt://x:7687
# inline comment between keys

NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=pw
AURA_INSTANCENAME=Inst
AURA_INSTANCEID=abc12345
`},
			command: "add --file " + credsPath,
			wantCredentials: `[{"name":"Inst","username":"neo4j","password":"pw",` +
				`"database-name":"neo4j","uri":"bolt://x:7687"}]`,
			wantDefaultCred: "Inst",
		},
		{
			name:             "empty file errors on first missing field (name)",
			initialCreds:     []map[string]interface{}{},
			files:            map[string]string{credsPath: ""},
			command:          "add --file " + credsPath,
			wantErr:          `--name is required (provide via --file as AURA_INSTANCENAME, or pass --name)`,
			assertNoCredsAdd: true,
		},
		{
			name:             "NEO4J_URI= empty with no flag override is a usage error",
			initialCreds:     []map[string]interface{}{},
			files:            map[string]string{credsPath: "NEO4J_URI=\nNEO4J_USERNAME=neo4j\nNEO4J_PASSWORD=pw\nAURA_INSTANCENAME=Inst\n"},
			command:          "add --file " + credsPath,
			wantErr:          `NEO4J_URI has an empty value`,
			assertNoCredsAdd: true,
		},
		{
			name:         "NEO4J_URI= empty with --uri override succeeds",
			initialCreds: []map[string]interface{}{},
			files:        map[string]string{credsPath: "NEO4J_URI=\nNEO4J_USERNAME=neo4j\nNEO4J_PASSWORD=pw\nAURA_INSTANCENAME=Inst\n"},
			command:      "add --file " + credsPath + " --uri bolt://x:7687",
			wantCredentials: `[{"name":"Inst","username":"neo4j","password":"pw",` +
				`"database-name":"neo4j","uri":"bolt://x:7687"}]`,
			wantDefaultCred: "Inst",
		},
		{
			name:             "NEO4J_DATABASE= empty with no --database-name flag is an error (default NOT applied)",
			initialCreds:     []map[string]interface{}{},
			files:            map[string]string{credsPath: "NEO4J_URI=bolt://x:7687\nNEO4J_USERNAME=neo4j\nNEO4J_PASSWORD=pw\nAURA_INSTANCENAME=Inst\nNEO4J_DATABASE=\n"},
			command:          "add --file " + credsPath,
			wantErr:          `NEO4J_DATABASE has an empty value`,
			assertNoCredsAdd: true,
		},
		{
			name:             "missing --file path returns a wrapped open error",
			initialCreds:     []map[string]interface{}{},
			files:            map[string]string{},
			command:          "add --file " + missingPath,
			wantErr:          "--file \"" + missingPath + "\":",
			assertNoCredsAdd: true,
		},
		{
			name:         "--file + --embed-credential matching existing embed cred links it",
			initialCreds: []map[string]interface{}{},
			initialEmbed: []map[string]interface{}{
				{"name": "myembed", "provider": "openai", "model": "text-embedding-3-small", "base-url": "", "dimensions": 0, "api-key": "k"},
			},
			files:   map[string]string{credsPath: auraFileContent},
			command: "add --file " + credsPath + " --embed-credential myembed",
			wantCredentials: `[{"name":"Instance01","username":"neo4j","password":"supersecret",` +
				`"database-name":"neo4j","uri":"neo4j+s://abc12345.databases.neo4j.io","embed-credential":"myembed"}]`,
			wantDefaultCred: "Instance01",
		},
		{
			name:             "--file + --embed-credential pointing at missing embed errors before persisting",
			initialCreds:     []map[string]interface{}{},
			initialEmbed:     []map[string]interface{}{},
			files:            map[string]string{credsPath: auraFileContent},
			command:          "add --file " + credsPath + " --embed-credential nope",
			wantErr:          `invalid --embed-credential "nope"`,
			assertNoCredsAdd: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newDbmsTestHelper(t)
			h.setCredentialsValue("dbms.credentials", tc.initialCreds)
			if tc.initialEmbed != nil {
				h.setCredentialsValue("embed.credentials", tc.initialEmbed)
			}
			h.files = tc.files

			h.executeCommand(tc.command) //nolint:errcheck // error checked via assertErr

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				if tc.assertNoCredsAdd {
					h.assertCredentialsValue("dbms.credentials", `[]`)
				}
				return
			}

			h.assertErr("")
			h.assertCredentialsValue("dbms.credentials", tc.wantCredentials)
			h.assertCredentialsValue("dbms.default-credential", tc.wantDefaultCred)
		})
	}
}
