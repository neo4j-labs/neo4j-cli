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
	debug := cfg.Aura.Debug()

	if credential.HasValidAccessToken() {
		if debug {
			debugInfo("reusing cached valid access token")
		}
		return credential.AccessToken, nil
	}

	data := url.Values{}

	data.Set("grant_type", "client_credentials")

	url := cfg.Aura.AuthUrl()
	if err := urlcheck.ValidateRemoteURL(url); err != nil {
		return "", clierr.NewUsageError("aura auth-url rejected: %s", err.Error())
	}

	if debug {
		debugInfo("no cached access token; fetching new one from %s", url)
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

	if debug {
		debugInfo("auth response status %d %s", res.StatusCode, http.StatusText(res.StatusCode))
	}

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
	return grant.AccessToken, err
}
