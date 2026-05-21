// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package login

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestConfig creates a clicfg.Config backed by an in-memory filesystem with
// empty Aura credentials, suitable for login command tests.
func newTestConfig(t *testing.T) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"json"}`, `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)
	return clicfg.NewConfig(fs, "test", clicfg.AuraScope)
}

// buildAndRun wires up the login command with the given env vars and runs it,
// returning captured stdout, stderr, and the error returned by cobra.
func buildAndRun(t *testing.T, envVars map[string]string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cfg := newTestConfig(t)
	return buildAndRunWithCfg(t, cfg, envVars, args...)
}

// buildAndRunWithCfg is like buildAndRun but uses the provided cfg so the
// caller can inspect persisted credentials after the command runs.
func buildAndRunWithCfg(t *testing.T, cfg *clicfg.Config, envVars map[string]string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	for k, v := range envVars {
		t.Setenv(k, v)
	}

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}

	cmd := NewCmd(cfg)

	// Wrap in a root command so cmd.Context() is non-nil.
	root := &cobra.Command{Use: "root"}
	root.AddCommand(cmd)
	root.SetOut(outBuf)
	root.SetErr(errBuf)

	cmdArgs := append([]string{"login"}, args...)
	root.SetArgs(cmdArgs)

	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

// allEnvVars returns a complete set of env vars pointing at the given server URL.
func allEnvVars(deviceURL, tokenURL string) map[string]string {
	return map[string]string{
		envDeviceEndpoint: deviceURL,
		envTokenEndpoint:  tokenURL,
		envClientID:       "test-client-id",
		envAudience:       "https://api.example.com",
	}
}

// deviceCodeJSON returns the device authorization response JSON.
func deviceCodeJSON(deviceCode, userCode, verificationURI, verificationURIComplete string, expiresIn, interval int) string {
	m := map[string]interface{}{
		"device_code":      deviceCode,
		"user_code":        userCode,
		"verification_uri": verificationURI,
		"expires_in":       expiresIn,
		"interval":         interval,
	}
	if verificationURIComplete != "" {
		m["verification_uri_complete"] = verificationURIComplete
	}
	b, _ := json.Marshal(m) //nolint:errcheck // test helper; marshal of known types never fails
	return string(b)
}

// tokenErrorJSON returns an RFC 8628 error response.
func tokenErrorJSON(errCode string) string {
	return fmt.Sprintf(`{"error":%q}`, errCode)
}

// writeJSON is a test helper that writes a JSON body to a ResponseWriter;
// write errors in httptest handlers are not actionable so the error is ignored.
func writeJSON(w http.ResponseWriter, body string) {
	fmt.Fprint(w, body) //nolint:errcheck // httptest response write; errors manifest as assertion failures
}

// withNoSleep replaces sleepFn with a no-op for the duration of the test.
func withNoSleep(t *testing.T) {
	t.Helper()
	orig := sleepFn
	sleepFn = func(time.Duration) {}
	t.Cleanup(func() { sleepFn = orig })
}

// withRecordingSleep replaces sleepFn with a function that records durations.
func withRecordingSleep(t *testing.T) *[]time.Duration {
	t.Helper()
	var recorded []time.Duration
	orig := sleepFn
	sleepFn = func(d time.Duration) { recorded = append(recorded, d) }
	t.Cleanup(func() { sleepFn = orig })
	return &recorded
}

// pointHTTPClientAt replaces the package-level httpClient with one that has no
// timeout (tests manage timing themselves) and restores it on cleanup.
func pointHTTPClientAt(t *testing.T) {
	t.Helper()
	orig := httpClient
	httpClient = &http.Client{}
	t.Cleanup(func() { httpClient = orig })
}

// ── readLoginConfig tests ────────────────────────────────────────────────────

