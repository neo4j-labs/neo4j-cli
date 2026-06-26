// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clicfg/urlcheck"
	"github.com/neo4j/cli/common/clierr"
)

// mintToken performs the OAuth client-credentials exchange. It is a
// package-level seam so tests can count mints and bypass HTTP.
var mintToken = mintTokenHTTP

func getToken(credential *credentials.AuraCredential, cfg *clicfg.Config, warnW io.Writer) (string, error) {
	debug := cfg.Aura.Debug()

	if credential.HasValidAccessToken() {
		if debug {
			debugInfo("reusing cached valid access token")
		}
		return credential.AccessToken, nil
	}

	authURL := cfg.Aura.AuthUrl()
	if err := urlcheck.ValidateRemoteURL(authURL); err != nil {
		return "", clierr.NewUsageError("aura auth-url rejected: %s", err.Error())
	}

	// An env-var-synthesized credential (accept-env-vars mode) carries the
	// Ephemeral marker set at synthesis. For such credentials the derived JWT is
	// cached on disk keyed by the (id|secret|authURL) identity so repeated
	// short-lived CI invocations reuse one mint. Stored (keyring/insecure)
	// credentials are persisted via the store instead and never touch this disk
	// cache.
	envMode := credential.Ephemeral && cfg.Global.AcceptEnvVars()

	var fullHash, cachePath string
	if envMode {
		var short string
		fullHash, short = tokenCacheHash(credential.ClientId, credential.ClientSecret, authURL)
		cachePath = tokenCachePath(short)
		if token, ok := readTokenCache(cachePath, fullHash); ok {
			if debug {
				debugInfo("reusing cached env-var access token from disk")
			}
			return token, nil
		}
	}

	if debug {
		debugInfo("no cached access token; fetching new one from %s", authURL)
	}

	grant, err := mintToken(credential, cfg)
	if err != nil {
		return "", err
	}

	if envMode {
		if writeErr := writeTokenCache(cachePath, fullHash, grant.AccessToken, grant.ExpiresIn); writeErr != nil && debug {
			debugInfo("failed to write env-var access token cache: %v", writeErr)
		}
		return grant.AccessToken, nil
	}

	// A stored (keyring/insecure) credential persists its token via the store.
	// A not-in-store credential outside env-var mode (e.g. an in-process active
	// credential) is kept in-memory only — UpdateAccessToken would panic on its
	// missing Get lookup.
	if _, getErr := cfg.Credentials.Aura.Get(credential.Name); getErr == nil {
		if _, saveErr := cfg.Credentials.Aura.UpdateAccessToken(credential, grant.AccessToken, grant.ExpiresIn); saveErr != nil {
			fmt.Fprintf(warnW, "Warning: failed to persist access token to keyring: %v\n", saveErr) //nolint:errcheck
		}
	}
	return grant.AccessToken, nil
}

func mintTokenHTTP(credential *credentials.AuraCredential, cfg *clicfg.Config) (Grant, error) {
	debug := cfg.Aura.Debug()

	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	authURL := cfg.Aura.AuthUrl()

	req, err := http.NewRequest(http.MethodPost, authURL, strings.NewReader(data.Encode()))
	if err != nil {
		panic(clierr.NewFatalError("can't retrieve authentication token. %w", err))
	}

	req.Header = http.Header{
		"Content-Type": {"application/x-www-form-urlencoded"},
		"User-Agent":   {fmt.Sprintf(userAgent, cfg.Version)},
	}
	req.SetBasicAuth(credential.ClientId, credential.ClientSecret)

	client := http.Client{Timeout: httpClientTimeout}

	res, err := client.Do(req)
	if err != nil {
		panic(clierr.NewFatalError("can't retrieve authentication token. %w", err))
	}
	defer res.Body.Close() //nolint:errcheck // response body close error is not actionable in a defer

	if debug {
		debugInfo("auth response status %d %s", res.StatusCode, http.StatusText(res.StatusCode))
	}

	switch statusCode := res.StatusCode; statusCode {
	case http.StatusUnauthorized:
		return Grant{}, clierr.NewAuthError("the provided credentials are invalid, expired, or revoked")
	case http.StatusForbidden:
		return Grant{}, clierr.NewAuthError("forbidden (HTTP 403): the credentials are valid but not authorized for this request (insufficient permission)")
	default:
		if statusCode < 200 || statusCode >= 300 {
			return Grant{}, clierr.NewUpstreamError("can't retrieve authentication token: the authentication endpoint returned HTTP %d %s", statusCode, http.StatusText(statusCode))
		}
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		panic(clierr.NewFatalError("can't retrieve authentication token. %w", err))
	}

	var grant Grant
	if err := json.Unmarshal(resBody, &grant); err != nil {
		panic(clierr.NewFatalError("can't retrieve authentication token. %w", err))
	}
	if grant.AccessToken == "" {
		return Grant{}, clierr.NewUpstreamError("can't retrieve authentication token: the authentication endpoint returned an empty token")
	}
	return grant, nil
}
