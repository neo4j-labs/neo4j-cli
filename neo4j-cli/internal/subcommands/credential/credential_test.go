// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential_test

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/credential"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/neo4j/cli/test/utils/testjson"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	gokeyring "github.com/zalando/go-keyring"
)

// credentialTestHelper wires NewCredentialCmd with an in-memory filesystem,
// mirroring the pattern used by AuraTestHelper for the aura subcommand tree.
type credentialTestHelper struct {
	out         *bytes.Buffer
	err         *bytes.Buffer
	credentials string
	stdin       string
	fs          afero.Fs
	t           *testing.T
}

func newCredentialTestHelper(t *testing.T) credentialTestHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	return credentialTestHelper{
		out: bytes.NewBufferString(""),
		err: bytes.NewBufferString(""),
		credentials: `{
			"aura": {
				"credentials": [],
				"default-credential": ""
			}
		}`,
		t: t,
	}
}

func (h *credentialTestHelper) setCredentialsValue(key string, value interface{}) {
	creds, err := sjson.Set(h.credentials, key, value)
	assert.Nil(h.t, err)
	h.credentials = creds
}

func (h *credentialTestHelper) setStdin(in string) {
	h.stdin = in
}

func (h *credentialTestHelper) executeCommand(command string) error {
	return h.executeCommandWithConfig(command, "{}")
}

func (h *credentialTestHelper) executeCommandWithConfig(command string, configJSON string) error {
	args, err := shlex.Split(command)
	assert.Nil(h.t, err)

	fs, err := testfs.GetTestFs(configJSON, h.credentials)
	assert.Nil(h.t, err)
	h.fs = fs

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)

	cmd := credential.NewCredentialCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	flags.RegisterRwFlag(cmd)
	cmd.PersistentPreRunE = flags.ComposeRootPersistentPreRunE(cfg)
	cmd.SetArgs(args)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)
	cmd.SetIn(strings.NewReader(h.stdin))

	return cmd.Execute()
}

func (h *credentialTestHelper) assertCredentialsValue(key string, expected string) {
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

func (h *credentialTestHelper) assertOut(expected string) {
	out, err := io.ReadAll(h.out)
	assert.Nil(h.t, err)
	assert.Equal(h.t, strings.TrimSpace(expected), strings.TrimSpace(string(out)))
}

func (h *credentialTestHelper) assertErr(expected string) {
	out, err := io.ReadAll(h.err)
	assert.Nil(h.t, err)
	assert.Equal(h.t, strings.TrimSpace(expected), strings.TrimSpace(string(out)))
}

// --- add aura-client tests ---

func TestCredentialAddAuraClientRequiresRw(t *testing.T) {
	h := newCredentialTestHelper(t)

	h.executeCommand("aura-client add --name test --client-id testclientid --client-secret testclientsecret") //nolint:errcheck // error checked via assertErr

	h.assertErr("Error: this command writes; pass --rw to allow it")
}

func TestCredentialAddAuraClient(t *testing.T) {
	tests := []struct {
		name            string
		initialCreds    []map[string]string
		initialDefault  string
		command         string
		wantErr         string
		wantCredentials string
		wantDefaultCred string
	}{
		{
			name:            "first credential is stored and set as default",
			initialCreds:    []map[string]string{},
			initialDefault:  "",
			command:         "aura-client add --rw --name test --client-id testclientid --client-secret testclientsecret",
			wantCredentials: `[{"name":"test","client-id":"testclientid","client-secret":"testclientsecret","access-token":"","token-expiry":0}]`,
			wantDefaultCred: "test",
		},
		{
			name:           "duplicate name returns an error",
			initialCreds:   []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}},
			initialDefault: "test",
			command:        "aura-client add --rw --name test --client-id testclientid --client-secret testclientsecret",
			wantErr:        "Error: already have credential with name test",
		},
		{
			name:            "additional credential is stored without changing default",
			initialCreds:    []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}},
			initialDefault:  "test",
			command:         "aura-client add --rw --name test-new --client-id testclientid2 --client-secret testclientsecret2",
			wantCredentials: `[{"name":"test","client-id":"testclientid","client-secret":"testclientsecret","access-token":"","token-expiry":0},{"name":"test-new","client-id":"testclientid2","client-secret":"testclientsecret2","access-token":"","token-expiry":0}]`,
			wantDefaultCred: "test",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newCredentialTestHelper(t)
			h.setCredentialsValue("aura.credentials", tc.initialCreds)
			if tc.initialDefault != "" {
				h.setCredentialsValue("aura.default-credential", tc.initialDefault)
			}

			h.executeCommand(tc.command) //nolint:errcheck // error checked via assertErr

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			h.assertCredentialsValue("aura.credentials", tc.wantCredentials)
			h.assertCredentialsValue("aura.default-credential", tc.wantDefaultCred)
		})
	}
}

