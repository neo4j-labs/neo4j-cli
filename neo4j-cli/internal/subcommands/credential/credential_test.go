// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential_test

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/credential"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/neo4j/cli/test/utils/testjson"
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
	args, err := shlex.Split(command)
	assert.Nil(h.t, err)

	fs, err := testfs.GetTestFs("{}", h.credentials)
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
			confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

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

func TestCredentialRemoveAuraClientConfirmGate_NonTTYWithoutFlags_Exit2(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	h := newCredentialTestHelper(t)
	h.setCredentialsValue("aura.credentials", []map[string]string{
		{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"},
	})

	err := h.executeCommand("aura-client remove --rw test")

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
	h.assertCredentialsValue("aura.credentials", `[{"client-id":"testclientid","client-secret":"testclientsecret","name":"test"}]`)
}

func TestCredentialRemoveAuraClientConfirmGate_NonTTYWithBothFlags_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	h := newCredentialTestHelper(t)
	h.setCredentialsValue("aura.credentials", []map[string]string{
		{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"},
	})

	if err := h.executeCommand("aura-client remove --rw --yes --force test"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	h.assertCredentialsValue("aura.credentials", `[]`)
}

func TestCredentialRemoveAuraClientConfirmGate_TTYAnswerY_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	h := newCredentialTestHelper(t)
	h.setCredentialsValue("aura.credentials", []map[string]string{
		{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"},
	})
	h.setStdin("y\n")

	if err := h.executeCommand("aura-client remove --rw test"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	h.assertCredentialsValue("aura.credentials", `[]`)

	errOut, err := io.ReadAll(h.err)
	assert.Nil(t, err)
	assert.Contains(t, string(errOut), "Delete aura-client")
}

func TestCredentialRemoveAuraClientConfirmGate_TTYAnswerN_Cancels(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	h := newCredentialTestHelper(t)
	h.setCredentialsValue("aura.credentials", []map[string]string{
		{"name": "test", "client-id": "testclientid", "client-secret": "testclientsecret"},
	})
	h.setStdin("N\n")

	if err := h.executeCommand("aura-client remove --rw test"); !errors.Is(err, confirm.ErrCancelled) {
		t.Fatalf("expected confirm.ErrCancelled on cancel, got %v", err)
	}
	// Credential is unchanged on cancellation.
	h.assertCredentialsValue("aura.credentials", `[{"client-id":"testclientid","client-secret":"testclientsecret","name":"test"}]`)

	errOut, err := io.ReadAll(h.err)
	assert.Nil(t, err)
	assert.Contains(t, string(errOut), "cancelled.")
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
