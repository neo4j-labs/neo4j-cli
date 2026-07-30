// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// The end-to-end command tests live in the EXTERNAL test package: they drive the
// whole aura tree through testutils, which imports neo4j-cli/aura, which imports
// the package under test. An external test package compiles separately, so that
// is not an import cycle.
package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/neo4j/cli/neo4j-cli/aura"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	instancesBody = `{"data":[{"id":"i1","name":"one"},{"id":"i2","name":"two"}]}`
	testOrgID     = "org-abc-123"
	testProjectID = "proj-def-456"
)

// newAPIHelper is the plain aura test helper: cfg.Aura.BaseUrl() reduces any
// configured base-url to scheme://host, so the version segment the endpoint
// carries is the only one in the request path — the helper's default
// "<server>/v1" base-url does not double it.
func newAPIHelper(t *testing.T) testutils.AuraTestHelper {
	t.Helper()

	return testutils.NewAuraTestHelper(t)
}

func TestAPIGet(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody)

	helper.ExecuteCommand("api v1/instances")

	mock.AssertCalledTimes(1)
	mock.AssertCalledWithMethod(http.MethodGet)
	helper.AsssertOk()
	// Byte-for-byte, not a re-marshalled equivalent: --format json promises the
	// upstream document survives untouched so `| jq` sees what the server sent.
	assert.Equal(t, instancesBody+"\n", helper.PrintOut())
}

// TestAPIGet_LeadingSlashAndUnreleasedVersion pins the two halves of the
// endpoint contract: the version is whatever the caller wrote (no enum), and a
// leading slash is accepted.
func TestAPIGet_LeadingSlashAndUnreleasedVersion(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock("/v9alpha3/widgets", http.StatusOK, `{"data":[]}`)

	helper.ExecuteCommand("api /v9alpha3/widgets")

	mock.AssertCalledTimes(1)
	helper.AsssertOk()
}

func TestAPIGet_BaseUrlAndAuthUrlFlags(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	// Both configured values are unreachable, so the request can only succeed if
	// the flags are bound onto cfg.Aura.
	helper.SetConfigValue("aura.base-url", "https://base-url-flag-was-ignored.invalid")
	helper.SetConfigValue("aura.auth-url", "https://auth-url-flag-was-ignored.invalid/oauth/token")

	mock := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody)

	helper.ExecuteCommand(fmt.Sprintf("api v1/instances --base-url %s --auth-url %s/oauth/token", helper.Server.URL, helper.Server.URL))

	mock.AssertCalledTimes(1)
	helper.AsssertOk()
}

func TestAPIGet_CredentialFlag(t *testing.T) {
	t.Run("known credential is used", func(t *testing.T) {
		helper := newAPIHelper(t)
		defer helper.Close()

		mock := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody)

		helper.ExecuteCommand("api v1/instances --credential test-cred")

		mock.AssertCalledTimes(1)
		helper.AsssertOk()
	})

	t.Run("unknown credential is a usage error", func(t *testing.T) {
		helper := newAPIHelper(t)
		defer helper.Close()

		mock := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody)

		err := helper.ExecuteCommandE("api v1/instances --credential nope")

		mock.AssertCalledTimes(0)
		assertExitCode(t, err, 2)
	})
}