// --- list aura-client tests ---

func TestCredentialListAuraClient(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		initialCreds []map[string]string
		wantOut      string
		wantContains []string
	}{
		{
			name:         "lists all stored credentials as table (explicit --format table)",
			command:      "aura-client list --format table",
			initialCreds: []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}},
			wantContains: []string{"NAME", "TYPE", "IDENTIFIER", "test", "aura-client", "testclientid"},
		},
		{
			name:         "lists all stored credentials as json (explicit --format json)",
			command:      "aura-client list --format json",
			initialCreds: []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}},
			wantOut: `[
	{
		"default": false,
		"identifier": "testclientid",
		"name": "test",
		"type": "aura-client"
	}
]`,
		},
		{
			name:         "lists all stored credentials as json (default auto-detects non-TTY)",
			command:      "aura-client list",
			initialCreds: []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}},
			wantOut: `[
	{
		"default": false,
		"identifier": "testclientid",
		"name": "test",
		"type": "aura-client"
	}
]`,
		},
		{
			name:         "lists empty credentials as table (explicit --format table)",
			command:      "aura-client list --format table",
			initialCreds: []map[string]string{},
			wantContains: []string{"NAME", "TYPE", "IDENTIFIER"},
		},
		{
			name:         "lists empty credentials as json (explicit --format json)",
			command:      "aura-client list --format json",
			initialCreds: []map[string]string{},
			wantOut:      "[]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newCredentialTestHelper(t)
			h.setCredentialsValue("aura.credentials", tc.initialCreds)

			h.executeCommand(tc.command) //nolint:errcheck // error checked via assertErr

			h.assertErr("")
			if tc.wantOut != "" {
				h.assertOut(tc.wantOut)
			}
			if len(tc.wantContains) > 0 {
				out, err := io.ReadAll(h.out)
				assert.Nil(t, err)
				outStr := string(out)
				for _, want := range tc.wantContains {
					assert.Contains(t, outStr, want)
				}
			}
		})
	}
}

// --- remove aura-client tests ---

func TestCredentialRemoveAuraClient(t *testing.T) {
	tests := []struct {
		name            string
		initialCreds    []map[string]string
		command         string
		wantErr         string
		wantCredentials string
	}{
		{
			name:            "named credential is removed",
			initialCreds:    []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}},
			command:         "aura-client remove --rw --yes --force test",
			wantCredentials: "[]",
		},
		{
			name:         "missing credential returns an error",
			initialCreds: []map[string]string{},
			command:      "aura-client remove --rw --yes --force nonexistent",
			wantErr:      "Error: could not find credential with name nonexistent to remove",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

			h := newCredentialTestHelper(t)
			h.setCredentialsValue("aura.credentials", tc.initialCreds)

			h.executeCommand(tc.command) //nolint:errcheck // error checked via assertErr

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			h.assertCredentialsValue("aura.credentials", tc.wantCredentials)
		})
	}
}

// --- remove aura-client keyring tests ---