func TestReadLoginConfig(t *testing.T) {
	allVars := map[string]string{
		envDeviceEndpoint: "https://device.example.com/authorize",
		envTokenEndpoint:  "https://token.example.com/oauth/token",
		envClientID:       "my-client-id",
		envAudience:       "https://api.example.com",
	}

	setEnv := func(t *testing.T, vars map[string]string) {
		t.Helper()
		for k, v := range vars {
			t.Setenv(k, v)
		}
	}

	t.Run("all vars set returns populated struct", func(t *testing.T) {
		setEnv(t, allVars)
		cfg, err := readLoginConfig()
		assert.NoError(t, err)
		assert.Equal(t, allVars[envDeviceEndpoint], cfg.DeviceEndpoint)
		assert.Equal(t, allVars[envTokenEndpoint], cfg.TokenEndpoint)
		assert.Equal(t, allVars[envClientID], cfg.ClientID)
		assert.Equal(t, allVars[envAudience], cfg.Audience)
	})

	missingVarCases := []struct {
		name       string
		missingVar string
	}{
		{
			name:       "missing device endpoint",
			missingVar: envDeviceEndpoint,
		},
		{
			name:       "missing token endpoint",
			missingVar: envTokenEndpoint,
		},
		{
			name:       "missing client ID",
			missingVar: envClientID,
		},
		{
			name:       "missing audience",
			missingVar: envAudience,
		},
	}

	for _, tc := range missingVarCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set all vars then clear the one under test.
			for k, v := range allVars {
				if k != tc.missingVar {
					t.Setenv(k, v)
				}
			}
			t.Setenv(tc.missingVar, "")

			cfg, err := readLoginConfig()
			assert.Nil(t, cfg)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.missingVar)

			var cliErr *clierr.CLIError
			assert.True(t, errors.As(err, &cliErr), "error should be a CLIError")
			assert.Equal(t, 2, cliErr.Code, "should be a usage error (exit code 2)")
		})
	}
}

// ── full-flow tests using httptest servers ───────────────────────────────────

func TestLoginCommand_HappyPath(t *testing.T) {
	pointHTTPClientAt(t)
	withNoSleep(t)

	const accessToken = "test-access-token-abc123"

	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "test-client-id", r.FormValue("client_id"))
		assert.Equal(t, "https://api.example.com", r.FormValue("audience"))

		w.WriteHeader(http.StatusOK)
		writeJSON(w, deviceCodeJSON("dc-code", "ABCD-1234", "https://verify.example.com", "https://verify.example.com?code=ABCD-1234", 60, 5))
	}))
	defer deviceServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		writeJSON(w, fmt.Sprintf(`{"access_token":%q,"expires_in":3600}`, accessToken))
	}))
	defer tokenServer.Close()

	cfg := newTestConfig(t)
	stdout, stderr, err := buildAndRunWithCfg(t, cfg, allEnvVars(deviceServer.URL, tokenServer.URL))
	require.NoError(t, err)

	// Token must NOT be printed to stdout.
	assert.NotContains(t, stdout, accessToken)
	// Confirmation and verification URL should appear on stderr.
	assert.Contains(t, stderr, "https://verify.example.com?code=ABCD-1234")
	assert.Contains(t, stderr, "Login successful")
	assert.Contains(t, stderr, "set as default")

	// Credential must be persisted.
	require.Len(t, cfg.Credentials.Aura.Credentials, 1)
	cred := cfg.Credentials.Aura.Credentials[0]
	assert.Equal(t, "login", cred.Name)
	assert.Equal(t, "test-client-id", cred.ClientId)
	assert.Equal(t, accessToken, cred.AccessToken)
	assert.Equal(t, "", cred.ClientSecret, "ClientSecret must remain empty for device-auth credentials")
	assert.Equal(t, "login", cfg.Credentials.Aura.DefaultCredential, "new credential should be the default")
}

