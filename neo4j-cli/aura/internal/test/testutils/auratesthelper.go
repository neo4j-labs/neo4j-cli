// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package testutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/aura"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type AuraTestHelper struct {
	mux          *http.ServeMux
	Server       *httptest.Server
	out          *bytes.Buffer
	err          *bytes.Buffer
	stdin        string
	cfg          string
	credentials  string
	fs           afero.Fs
	pendingFiles map[string]string
	t            *testing.T
}

func (helper *AuraTestHelper) Close() {
	helper.Server.Close()
}

func (helper *AuraTestHelper) ExecuteCommand(command string) {
	_ = helper.ExecuteCommandE(command)
}

// ExecuteCommandE runs the command like ExecuteCommand but returns the error
// returned by cobra's Execute(). Use this when a test needs to inspect the
// underlying *clierr.CLIError via errors.As (e.g. asserting Suggestion).
// Stdout / stderr are still captured for the existing AssertOut/AssertErr
// helpers.
func (helper *AuraTestHelper) ExecuteCommandE(command string) error {
	args, err := shlex.Split(command)
	assert.Nil(helper.t, err)

	fs, err := testfs.GetTestFs(helper.cfg, helper.credentials)
	assert.Nil(helper.t, err)

	helper.fs = fs

	for path, content := range helper.pendingFiles {
		assert.Nil(helper.t, afero.WriteFile(helper.fs, path, []byte(content), 0o644))
	}

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)

	cfg.Aura.SetPollingConfig(5, 0)

	cmd := aura.NewStandaloneCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.PersistentPreRunE = flags.ComposeRootPersistentPreRunE(cfg)

	cmd.SetArgs(args)

	cmd.SetOut(helper.out)
	cmd.SetErr(helper.err)
	cmd.SetIn(strings.NewReader(helper.stdin))

	return cmd.Execute()
}

// SetStdin buffers stdin for the next ExecuteCommand call. Used by confirm
// gating tests that drive the y/N prompt.
func (helper *AuraTestHelper) SetStdin(in string) {
	helper.stdin = in
}

func (helper *AuraTestHelper) SetConfig(cfg string) {
	helper.cfg = cfg
}

func (helper *AuraTestHelper) OverwriteConfig(cfg string) {
	helper.cfg = cfg
}

func (helper *AuraTestHelper) SetConfigValue(key string, value interface{}) {
	cfg, err := sjson.Set(helper.cfg, key, value)
	assert.Nil(helper.t, err)
	helper.cfg = cfg
}

func (helper *AuraTestHelper) SetCredentialsValue(key string, value interface{}) {
	credentials, err := sjson.Set(helper.credentials, key, value)
	assert.Nil(helper.t, err)
	helper.credentials = credentials
}

// SeedFile stashes a pending write to the in-memory test fs. The write is
// flushed inside ExecuteCommand after the fs is constructed, so callers can
// stage file content (e.g. an Aura-exported credentials file consumed via
// --env) before invoking the command under test.
func (helper *AuraTestHelper) SeedFile(path, content string) {
	if helper.pendingFiles == nil {
		helper.pendingFiles = map[string]string{}
	}
	helper.pendingFiles[path] = content
}

func (helper *AuraTestHelper) SetDefaultProjectInConfig(organizationId, projectId string) {
	helper.SetConfigValue("aura.default-workspace", organizationId+"/"+projectId)
}

// Assets no errors were returned
func (helper *AuraTestHelper) AsssertOk() {
	helper.AssertErr("")
}

func (helper *AuraTestHelper) AssertErr(expected string) {
	out, err := io.ReadAll(helper.err)
	assert.Nil(helper.t, err)

	assert.Equal(helper.t, strings.TrimSpace(expected), strings.TrimSpace(string(out)))
}

func (helper *AuraTestHelper) AssertOut(expected string) {
	out, err := io.ReadAll(helper.out)
	assert.Nil(helper.t, err)

	assert.Equal(helper.t, strings.TrimSpace(expected), strings.TrimSpace(string(out)))
}

func (helper *AuraTestHelper) PrintOut() string {
	out, err := io.ReadAll(helper.out)
	assert.Nil(helper.t, err)

	return string(out)
}
func (helper *AuraTestHelper) PrintErr() string {
	out, err := io.ReadAll(helper.err)
	assert.Nil(helper.t, err)

	return string(out)
}

func (helper *AuraTestHelper) AssertUsageNotShown() {
	assert.NotContains(helper.t, helper.PrintErr(), "Usage:")
}

func (helper *AuraTestHelper) AssertOutJson(expected string) {
	out, err := io.ReadAll(helper.out)
	assert.Nil(helper.t, err)

	formattedExpected, err := FormatJson(expected, "\t")
	if err != nil {
		panic(clierr.NewFatalError("invalid json in AssertOutJSON: %d", err))
	}

	assert.Nil(helper.t, err)

	assert.Equal(helper.t, formattedExpected, string(out))
}

func (helper *AuraTestHelper) AssertOutContainsStrings(expected []string) {
	out, err := io.ReadAll(helper.out)
	assert.Nil(helper.t, err)

	for _, exp := range expected {
		assert.Contains(helper.t, string(out), exp)
	}
}

func (helper *AuraTestHelper) AssertErrContainsStrings(expected []string) {
	out, err := io.ReadAll(helper.err)
	assert.Nil(helper.t, err)

	for _, exp := range expected {
		assert.Contains(helper.t, string(out), exp)
	}
}