func TestAPIGet_SubstitutesOrgAndProjectPlaceholders(t *testing.T) {
	testCases := []struct {
		name     string
		endpoint string
		args     string
		path     string
	}{
		{
			name:     "snake_case tokens from flags",
			endpoint: "v2beta1/organizations/{org_id}/projects/{project_id}/instances",
			args:     fmt.Sprintf("--organization-id %s --project-id %s", testOrgID, testProjectID),
			path:     fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/instances", testOrgID, testProjectID),
		},
		{
			name:     "short aliases from the default workspace",
			endpoint: "v2beta1/organizations/{org}/projects/{project}",
			path:     fmt.Sprintf("/v2beta1/organizations/%s/projects/%s", testOrgID, testProjectID),
		},
		{
			name:     "org-only path needs no project",
			endpoint: "v2beta1/organizations/{org_id}/projects",
			args:     fmt.Sprintf("--organization-id %s", testOrgID),
			path:     fmt.Sprintf("/v2beta1/organizations/%s/projects", testOrgID),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper := newAPIHelper(t)
			defer helper.Close()

			if tc.args == "" {
				helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
			}
			mock := helper.NewRequestHandlerMock(tc.path, http.StatusOK, `{"data":[]}`)

			helper.ExecuteCommand(fmt.Sprintf("api '%s' %s", tc.endpoint, tc.args))

			mock.AssertCalledTimes(1)
			helper.AsssertOk()
		})
	}
}

// TestAPIGet_PlaceholderFreePathIgnoresTheWorkspace pins that resolution is
// driven by the placeholders alone: a configured workspace must not leak into an
// unscoped path (TestAPIGet already covers the no-workspace half).
func TestAPIGet_PlaceholderFreePathIgnoresTheWorkspace(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	mock := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody)

	helper.ExecuteCommand("api v1/instances")

	mock.AssertCalledTimes(1)
	assert.Equal(t, "/v1/instances", mock.Calls[0].Path)
	assert.Empty(t, mock.Calls[0].QueryParams)
	helper.AsssertOk()
}

func TestAPIGet_MergesInlineQueryWithFields(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody)

	// --method GET is explicit because a field would otherwise infer POST.
	helper.ExecuteCommand("api 'v1/instances?include_deleted=true' --method GET --field page_limit=10")

	mock.AssertCalledTimes(1)
	mock.AssertCalledWithMethod(http.MethodGet)
	mock.AssertCalledWithQueryParam("include_deleted", "true")
	mock.AssertCalledWithQueryParam("page_limit", "10")
	helper.AsssertOk()
}

func TestAPIWrite_RequiresRw(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			helper := newAPIHelper(t)
			defer helper.Close()

			mock := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody)

			err := helper.ExecuteCommandE("api v1/instances --method " + method)

			mock.AssertCalledTimes(0)
			assertExitCode(t, err, 2)
			assert.Contains(t, err.Error(), "this command writes; pass --rw to allow it")
		})
	}
}

// TestAPIWrite_GateRunsBeforeThePayloadIsRead pins the gate ordering: the write
// gate must be reported even when the payload flags would also fail, so a
// refused invocation neither touches the filesystem nor drains stdin.
func TestAPIWrite_GateRunsBeforeThePayloadIsRead(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody)

	err := helper.ExecuteCommandE("api v1/instances --method POST --input does-not-exist.json")

	mock.AssertCalledTimes(0)
	assertExitCode(t, err, 2)
	assert.Contains(t, err.Error(), "this command writes; pass --rw to allow it")
}

// TestAPIDelete_PromptStillReadableWithAStdinField guards the same ordering from
// the other side: reading a `@-` field before the prompt would leave the prompt
// at EOF, silently cancelling every such delete.
func TestAPIDelete_PromptStillReadableWithAStdinField(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return true }))
	mock := helper.NewRequestHandlerMock("/v1/instances/i1", http.StatusAccepted, "")
	helper.SetStdin("y\n")

	err := helper.ExecuteCommandE("api v1/instances/i1 --method DELETE --field reason=@- --rw")

	require.NoError(t, err)
	mock.AssertCalledWithMethod(http.MethodDelete)
}

func TestAPIPost_InfersMethodAndSendsFieldsAsBody(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock("/v2beta1/instances/00000000/databases", http.StatusCreated, `{"data":{"name":"sales"}}`)

	helper.ExecuteCommand("api v2beta1/instances/00000000/databases --field name=sales --field wait=true --field size=3 --rw")

	mock.AssertCalledTimes(1)
	mock.AssertCalledWithMethod(http.MethodPost)
	mock.AssertCalledWithBody(`{"name":"sales","wait":true,"size":3}`)
	helper.AsssertOk()
	assert.Equal(t, `{"data":{"name":"sales"}}`+"\n", helper.PrintOut())
}