func TestLoginCommand_HappyPath_OverwritesExistingDefault(t *testing.T) {
	pointHTTPClientAt(t)
	withNoSleep(t)

	const accessToken = "overwrite-default-token"

	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		writeJSON(w, deviceCodeJSON("dc-code", "ABCD-1234", "https://verify.example.com", "https://verify.example.com?code=ABCD-1234", 60, 5))
	}))
	defer deviceServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		writeJSON(w, fmt.Sprintf(`{"access_token":%q,"expires_in":3600}`, accessToken))
	}))
	defer tokenServer.Close()

	cfg := newTestConfig(t)
	// Seed a pre-existing credential and make it the default.
	require.NoError(t, cfg.Credentials.Aura.Add("pre-existing", "cid-old", "secret-old"))
	require.Equal(t, "pre-existing", cfg.Credentials.Aura.DefaultCredential)

	_, stderr, err := buildAndRunWithCfg(t, cfg, allEnvVars(deviceServer.URL, tokenServer.URL))
	require.NoError(t, err)

	assert.Contains(t, stderr, "set as default")

	// "login" must now be the default, overwriting "pre-existing".
	assert.Equal(t, "login", cfg.Credentials.Aura.DefaultCredential, "login must overwrite the pre-existing default")
}

func TestLoginCommand_HappyPath_FallbackVerificationURI(t *testing.T) {
	pointHTTPClientAt(t)
	withNoSleep(t)

	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No verification_uri_complete — should fall back to uri + user_code.
		w.WriteHeader(http.StatusOK)
		writeJSON(w, deviceCodeJSON("dc-code", "WXYZ-5678", "https://verify.example.com/activate", "", 60, 5))
	}))
	defer deviceServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		writeJSON(w, `{"access_token":"tok","expires_in":3600}`)
	}))
	defer tokenServer.Close()

	_, stderr, err := buildAndRun(t, allEnvVars(deviceServer.URL, tokenServer.URL))
	require.NoError(t, err)
	assert.Contains(t, stderr, "https://verify.example.com/activate")
	assert.Contains(t, stderr, "WXYZ-5678")
	assert.Contains(t, stderr, "Login successful")
}

func TestLoginCommand_AuthorizationPendingThenSuccess(t *testing.T) {
	pointHTTPClientAt(t)
	withNoSleep(t)

	const accessToken = "final-token"

	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		writeJSON(w, deviceCodeJSON("dc-code", "ABCD", "https://verify.example.com", "https://verify.example.com?code=ABCD", 300, 5))
	}))
	defer deviceServer.Close()

	// First two calls → authorization_pending; third call → success.
	var callCount int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n < 3 {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, tokenErrorJSON("authorization_pending"))
			return
		}
		w.WriteHeader(http.StatusOK)
		writeJSON(w, fmt.Sprintf(`{"access_token":%q,"expires_in":3600}`, accessToken))
	}))
	defer tokenServer.Close()

	cfg := newTestConfig(t)
	stdout, stderr, err := buildAndRunWithCfg(t, cfg, allEnvVars(deviceServer.URL, tokenServer.URL))
	require.NoError(t, err)
	assert.NotContains(t, stdout, accessToken, "token must not be printed to stdout")
	assert.Contains(t, stderr, "Login successful")
	assert.Equal(t, int32(3), atomic.LoadInt32(&callCount), "expected exactly 3 token requests")

	// Credential must be persisted.
	require.Len(t, cfg.Credentials.Aura.Credentials, 1)
	assert.Equal(t, accessToken, cfg.Credentials.Aura.Credentials[0].AccessToken)
}

func TestLoginCommand_SlowDownIncreasesInterval(t *testing.T) {
	pointHTTPClientAt(t)
	recorded := withRecordingSleep(t)

	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Interval starts at 5 seconds.
		writeJSON(w, deviceCodeJSON("dc-code", "CODE", "https://verify.example.com", "", 300, 5))
	}))
	defer deviceServer.Close()

	// First call → slow_down; second call → success.
	var callCount int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, tokenErrorJSON("slow_down"))
			return
		}
		w.WriteHeader(http.StatusOK)
		writeJSON(w, `{"access_token":"tok","expires_in":3600}`)
	}))
	defer tokenServer.Close()

	_, _, err := buildAndRun(t, allEnvVars(deviceServer.URL, tokenServer.URL))
	require.NoError(t, err)

	// Two sleeps: first at base interval (5s), second after slow_down (+5s = 10s).
	require.Len(t, *recorded, 2)
	assert.Equal(t, 5*time.Second, (*recorded)[0], "first sleep should be the base interval")
	assert.Equal(t, 10*time.Second, (*recorded)[1], "second sleep should be interval + 5s after slow_down")
}