// AssertOutIsValidJSON parses stdout via json.Unmarshal and fails the test
// if stdout is empty or not valid JSON. This is the regression-pinning
// assertion for CLI-82 — pre-fix, stdout had narration mixed with the JSON
// body and would fail to unmarshal.
func (helper *AuraTestHelper) AssertOutIsValidJSON() {
	out, err := io.ReadAll(helper.out)
	assert.Nil(helper.t, err)

	var v any
	assert.NoErrorf(helper.t, json.Unmarshal(out, &v),
		"stdout is not valid JSON; got: %q", string(out))
}

func (helper *AuraTestHelper) AssertConfig(expected string) {
	file, err := helper.fs.Open(filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "config.json"))
	assert.Nil(helper.t, err)
	defer file.Close() //nolint:errcheck // in-memory FS close error is not actionable in a defer

	out, err := io.ReadAll(file)
	assert.Nil(helper.t, err)

	formatted, err := FormatJson(expected, "  ")
	assert.Nil(helper.t, err)

	assert.Equal(helper.t, formatted, string(out))
}

func (helper *AuraTestHelper) AssertConfigValue(key string, expected string) {
	file, err := helper.fs.Open(filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "config.json"))
	assert.Nil(helper.t, err)
	defer file.Close() //nolint:errcheck // in-memory FS close error is not actionable in a defer

	out, err := io.ReadAll(file)
	assert.Nil(helper.t, err)

	strOut := string(out)
	actual := gjson.Get(strOut, key)

	formattedExpected, err := FormatJson(expected, "\t")
	if err != nil {
		formattedExpected = expected
	}

	formattedActual, err := FormatJson(actual.String(), "\t")
	if err != nil {
		formattedActual = actual.String()
	}

	assert.Equal(helper.t, formattedExpected, formattedActual)
}

// CredentialsValue returns the raw JSON encoding of the credential field at
// key (gjson-style path). Useful for confirmtest sinks that need to detect
// whether the credentials store was mutated without asserting equality.
func (helper *AuraTestHelper) CredentialsValue(key string) string {
	file, err := helper.fs.Open(filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json"))
	assert.Nil(helper.t, err)
	defer file.Close() //nolint:errcheck // in-memory FS close error is not actionable in a defer

	out, err := io.ReadAll(file)
	assert.Nil(helper.t, err)

	return gjson.Get(string(out), key).String()
}

func (helper *AuraTestHelper) AssertCredentialsValue(key string, expected string) { // TODO: merge with assertConfig
	file, err := helper.fs.Open(filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json"))
	assert.Nil(helper.t, err)
	defer file.Close() //nolint:errcheck // in-memory FS close error is not actionable in a defer

	out, err := io.ReadAll(file)
	assert.Nil(helper.t, err)

	actual := gjson.Get(string(out), key)

	formattedExpected, err := FormatJson(expected, "\t")
	if err != nil {
		formattedExpected = expected
	}

	formattedActual, err := FormatJson(actual.String(), "\t")
	if err != nil {
		formattedActual = actual.String()
	}

	assert.Equal(helper.t, formattedExpected, formattedActual)
}

func (helper *AuraTestHelper) NewRequestHandlerMock(path string, status int, body string) *requestHandlerMock {
	mock := requestHandlerMock{Calls: []call{}, t: helper.t, Responses: []response{
		{status: status, body: body},
	}}

	helper.mux.HandleFunc(path, func(res http.ResponseWriter, req *http.Request) {
		requestBody, err := io.ReadAll(req.Body)
		assert.Nil(helper.t, err)

		var unmarshalledBody map[string]interface{}
		if len(requestBody) > 0 {
			unmarshalledBody, err = UmarshalJson(requestBody)
			assert.Nil(helper.t, err)
		}

		requestCount := len(mock.Calls)
		mock.Calls = append(mock.Calls, call{Method: req.Method, Path: req.URL.Path, Body: unmarshalledBody, QueryParams: req.URL.Query()})

		if requestCount >= len(mock.Responses) {
			res.WriteHeader(404)
		} else {
			response := mock.Responses[requestCount]

			res.WriteHeader(response.status)
			res.Write([]byte(response.body)) //nolint:errcheck // test HTTP response write; errors would manifest as test assertion failures
		}
	})

	return &mock
}

func NewAuraTestHelper(t *testing.T) AuraTestHelper {
	cobra.EnableTraverseRunHooks = true

	helper := AuraTestHelper{}

	helper.t = t

	helper.out = bytes.NewBufferString("")
	helper.err = bytes.NewBufferString("")

	helper.mux = http.NewServeMux()
	helper.mux.HandleFunc("/oauth/token", func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(200)
		res.Write([]byte(`{"access_token":"<token>","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck // test HTTP response write; errors would manifest as test assertion failures
	})

	server := httptest.NewServer(helper.mux)

	helper.cfg = fmt.Sprintf(`{
				"format": "json",
				"aura": {
					"auth-url": "%s/oauth/token",
					"base-url": "%s/v1"
					}
				}`, server.URL, server.URL)
	helper.credentials = `{
				"aura": {
					"credentials": [{
						"name": "test-cred",
						"access-token": "dsa",
						"token-expiry": 123
					}],
					"default-credential": "test-cred"
					}
				}`

	helper.Server = server

	return helper
}
