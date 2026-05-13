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

// TestAddCredentialMissingFlag covers the three required-flag cases that used
// to surface cobra's auto-generated `required flag(s)` text. After CLI-100,
// validation moved into RunE so the message names the offending flag and
// points at the equivalent --file env key.
func TestAddCredentialMissingFlag(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr string
	}{
		{
			name:    "missing --name",
			command: "credential add --client-id testclientid --client-secret testclientsecret --rw",
			wantErr: `--name is required (provide via --file as CLIENT_NAME, or pass --name)`,
		},
		{
			name:    "missing --client-id",
			command: "credential add --name test --client-secret testclientsecret --rw",
			wantErr: `--client-id is required (provide via --file as CLIENT_ID, or pass --client-id)`,
		},
		{
			name:    "missing --client-secret",
			command: "credential add --name test --client-id testclientid --rw",
			wantErr: `--client-secret is required (provide via --file as CLIENT_SECRET, or pass --client-secret)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			helper.SetCredentialsValue("aura.credentials", []map[string]string{})

			helper.ExecuteCommand(tc.command)

			helper.AssertErrContainsStrings([]string{tc.wantErr})
			helper.AssertCredentialsValue("aura.credentials", `[]`)
		})
	}
}

// auraClientFileContent is the verbatim Aura console–exported aura-client
// credentials file shape from the CLI-100 issue: comments, blank lines, and
// the three CLIENT_* keys.
const auraClientFileContent = `# Aura API client credentials
# Created at https://console.neo4j.io/account
CLIENT_ID=abc123def456
CLIENT_SECRET=supersecretvalue
CLIENT_NAME=MyClient
`

func TestAddCredentialFromFile(t *testing.T) {
	// Use forward-slash literal paths: shlex.Split treats backslashes as
	// escapes (POSIX-style), so a Windows-native path would be mangled by the
	// command parser. afero.MemMapFs normalises paths via filepath.Clean, so
	// `/tmp/x` is equivalent to `\tmp\x` on Windows for storage lookup.
	credsPath := "/tmp/aura-client.txt"
	missingPath := "/tmp/missing.txt"

	tests := []struct {
		name            string
		files           map[string]string
		command         string
		wantErr         string
		wantCredentials string
		wantDefaultCred string
		assertNoAdd     bool
	}{
		{
			name:            "happy path: --file alone populates all fields",
			files:           map[string]string{credsPath: auraClientFileContent},
			command:         "credential add --file " + credsPath + " --rw",
			wantCredentials: `[{"name":"MyClient","client-id":"abc123def456","client-secret":"supersecretvalue","access-token":"","token-expiry":0}]`,
			wantDefaultCred: "MyClient",
		},
		{
			name:            "--name overrides CLIENT_NAME",
			files:           map[string]string{credsPath: auraClientFileContent},
			command:         "credential add --file " + credsPath + " --name custom --rw",
			wantCredentials: `[{"name":"custom","client-id":"abc123def456","client-secret":"supersecretvalue","access-token":"","token-expiry":0}]`,
			wantDefaultCred: "custom",
		},
		{
			name:            "--client-secret overrides CLIENT_SECRET from file",
			files:           map[string]string{credsPath: auraClientFileContent},
			command:         "credential add --file " + credsPath + " --client-secret override --rw",
			wantCredentials: `[{"name":"MyClient","client-id":"abc123def456","client-secret":"override","access-token":"","token-expiry":0}]`,
			wantDefaultCred: "MyClient",
		},
		{
			name: "comments and blank lines parse without error",
			files: map[string]string{credsPath: `# leading comment

CLIENT_ID=abc123
# inline comment between keys

CLIENT_SECRET=ssss
CLIENT_NAME=Inst
`},
			command:         "credential add --file " + credsPath + " --rw",
			wantCredentials: `[{"name":"Inst","client-id":"abc123","client-secret":"ssss","access-token":"","token-expiry":0}]`,
			wantDefaultCred: "Inst",
		},
		{
			name:        "file missing CLIENT_ID errors naming client-id",
			files:       map[string]string{credsPath: "CLIENT_SECRET=ssss\nCLIENT_NAME=Inst\n"},
			command:     "credential add --file " + credsPath + " --rw",
			wantErr:     `--client-id is required (provide via --file as CLIENT_ID, or pass --client-id)`,
			assertNoAdd: true,
		},
		{
			name:        "empty file errors on first missing field (name)",
			files:       map[string]string{credsPath: ""},
			command:     "credential add --file " + credsPath + " --rw",
			wantErr:     `--name is required (provide via --file as CLIENT_NAME, or pass --name)`,
			assertNoAdd: true,
		},
		{
			name:        "CLIENT_ID= empty with no flag override is a usage error",
			files:       map[string]string{credsPath: "CLIENT_ID=\nCLIENT_SECRET=ssss\nCLIENT_NAME=Inst\n"},
			command:     "credential add --file " + credsPath + " --rw",
			wantErr:     `CLIENT_ID has an empty value`,
			assertNoAdd: true,
		},
		{
			name:            "CLIENT_ID= empty with --client-id override succeeds",
			files:           map[string]string{credsPath: "CLIENT_ID=\nCLIENT_SECRET=ssss\nCLIENT_NAME=Inst\n"},
			command:         "credential add --file " + credsPath + " --client-id real --rw",
			wantCredentials: `[{"name":"Inst","client-id":"real","client-secret":"ssss","access-token":"","token-expiry":0}]`,
			wantDefaultCred: "Inst",
		},
		{
			name:        "missing --file path returns a wrapped open error",
			files:       map[string]string{},
			command:     "credential add --file " + missingPath + " --rw",
			wantErr:     "--file \"" + missingPath + "\":",
			assertNoAdd: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			helper.SetCredentialsValue("aura.credentials", []map[string]string{})

			for path, content := range tc.files {
				helper.SeedFile(path, content)
			}

			helper.ExecuteCommand(tc.command)

			if tc.wantErr != "" {
				helper.AssertErrContainsStrings([]string{tc.wantErr})
				if tc.assertNoAdd {
					helper.AssertCredentialsValue("aura.credentials", `[]`)
				}
				return
			}

			helper.AssertErr("")
			helper.AssertCredentialsValue("aura.credentials", tc.wantCredentials)
			helper.AssertCredentialsValue("aura.default-credential", tc.wantDefaultCred)
		})
	}
}