func TestLoginCommand_ExpiredToken(t *testing.T) {
	pointHTTPClientAt(t)
	withNoSleep(t)

	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		writeJSON(w, deviceCodeJSON("dc-code", "CODE", "https://verify.example.com", "", 60, 5))
	}))
	defer deviceServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, tokenErrorJSON("expired_token"))
	}))
	defer tokenServer.Close()

	_, _, err := buildAndRun(t, allEnvVars(deviceServer.URL, tokenServer.URL))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")

	var cliErr *clierr.CLIError
	require.True(t, errors.As(err, &cliErr))
	assert.Equal(t, 2, cliErr.Code, "expired_token should be a usage error")
}

func TestLoginCommand_AccessDenied(t *testing.T) {
	pointHTTPClientAt(t)
	withNoSleep(t)

	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		writeJSON(w, deviceCodeJSON("dc-code", "CODE", "https://verify.example.com", "", 60, 5))
	}))
	defer deviceServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, tokenErrorJSON("access_denied"))
	}))
	defer tokenServer.Close()

	_, _, err := buildAndRun(t, allEnvVars(deviceServer.URL, tokenServer.URL))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "denied")

	var cliErr *clierr.CLIError
	require.True(t, errors.As(err, &cliErr))
	assert.Equal(t, 2, cliErr.Code, "access_denied should be a usage error")
}

func TestLoginCommand_ContextTimeout(t *testing.T) {
	pointHTTPClientAt(t)
	withNoSleep(t)

	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// ExpiresIn=1 so the context times out very quickly.
		writeJSON(w, deviceCodeJSON("dc-code", "CODE", "https://verify.example.com", "", 1, 0))
	}))
	defer deviceServer.Close()

	// Token server keeps returning authorization_pending; timeout should fire first.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block briefly to give the context a chance to expire.
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, tokenErrorJSON("authorization_pending"))
	}))
	defer tokenServer.Close()

	// We need a cancellable root context. Build and run manually.
	for k, v := range allEnvVars(deviceServer.URL, tokenServer.URL) {
		t.Setenv(k, v)
	}

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}

	cfg := newTestConfig(t)
	cmd := NewCmd(cfg)

	root := &cobra.Command{Use: "root"}
	root.AddCommand(cmd)
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs([]string{"login"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := root.ExecuteContext(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestLoginCommand_DeviceEndpointError(t *testing.T) {
	pointHTTPClientAt(t)
	withNoSleep(t)

	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, `{"error":"server_error","description":"internal error"}`)
	}))
	defer deviceServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should never be called.
		t.Error("token endpoint should not be called when device endpoint fails")
	}))
	defer tokenServer.Close()

	_, _, err := buildAndRun(t, allEnvVars(deviceServer.URL, tokenServer.URL))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestLoginCommand_MissingEnvVars(t *testing.T) {
	// Ensure httpClient and sleepFn are not exercised — env errors occur before any HTTP call.
	withNoSleep(t)

	allVars := allEnvVars("https://device.example.com", "https://token.example.com")

	cases := []struct {
		name       string
		missingVar string
	}{
		{name: "missing device endpoint", missingVar: envDeviceEndpoint},
		{name: "missing token endpoint", missingVar: envTokenEndpoint},
		{name: "missing client ID", missingVar: envClientID},
		{name: "missing audience", missingVar: envAudience},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vars := make(map[string]string, len(allVars))
			for k, v := range allVars {
				vars[k] = v
			}
			vars[tc.missingVar] = "" // clear the one under test

			_, _, err := buildAndRun(t, vars)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.missingVar,
				"error message should name the missing variable")

			var cliErr *clierr.CLIError
			require.True(t, errors.As(err, &cliErr), "error should be a CLIError")
			assert.Equal(t, 2, cliErr.Code, "missing env var should be a usage error")
		})
	}
}