func TestAPIPost_SendsInputFileVerbatim(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	helper.SeedFile("database.json", `{"name":"sales","memory":"1GB"}`)
	mock := helper.NewRequestHandlerMock("/v2beta1/instances/00000000/databases", http.StatusCreated, `{"data":{"name":"sales"}}`)

	helper.ExecuteCommand("api v2beta1/instances/00000000/databases --input database.json --rw")

	mock.AssertCalledTimes(1)
	mock.AssertCalledWithMethod(http.MethodPost)
	mock.AssertCalledWithBody(`{"name":"sales","memory":"1GB"}`)
	helper.AsssertOk()
}

// TestAPIDelete_ConfirmGate replays the four canonical destructive-leaf
// scenarios. --rw is passed throughout so the write gate (which runs first)
// never masks the confirm gate under test.
func TestAPIDelete_ConfirmGate(t *testing.T) {
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "aura api --method DELETE",
		NoFlagsArgs:   "api v1/instances/i1 --method DELETE --rw",
		BothFlagsArgs: "api v1/instances/i1 --method DELETE --rw --yes --force",
		ResourceLabel: "endpoint",
		Run: func(t *testing.T, args string, stdin string) confirmtest.GateRunResult {
			helper := newAPIHelper(t)
			defer helper.Close()

			mock := helper.NewRequestHandlerMock("/v1/instances/i1", http.StatusAccepted, "")
			helper.SetStdin(stdin)

			err := helper.ExecuteCommandE(args)

			return confirmtest.GateRunResult{
				Err:     err,
				Stderr:  helper.PrintErr(),
				Invoked: mock.CalledWithMethod(http.MethodDelete),
			}
		},
	})
}

// TestAPIDelete_PromptNamesEndpoint pins the noun confirm.RequireTyped is given:
// confirm.Require would derive it from the parent, which is the aura root.
func TestAPIDelete_PromptNamesEndpoint(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock("/v1/instances/i1", http.StatusAccepted, "")

	err := helper.ExecuteCommandE("api v1/instances/i1 --method DELETE --rw")

	mock.AssertCalledTimes(0)
	assertExitCode(t, err, 2)
	assert.Contains(t, err.Error(), `refusing to delete endpoint "v1/instances/i1"`)
}

func TestAPIInclude(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody).
		WithResponseHeader("X-Trace-Id", "abc123")

	helper.ExecuteCommand("api v1/instances --include")

	helper.AsssertOk()
	out := helper.PrintOut()
	assert.True(t, strings.HasPrefix(out, "HTTP/1.1 200 OK\n"), "status line must come first; got %q", out)
	assert.Contains(t, out, "X-Trace-Id: abc123")
	assert.True(t, strings.Index(out, "Content-Type:") < strings.Index(out, "X-Trace-Id:"), "headers must be sorted by name; got %q", out)
	assert.True(t, strings.Index(out, "X-Trace-Id:") < strings.Index(out, instancesBody), "the body must come last; got %q", out)
}

func TestAPISilent(t *testing.T) {
	testCases := []struct {
		name       string
		args       string
		wantPrefix string
	}{
		{name: "body suppressed", args: "api v1/instances --silent"},
		{name: "headers still printed", args: "api v1/instances --silent --include", wantPrefix: "HTTP/1.1 200 OK\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper := newAPIHelper(t)
			defer helper.Close()

			mock := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody)

			helper.ExecuteCommand(tc.args)

			mock.AssertCalledTimes(1)
			helper.AsssertOk()
			// PrintOut drains the buffer, so read it once.
			out := helper.PrintOut()
			if tc.wantPrefix == "" {
				assert.Empty(t, out)
				return
			}
			assert.NotContains(t, out, instancesBody)
			assert.True(t, strings.HasPrefix(out, tc.wantPrefix), "expected stdout to start with %q; got %q", tc.wantPrefix, out)
		})
	}
}

