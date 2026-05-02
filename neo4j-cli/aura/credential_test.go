// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura_test

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// credentialTestHelper wires NewCredentialCmd with an in-memory filesystem,
// mirroring the pattern used by AuraTestHelper for the aura subcommand tree.
type credentialTestHelper struct {
	out         *bytes.Buffer
	err         *bytes.Buffer
	credentials string
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

func (h *credentialTestHelper) executeCommand(command string) {
	args, err := shlex.Split(command)
	assert.Nil(h.t, err)

	fs, err := testfs.GetTestFs("{}", h.credentials)
	assert.Nil(h.t, err)
	h.fs = fs

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)

	cmd := aura.NewCredentialCmd(cfg)
	aura.RegisterOutputFlag(cmd, cfg)
	cmd.SetArgs(args)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)

	cmd.Execute() //nolint:errcheck // cobra prints the error itself; test assertions check cmd output
}

func (h *credentialTestHelper) assertCredentialsValue(key string, expected string) {
	file, err := h.fs.Open(filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json"))
	assert.Nil(h.t, err)
	defer file.Close() //nolint:errcheck // in-memory FS close error is not actionable in a defer

	out, err := io.ReadAll(file)
	assert.Nil(h.t, err)

	actual := gjson.Get(string(out), key).String()

	formattedExpected, err := testutils.FormatJson(expected, "\t")
	if err != nil {
		formattedExpected = expected
	}
	formattedActual, err := testutils.FormatJson(actual, "\t")
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
			command:         "add aura-client --name test --client-id testclientid --client-secret testclientsecret",
			wantCredentials: `[{"name":"test","client-id":"testclientid","client-secret":"testclientsecret","access-token":"","token-expiry":0}]`,
			wantDefaultCred: "test",
		},
		{
			name:           "duplicate name returns an error",
			initialCreds:   []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}},
			initialDefault: "test",
			command:        "add aura-client --name test --client-id testclientid --client-secret testclientsecret",
			wantErr:        "Error: already have credential with name test",
		},
		{
			name:            "additional credential is stored without changing default",
			initialCreds:    []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}},
			initialDefault:  "test",
			command:         "add aura-client --name test-new --client-id testclientid2 --client-secret testclientsecret2",
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

			h.executeCommand(tc.command)

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
			name:         "lists all stored credentials as table (default)",
			command:      "list aura-client",
			initialCreds: []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}},
			wantContains: []string{"NAME", "CLIENT-ID", "test", "testclientid"},
		},
		{
			name:         "lists all stored credentials as json",
			command:      "list aura-client --output json",
			initialCreds: []map[string]string{{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"}},
			wantOut: `[
	{
		"client-id": "testclientid",
		"name": "test"
	}
]`,
		},
		{
			name:         "lists empty credentials as table (default)",
			command:      "list aura-client",
			initialCreds: []map[string]string{},
			wantContains: []string{"NAME", "CLIENT-ID"},
		},
		{
			name:         "lists empty credentials as json",
			command:      "list aura-client --output json",
			initialCreds: []map[string]string{},
			wantOut:      "[]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newCredentialTestHelper(t)
			h.setCredentialsValue("aura.credentials", tc.initialCreds)

			h.executeCommand(tc.command)

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
			command:         "remove aura-client test",
			wantCredentials: "[]",
		},
		{
			name:         "missing credential returns an error",
			initialCreds: []map[string]string{},
			command:      "remove aura-client nonexistent",
			wantErr:      "Error: could not find credential with name nonexistent to remove",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newCredentialTestHelper(t)
			h.setCredentialsValue("aura.credentials", tc.initialCreds)

			h.executeCommand(tc.command)

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			h.assertCredentialsValue("aura.credentials", tc.wantCredentials)
		})
	}
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
			command:         "use aura-client test",
			wantDefaultCred: "test",
		},
		{
			name:         "nonexistent credential returns an error",
			initialCreds: []map[string]string{},
			command:      "use aura-client nonexistent",
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

			h.executeCommand(tc.command)

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			h.assertCredentialsValue("aura.default-credential", tc.wantDefaultCred)
		})
	}
}