// TestCredentialRemoveAuraClient_KeyringMode verifies that in keyring mode,
// removing a credential also deletes its keyring entries. ErrNotFound on
// delete is silently ignored and does not block the credential removal.
func TestCredentialRemoveAuraClient_KeyringMode(t *testing.T) {
	const keyringConfig = `{"credential-storage":"keyring"}`

	t.Run("removes credential and deletes keyring entries", func(t *testing.T) {
		gokeyring.MockInit()
		h := newCredentialTestHelper(t)
		h.setCredentialsValue("aura.credentials", []map[string]interface{}{
			{"name": "prod", "client-id": "id1", "client-secret": "s3cr3t", "access-token": "tok", "token-expiry": 0},
		})
		// Seed keyring entries that should be deleted after remove
		assert.Nil(t, gokeyring.Set("neo4j-cli", "aura/prod/client-secret", "s3cr3t"))
		assert.Nil(t, gokeyring.Set("neo4j-cli", "aura/prod/access-token", "tok"))

		h.executeCommandWithConfig("aura-client remove --rw --yes --force prod", keyringConfig) //nolint:errcheck

		h.assertErr("")
		h.assertCredentialsValue("aura.credentials", "[]")

		// Keyring entries must be absent after remove
		_, err := gokeyring.Get("neo4j-cli", "aura/prod/client-secret")
		assert.ErrorIs(t, err, gokeyring.ErrNotFound, "keyring client-secret must be deleted on remove")
		_, err = gokeyring.Get("neo4j-cli", "aura/prod/access-token")
		assert.ErrorIs(t, err, gokeyring.ErrNotFound, "keyring access-token must be deleted on remove")
	})

	t.Run("remove succeeds even when keyring entries are already absent", func(t *testing.T) {
		gokeyring.MockInit()
		h := newCredentialTestHelper(t)
		h.setCredentialsValue("aura.credentials", []map[string]interface{}{
			{"name": "prod", "client-id": "id1", "client-secret": "s3cr3t", "access-token": "", "token-expiry": 0},
		})
		// No keyring entries seeded — ErrNotFound on delete must not block removal

		h.executeCommandWithConfig("aura-client remove --rw --yes --force prod", keyringConfig) //nolint:errcheck

		h.assertErr("")
		h.assertCredentialsValue("aura.credentials", "[]")
	})

	t.Run("remove in insecure mode does not touch keyring", func(t *testing.T) {
		gokeyring.MockInit()
		h := newCredentialTestHelper(t)
		h.setCredentialsValue("aura.credentials", []map[string]interface{}{
			{"name": "prod", "client-id": "id1", "client-secret": "s3cr3t", "access-token": "", "token-expiry": 0},
		})
		// Seed a keyring entry that must NOT be deleted in insecure mode
		assert.Nil(t, gokeyring.Set("neo4j-cli", "aura/prod/client-secret", "s3cr3t"))

		h.executeCommand("aura-client remove --rw --yes --force prod") //nolint:errcheck

		h.assertErr("")
		// Keyring entry must still be present (insecure mode does not clean up)
		val, err := gokeyring.Get("neo4j-cli", "aura/prod/client-secret")
		assert.NoError(t, err)
		assert.Equal(t, "s3cr3t", val)
	})
}

func TestCredentialRemoveAuraClient_ConfirmGate(t *testing.T) {
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "credential aura-client remove",
		NoFlagsArgs:   "aura-client remove --rw test",
		BothFlagsArgs: "aura-client remove --rw --yes --force test",
		ResourceLabel: "aura-client",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			h := newCredentialTestHelper(t)
			h.setCredentialsValue("aura.credentials", []map[string]string{
				{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"},
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
			invoked := gjson.Get(string(contents), "aura.credentials").String() == "[]"
			return confirmtest.GateRunResult{Err: err, Stderr: h.err.String(), Invoked: invoked}
		},
	})
}

// --- use aura-client tests ---

func TestCredentialUseAuraClient(t *testing.T) {
	tests := []struct {
		name            string
		initialCreds    []map[string]string
		initialDefault  string
		command         string
		wantErr         string
		wantDefaultCred string
	}{
		{
			name:            "named credential becomes the default",
			initialCreds:    []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}},
			initialDefault:  "",
			command:         "aura-client use --rw test",
			wantDefaultCred: "test",
		},
		{
			name:         "nonexistent credential returns an error",
			initialCreds: []map[string]string{},
			command:      "aura-client use --rw nonexistent",
			wantErr:      "Error: could not find credential with name nonexistent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newCredentialTestHelper(t)
			h.setCredentialsValue("aura.credentials", tc.initialCreds)
			if tc.initialDefault != "" {
				h.setCredentialsValue("aura.default-credential", tc.initialDefault)
			}

			h.executeCommand(tc.command) //nolint:errcheck // error checked via assertErr

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			h.assertCredentialsValue("aura.default-credential", tc.wantDefaultCred)
		})
	}
}
