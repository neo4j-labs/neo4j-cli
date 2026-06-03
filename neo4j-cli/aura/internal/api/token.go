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

func getToken(credential *credentials.AuraCredential, cfg *clicfg.Config, warnW io.Writer) (string, error) {
	envMode := cfg.Credentials.StorageMode() == credentials.StorageModeEnv

	// In env mode the in-memory token is cleared on every process start, so an
	// identity-bound temp cache lets CI reuse a still-valid token instead of
	// re-minting. The cache re-verifies identity, so a stale/corrupt entry can
	// only cost an extra mint — never identity reuse (defense in depth).
	if envMode && !credential.HasValidAccessToken() {
		if accessToken, tokenExpiry, ok := loadTokenCache(cfg.Aura.Fs(), credential.ClientId, credential.ClientSecret, cfg.Aura.AuthUrl()); ok {
			credential.AccessToken = accessToken
			credential.TokenExpiry = tokenExpiry
			return accessToken, nil
		}
	}

	if credential.HasValidAccessToken() {
		return credential.AccessToken, nil
	}

	data := url.Values{}

	data.Set("grant_type", "client_credentials")

	url := cfg.Aura.AuthUrl()
	if err := urlcheck.ValidateRemoteURL(url); err != nil {
		return "", clierr.NewUsageError("aura auth-url rejected: %s", err.Error())
	}

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(data.Encode()))
	if err != nil {
		panic(clierr.NewFatalError("can't retrieve authentication token. %w", err))
	}

	version := cfg.Version

	req.Header = http.Header{
		"Content-Type": {"application/x-www-form-urlencoded"},
		"User-Agent":   {fmt.Sprintf(userAgent, version)},
	}
	req.SetBasicAuth(credential.ClientId, credential.ClientSecret)

	client := http.Client{Timeout: httpClientTimeout}

	res, err := client.Do(req)
	if err != nil {
		panic(clierr.NewFatalError("can't retrieve authentication token. %w", err))
	}
	defer res.Body.Close() //nolint:errcheck // response body close error is not actionable in a defer

	switch statusCode := res.StatusCode; statusCode {
	case http.StatusUnauthorized:
		return "", clierr.NewAuthError("the provided credentials are invalid, expired, or revoked")
	case http.StatusBadRequest:
	case http.StatusForbidden:
	case http.StatusNotFound:
		panic(clierr.NewFatalError("can't retrieve authentication token. Response status code [%d]", http.StatusBadRequest))
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		panic(clierr.NewFatalError("can't retrieve authentication token. %w", err))
	}

	var grant Grant

	err = json.Unmarshal(resBody, &grant)
	if err != nil {
		panic(clierr.NewFatalError("can't retrieve authentication token. %w", err))
	}

	if _, saveErr := cfg.Credentials.Aura.UpdateAccessToken(credential, grant.AccessToken, grant.ExpiresIn); saveErr != nil {
		fmt.Fprintf(warnW, "Warning: failed to persist access token to keyring: %v\n", saveErr) //nolint:errcheck
	}

	if envMode {
		if cacheErr := saveTokenCache(cfg.Aura.Fs(), credential.ClientId, credential.ClientSecret, cfg.Aura.AuthUrl(), credential.AccessToken, credential.TokenExpiry); cacheErr != nil {
			fmt.Fprintf(warnW, "Warning: failed to cache access token: %v\n", cacheErr) //nolint:errcheck
		}
	}
	return grant.AccessToken, err
}