func TestAPIFormatTable(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody)

	helper.ExecuteCommand("api v1/instances --format table")

	mock.AssertCalledTimes(1)
	helper.AsssertOk()
	helper.AssertOutContainsStrings([]string{"ID", "NAME", "i1", "two"})
}

// TestAPIError_UpstreamBodyGoesToTheError pins REQ-F-028: the upstream body must
// reach the error, never stdout, so clierr.Render's envelope stays the only JSON
// document on stdout.
func TestAPIError_UpstreamBodyGoesToTheError(t *testing.T) {
	testCases := []struct {
		name     string
		args     string
		status   int
		body     string
		wantCode int
	}{
		{name: "not found", status: http.StatusNotFound, body: `{"errors":[{"message":"instance not found"}]}`, wantCode: 3},
		{name: "bad request", status: http.StatusBadRequest, body: `{"errors":[{"message":"bad field"}]}`, wantCode: 6},
		{name: "empty body", status: http.StatusBadRequest, body: "", wantCode: 6},
		{name: "non-json body", status: http.StatusInternalServerError, body: "<html>boom</html>", wantCode: 8},
		// --include is the risky combination: it is the one flag that writes to
		// stdout ahead of the body, and it must stay silent on a failing status so
		// the error envelope remains the only document there.
		{name: "include is skipped", args: " --include", status: http.StatusNotFound, body: `{"errors":[{"message":"nope"}]}`, wantCode: 3},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper := newAPIHelper(t)
			defer helper.Close()

			helper.NewRequestHandlerMock("/v1/instances", tc.status, tc.body)

			err := helper.ExecuteCommandE("api v1/instances" + tc.args)

			ce := cliError(t, err)
			assert.Equal(t, tc.wantCode, ce.Code, "unexpected exit code for error %q", err)
			assert.Empty(t, helper.PrintOut(), "the upstream body must not be echoed to stdout")
			if tc.body != "" {
				assert.Contains(t, ce.Message, tc.body)
			}

			// The rendered envelope must be the single JSON document on stdout.
			envelope, marshalErr := json.Marshal(ce.BuildEnvelope())
			require.NoError(t, marshalErr)
			var decoded any
			require.NoError(t, json.Unmarshal(envelope, &decoded))
		})
	}
}

func TestAPIUsageErrors(t *testing.T) {
	testCases := []struct {
		name string
		args string
	}{
		{name: "absolute url", args: "api https://evil.example.com/v1/instances"},
		{name: "protocol relative url", args: "api //evil.example.com/v1/instances"},
		{name: "parent traversal", args: "api v1/../../instances"},
		{name: "unsupported placeholder", args: "api 'v1/{instance_id}'"},
		{name: "unsupported method", args: "api v1/instances --method BREW --rw"},
		{name: "input with field", args: "api v1/instances --input database.json --field name=x --rw"},
		{name: "malformed header", args: "api v1/instances --header NoColonHere"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper := newAPIHelper(t)
			defer helper.Close()

			mock := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody)

			err := helper.ExecuteCommandE(tc.args)

			mock.AssertCalledTimes(0)
			assertExitCode(t, err, 2)
		})
	}
}

// TestAPIMissingEndpoint pins the positional-argument contract.
func TestAPIMissingEndpoint(t *testing.T) {
	helper := newAPIHelper(t)
	defer helper.Close()

	mock := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, instancesBody)

	err := helper.ExecuteCommandE("api")

	mock.AssertCalledTimes(0)
	require.Error(t, err)
}

// TestAPICommand_IsRegisteredOnAuraRoot also guards the flag surface the command
// must replicate: the aura root owns only --debug, so an omitted bind would
// leave --base-url silently inert.
func TestAPICommand_IsRegisteredOnAuraRoot(t *testing.T) {
	cmd := findAPICmd(t)

	for _, name := range []string{"auth-url", "base-url", "credential", "organization-id", "project-id", "yes", "force", "include", "silent", "method", "field", "raw-field", "input", "header"} {
		assert.NotNil(t, cmd.LocalFlags().Lookup(name), "aura api must declare --%s", name)
	}
	assert.Empty(t, cmd.Annotations["write"], "write-ness is decided from the resolved method, so no static annotation")
}

