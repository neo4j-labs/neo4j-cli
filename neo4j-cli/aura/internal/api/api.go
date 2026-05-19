// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clicfg/urlcheck"
	"github.com/neo4j/cli/common/clierr"
)

const userAgent = "Neo4jCLI/%s"

// httpClientTimeout caps every Aura HTTP request so a slow/silent server cannot
// stall the CLI indefinitely. Variable rather than const to let tests dial it
// down (timeout-fires assertion in api_test.go).
var httpClientTimeout = 60 * time.Second

type Grant struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

type AuraApiVersion string

const (
	AuraApiVersion1 AuraApiVersion = "1"
	AuraApiVersion2 AuraApiVersion = "2"
)

type RequestConfig struct {
	Version     AuraApiVersion
	Method      string
	PostBody    map[string]any
	QueryParams map[string]string
}

func MakeRequest(cfg *clicfg.Config, path string, config *RequestConfig) (responseBody []byte, statusCode int, err error) {
	client := http.Client{Timeout: httpClientTimeout}
	var method = config.Method
	if method == "" {
		panic(fmt.Sprintf("method not set in requests %s", path))
	}

	body := createBody(config.PostBody)

	baseUrl := cfg.Aura.BaseUrl()
	if err := urlcheck.ValidateRemoteURL(baseUrl); err != nil {
		return responseBody, 0, clierr.NewUsageError("aura base-url rejected: %s", err.Error())
	}
	if config.Version == "" {
		config.Version = AuraApiVersion1
	}
	versionPath := getVersionPath(cfg, config.Version)

	u, err := url.ParseRequestURI(baseUrl)
	if err != nil {
		return responseBody, 0, clierr.NewUsageError("aura base-url is invalid: %s", err.Error())
	}
	u = u.JoinPath(versionPath)
	u = u.JoinPath(path)

	addQueryParams(u, config.QueryParams)

	urlString := u.String()
	req, err := http.NewRequest(method, urlString, body)

	if err != nil {
		panic(err)
	}

	var credential *credentials.AuraCredential
	if active := cfg.Aura.ActiveCredential(); active != nil {
		credential = active
	} else {
		credential, err = cfg.Credentials.Aura.GetDefault()
		if err != nil {
			return responseBody, 0, err
		}
	}

	req.Header, err = getHeaders(credential, cfg)
	if err != nil {
		return responseBody, 0, err
	}

	res, err := client.Do(req)
	if err != nil {
		panic(err)
	}

	defer res.Body.Close() //nolint:errcheck // response body close error is not actionable in a defer

	if IsSuccessful(res.StatusCode) {
		responseBody, err = io.ReadAll(res.Body)

		if err != nil {
			panic(err)
		}

		if msgs := extractEmbeddedErrors(responseBody); len(msgs) > 0 {
			resourceType, resourceID := parseResourceFromRequest(req)
			return responseBody, res.StatusCode, clierr.NewNotFoundError("%s", formatBracketedMessages(msgs)).WithResource(resourceType, resourceID).WithSuggestion(suggestionForResource(resourceType))
		}

		return responseBody, res.StatusCode, nil
	}

	return responseBody, res.StatusCode, handleResponseError(res, credential, cfg)
}

func getVersionPath(cfg *clicfg.Config, version AuraApiVersion) string {
	betaEnabled := cfg.Flags.Enabled("flag.aura-beta")

	switch version {
	case AuraApiVersion1:
		if betaEnabled {
			return cfg.Aura.BetaPathV1()
		}
		return "v1"
	case AuraApiVersion2:
		return cfg.Aura.BetaPathV2()
	default:
		panic(fmt.Sprintf("version not set in requests %s", version))
	}
}

func createBody(data map[string]any) io.Reader {
	if data == nil {
		return nil
	} else {
		jsonData, err := json.Marshal(data)

		if err != nil {
			panic(err)
		}

		return bytes.NewBuffer(jsonData)
	}
}

func addQueryParams(u *url.URL, params map[string]string) {
	if params != nil {
		q := u.Query()
		for key, val := range params {
			q.Add(key, val)
		}
		u.RawQuery = q.Encode()
	}
}

// Checks status code is 2xx
func IsSuccessful(statusCode int) bool {
	return statusCode >= 200 && statusCode <= 299
}
