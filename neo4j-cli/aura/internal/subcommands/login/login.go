// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package login

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
)

const (
	envDeviceEndpoint = "NEO4J_AURA_LOGIN_DEVICE_ENDPOINT"
	envTokenEndpoint  = "NEO4J_AURA_LOGIN_TOKEN_ENDPOINT"
	envClientID       = "NEO4J_AURA_LOGIN_CLIENT_ID"
	envAudience       = "NEO4J_AURA_LOGIN_AUDIENCE"
)

type loginConfig struct {
	DeviceEndpoint string
	TokenEndpoint  string
	ClientID       string
	Audience       string
}

func readLoginConfig() (*loginConfig, error) {
	vars := []string{envDeviceEndpoint, envTokenEndpoint, envClientID, envAudience}
	for _, v := range vars {
		if os.Getenv(v) == "" {
			return nil, clierr.NewUsageError("environment variable %s is not set", v)
		}
	}
	return &loginConfig{
		DeviceEndpoint: os.Getenv(envDeviceEndpoint),
		TokenEndpoint:  os.Getenv(envTokenEndpoint),
		ClientID:       os.Getenv(envClientID),
		Audience:       os.Getenv(envAudience),
	}, nil
}

// deviceCodeResponse holds the device authorization response from the device endpoint.
type deviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// httpClient is the package-level HTTP client used for device auth requests.
// It is a variable so tests can substitute a custom transport.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// requestDeviceCode POSTs to the device endpoint and returns the parsed response.
func requestDeviceCode(cfg *loginConfig) (*deviceCodeResponse, error) {
	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("audience", cfg.Audience)

	req, err := http.NewRequest(http.MethodPost, cfg.DeviceEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, clierr.NewFatalError("failed to build device code request: %s", err.Error())
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, clierr.NewUpstreamError("device authorization request failed: %s", err.Error())
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, clierr.NewFatalError("failed to read device endpoint response: %s", err.Error())
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, clierr.NewUpstreamError("device endpoint returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var dcResp deviceCodeResponse
	if err := json.Unmarshal(body, &dcResp); err != nil {
		return nil, clierr.NewFatalError("failed to parse device endpoint response: %s", err.Error())
	}
	return &dcResp, nil
}

// printVerificationPrompt writes the browser-open instructions to stderr.
func printVerificationPrompt(w io.Writer, dcResp *deviceCodeResponse) {
	if dcResp.VerificationURIComplete != "" {
		fmt.Fprintf(w, "Open the following URL in your browser to authenticate:\n\n  %s\n\n", dcResp.VerificationURIComplete) //nolint:errcheck // narration to stderr; write errors are not actionable
	} else {
		fmt.Fprintf(w, "Open the following URL in your browser to authenticate:\n\n  %s\n\nWhen prompted, enter the code: %s\n\n", dcResp.VerificationURI, dcResp.UserCode) //nolint:errcheck // narration to stderr; write errors are not actionable
	}
}

// tokenResponse holds the token endpoint response fields we care about.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

// tokenErrorResponse holds the RFC 8628 error response shape.
type tokenErrorResponse struct {
	Error string `json:"error"`
}

// sleepFn is the package-level sleep function; swapped in tests to avoid real waits.
var sleepFn = time.Sleep

// pollForToken polls the token endpoint until a token is obtained, a terminal
// RFC 8628 error is returned, or the context deadline expires.
func pollForToken(ctx context.Context, cfg *loginConfig, dcResp *deviceCodeResponse) (string, error) {
	interval := time.Duration(dcResp.Interval) * time.Second

	for {
		select {
		case <-ctx.Done():
			return "", clierr.NewUsageError("device authorization timed out: the code expired before you completed authentication")
		default:
		}

		sleepFn(interval)

		// Re-check context after sleeping.
		select {
		case <-ctx.Done():
			return "", clierr.NewUsageError("device authorization timed out: the code expired before you completed authentication")
		default:
		}

		form := url.Values{}
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		form.Set("client_id", cfg.ClientID)
		form.Set("device_code", dcResp.DeviceCode)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenEndpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return "", clierr.NewFatalError("failed to build token request: %s", err.Error())
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := httpClient.Do(req)
		if err != nil {
			// Context cancellation surfaces as an error here too.
			if ctx.Err() != nil {
				return "", clierr.NewUsageError("device authorization timed out: the code expired before you completed authentication")
			}
			return "", clierr.NewUpstreamError("token request failed: %s", err.Error())
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck // response body close; errors are not actionable here

		if err != nil {
			return "", clierr.NewFatalError("failed to read token endpoint response: %s", err.Error())
		}

		if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			var tr tokenResponse
			if err := json.Unmarshal(body, &tr); err != nil {
				return "", clierr.NewFatalError("failed to parse token response: %s", err.Error())
			}
			return tr.AccessToken, nil
		}

		// Non-2xx — parse the RFC 8628 error field.
		var errResp tokenErrorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return "", clierr.NewUpstreamError("token endpoint returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		switch errResp.Error {
		case "authorization_pending":
			// Normal: user hasn't authorized yet; keep polling.
			continue
		case "slow_down":
			// Server requested slower polling; add 5 seconds per RFC 8628 §3.5.
			interval += 5 * time.Second
			continue
		case "expired_token":
			return "", clierr.NewUsageError("device code has expired: please run login again to start a new device authorization flow")
		case "access_denied":
			return "", clierr.NewUsageError("access denied: the user rejected the authorization request")
		default:
			return "", clierr.NewUpstreamError("token endpoint returned error %q: %s", errResp.Error, strings.TrimSpace(string(body)))
		}
	}
}

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Aura using the device authorization flow",
		Long: "Authenticate with Aura using the OAuth 2.0 Device Authorization Grant (RFC 8628).\n" +
			"On success, the access token is printed to stdout.\n\n" +
			"The following environment variables must be set before running:\n" +
			"  NEO4J_AURA_LOGIN_DEVICE_ENDPOINT  Device authorization endpoint URL\n" +
			"  NEO4J_AURA_LOGIN_TOKEN_ENDPOINT    Token endpoint URL\n" +
			"  NEO4J_AURA_LOGIN_CLIENT_ID         Public OAuth client ID\n" +
			"  NEO4J_AURA_LOGIN_AUDIENCE          OAuth audience",
		Example: `# Log in interactively; the command prints a URL to open in your browser
neo4j-cli aura login

# Source the example env file first, then log in
source .env.aura-login-spike && neo4j-cli aura login

# Capture the access token into a shell variable for use in subsequent calls
TOKEN=$(neo4j-cli aura login)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			loginCfg, err := readLoginConfig()
			if err != nil {
				return err
			}

			dcResp, err := requestDeviceCode(loginCfg)
			if err != nil {
				return err
			}

			printVerificationPrompt(cmd.ErrOrStderr(), dcResp)

			ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(dcResp.ExpiresIn)*time.Second)
			defer cancel()

			token, err := pollForToken(ctx, loginCfg, dcResp)
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), token) //nolint:errcheck // writing the token to stdout; write errors are not actionable
			return nil
		},
	}

	return cmd
}