// TestAPICommand_SecretFlagsAreRedacted walks the live command's flags and
// asserts every secret-bearing long name AND shorthand is matched by
// clievents.RedactArgs, so renaming a shorthand cannot silently fail open — the
// redaction list names the shorthands independently of this command.
func TestAPICommand_SecretFlagsAreRedacted(t *testing.T) {
	cmd := findAPICmd(t)

	testCases := []struct {
		flag   string
		value  string
		secret string
		want   string
	}{
		{flag: "header", value: "Authorization: Bearer s3cret-token", secret: "s3cret-token", want: "***"},
		{flag: "field", value: "password=hunter2", secret: "hunter2", want: "password=***"},
		{flag: "raw-field", value: "client_secret=hunter2", secret: "hunter2", want: "client_secret=***"},
	}

	for _, tc := range testCases {
		t.Run(tc.flag, func(t *testing.T) {
			flag := cmd.LocalFlags().Lookup(tc.flag)
			require.NotNil(t, flag, "aura api must declare --%s", tc.flag)
			require.NotEmpty(t, flag.Shorthand, "--%s must keep its shorthand", tc.flag)

			argvs := [][]string{
				{"aura", "api", "v1/instances", "--" + flag.Name, tc.value},
				{"aura", "api", "v1/instances", "--" + flag.Name + "=" + tc.value},
				{"aura", "api", "v1/instances", "-" + flag.Shorthand, tc.value},
				{"aura", "api", "v1/instances", "-" + flag.Shorthand + tc.value},
			}
			for _, argv := range argvs {
				got := clievents.RedactArgs(argv)
				assert.NotContains(t, got, tc.secret, "RedactArgs(%v) leaked the secret", argv)
				assert.Contains(t, got, tc.want, "RedactArgs(%v)", argv)
			}
		})
	}
}

// TestAPICommand_ExampleShape mirrors the repo-wide leaf gates locally, so a
// drifting Example fails next to the command rather than in agentcontext.
func TestAPICommand_ExampleShape(t *testing.T) {
	cmd := findAPICmd(t)

	require.NotEmpty(t, cmd.Example)
	assert.False(t, strings.HasPrefix(strings.SplitN(cmd.Example, "\n", 2)[0], " "), "Example must be flush-left")
	assert.Contains(t, cmd.Example, "--format json")
	assert.Contains(t, cmd.Example, "--rw")

	comments, invocations := 0, 0
	for _, line := range strings.Split(cmd.Example, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
		case strings.HasPrefix(line, "# "):
			comments++
		default:
			assert.True(t, strings.HasPrefix(trimmed, "neo4j-cli "), "every Example invocation must start with neo4j-cli; got %q", line)
			invocations++
		}
	}
	assert.GreaterOrEqual(t, invocations, 3)
	assert.Equal(t, invocations, comments, "each invocation needs its own `# comment` header")
}

// findAPICmd returns the api command as mounted on the aura root, so the tests
// exercise the real registration rather than a locally built command.
func findAPICmd(t *testing.T) *cobra.Command {
	t.Helper()

	fs, err := testfs.GetTestFs(`{"aura":{}}`, "{}")
	require.NoError(t, err)

	for _, sub := range aura.NewStandaloneCmd(clicfg.NewConfig(fs, "test", clicfg.AuraScope)).Commands() {
		if sub.Name() == "api" {
			return sub
		}
	}
	t.Fatal("the api command is not registered on the aura root")

	return nil
}

func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()

	assert.Equal(t, want, cliError(t, err).Code, "unexpected exit code for error %q", err)
}

func cliError(t *testing.T, err error) *clierr.CLIError {
	t.Helper()

	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)

	return ce
}
