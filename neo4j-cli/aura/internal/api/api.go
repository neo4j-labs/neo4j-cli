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
	"os"
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
	AuraApiVersion1     AuraApiVersion = "1"
	AuraApiVersion2     AuraApiVersion = "2"
	AuraApiVersionBeta1 AuraApiVersion = "beta1"
)

type RequestConfig struct {
	Version     AuraApiVersion
	Method      string
	PostBody    map[string]any
	QueryParams map[string]string
	// WarnW receives keyring-failure warnings produced during token acquisition.
	// If nil, warnings are written to os.Stderr.
	WarnW io.Writer
	// ResponseHeader, when non-nil, receives the response header on every return
	// path so callers can read headers (e.g. X-Agent-Invocation-Id) regardless
	// of success or failure.
	ResponseHeader *http.Header
}

func MakeRequest(cfg *clicfg.Config, path string, config *RequestConfig) (responseBody []byte, statusCode int, err error) {
	client := http.Client{Timeout: httpClientTimeout}
	var method = config.Method
	if method == "" {
		panic(fmt.Sprintf("method not set in requests %s", path))
	}

	warnW := config.WarnW
	if warnW == nil {
		warnW = os.Stderr
	}

	bodyBytes := marshalBody(config.PostBody)

	if config.Version == "" {
		config.Version = AuraApiVersion1
	}
	versionPath := getVersionPath(config.Version)

	req, credential, err := prepareRequest(cfg, method, versionPath, path, bodyBytes, queryValues(config.QueryParams), warnW)
	if err != nil {
		return responseBody, 0, err
	}

	debug := cfg.Aura.Debug()

	start := time.Now()
	res, err := client.Do(req)
	if err != nil {
		panic(err)
	}

	defer res.Body.Close() //nolint:errcheck // response body close error is not actionable in a defer

	if config.ResponseHeader != nil {
		*config.ResponseHeader = res.Header
	}

	if IsSuccessful(res.StatusCode) {
		responseBody, err = io.ReadAll(res.Body)

		if err != nil {
			panic(err)
		}

		if debug {
			debugResponse(res.StatusCode, res.Header, responseBody, time.Since(start))
		}

		if msgs := extractEmbeddedErrors(responseBody); len(msgs) > 0 {
			resourceType, resourceID := parseResourceFromRequest(req)
			return responseBody, res.StatusCode, clierr.NewNotFoundError("%s", formatBracketedMessages(msgs)).WithResource(resourceType, resourceID).WithSuggestion(suggestionForResource(resourceType))
		}

		return responseBody, res.StatusCode, nil
	}

	if debug {
		errBody, _ := io.ReadAll(res.Body)
		debugResponse(res.StatusCode, res.Header, errBody, time.Since(start))
		res.Body = io.NopCloser(bytes.NewReader(errBody))
	}

	return responseBody, res.StatusCode, handleResponseError(res, credential, cfg)
}

// prepareRequest is the shared prologue for every Aura request entrypoint, so a
// second entrypoint inherits identical auth and SSRF gating. An empty
// versionPath omits the version segment entirely. The returned credential is
// the one the request was signed with, needed to clear a stale access token on
// a 401.
func prepareRequest(cfg *clicfg.Config, method, versionPath, path string, bodyBytes []byte, queryParams url.Values, warnW io.Writer) (*http.Request, *credentials.AuraCredential, error) {
	baseUrl := cfg.Aura.BaseUrl()
	if err := urlcheck.ValidateRemoteURL(baseUrl); err != nil {
		return nil, nil, clierr.NewUsageError("aura base-url rejected: %s", err.Error())
	}

	u, err := url.ParseRequestURI(baseUrl)
	if err != nil {
		return nil, nil, clierr.NewUsageError("aura base-url is invalid: %s", err.Error())
	}
	if versionPath != "" {
		u = u.JoinPath(versionPath)
	}
	u = u.JoinPath(path)

	addQueryParams(u, queryParams)

	var body io.Reader
	if bodyBytes != nil {
		body = bytes.NewReader(bodyBytes)
	}

	urlString := u.String()
	// http.NewRequest only fails on a malformed method token or an unparsable
	// URL, both already screened by the callers; it is returned rather than
	// panicked on so no entrypoint inherits a panic on user-supplied input.
	req, err := http.NewRequest(method, urlString, body)
	if err != nil {
		return nil, nil, clierr.NewUsageError("aura request could not be built: %s", err.Error())
	}

	var credential *credentials.AuraCredential
	if active := cfg.Aura.ActiveCredential(); active != nil {
		credential = active
	} else {
		credential, err = cfg.Credentials.Aura.GetDefault()
		if err != nil {
			return nil, nil, err
		}
	}

	req.Header, err = getHeaders(credential, cfg, warnW)
	if err != nil {
		return nil, nil, err
	}

	if cfg.Aura.Debug() {
		debugRequest(method, urlString, req.Header, bodyBytes)
	}

	return req, credential, nil
}

func getVersionPath(version AuraApiVersion) string {
	switch version {
	case AuraApiVersion1:
		return "v1"
	case AuraApiVersionBeta1:
		return "v1beta5"
	case AuraApiVersion2:
		return "v2beta1"
	default:
		panic(fmt.Sprintf("version not set in requests %s", version))
	}
}

func marshalBody(data map[string]any) []byte {
	if data == nil {
		return nil
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}

	return jsonData
}

func queryValues(params map[string]string) url.Values {
	if len(params) == 0 {
		return nil
	}

	values := make(url.Values, len(params))
	for key, val := range params {
		values.Add(key, val)
	}
	return values
}

func addQueryParams(u *url.URL, params url.Values) {
	if len(params) == 0 {
		return
	}

	q := u.Query()
	for key, vals := range params {
		for _, val := range vals {
			q.Add(key, val)
		}
	}
	u.RawQuery = q.Encode()
}

// Checks status code is 2xx
func IsSuccessful(statusCode int) bool {
	return statusCode >= 200 && statusCode <= 299
}
