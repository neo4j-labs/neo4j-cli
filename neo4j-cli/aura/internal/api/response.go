// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	common_output "github.com/neo4j/cli/common/output"
)

type ErrorResponse struct {
	Errors []Error `json:"errors"`
}

type Error struct {
	Message string `json:"message"`
	Reason  string `json:"reason"`
	Field   string `json:"field"`
}

type ServerError struct {
	Error string `json:"error"`
}

func handleResponseError(res *http.Response, credential *credentials.AuraCredential, cfg *clicfg.Config) error {
	resBody, err := io.ReadAll(res.Body)

	if err != nil {
		panic(clierr.NewFatalError("unexpected error reading response body. %w", err))
	}

	switch statusCode := res.StatusCode; statusCode {
	// redirection messages
	case http.StatusPermanentRedirect:
		panic(clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in https://github.com/neo4j/cli", statusCode, os.Args[1:]))
	// client error responses
	case http.StatusBadRequest:
		var errorResponse ErrorResponse

		err = json.Unmarshal(resBody, &errorResponse)
		if err != nil {
			panic(clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in https://github.com/neo4j/cli", statusCode, os.Args[1:]))
		}

		messages := []string{}
		for _, e := range errorResponse.Errors {
			message := e.Message
			if e.Field != "" {
				message = fmt.Sprintf("%s: %s", e.Field, e.Message)
			}
			messages = append(messages, message)
		}

		return clierr.NewUpstreamError("%s", messages)
	case http.StatusUnauthorized:
		return formatAuthorizationError(resBody, statusCode, credential, cfg)
	case http.StatusForbidden:
		// Requested endpoint is forbidden
		var serverError ServerError
		err := json.Unmarshal(resBody, &serverError)
		if err != nil {
			panic(clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in https://github.com/neo4j/cli", statusCode, os.Args[1:]))
		}
		if serverError.Error != "" {
			return clierr.NewUpstreamError("%s", serverError.Error)
		}

		return formatAuthorizationError(resBody, statusCode, credential, cfg)
	case http.StatusNotFound:
		var errorResponse ErrorResponse

		err = json.Unmarshal(resBody, &errorResponse)
		if err != nil {
			panic(clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in https://github.com/neo4j/cli", statusCode, os.Args[1:]))
		}

		messages := []string{}
		for _, e := range errorResponse.Errors {
			messages = append(messages, e.Message)
		}

		return clierr.NewUpstreamError("%s", messages)
	case http.StatusMethodNotAllowed:
		var errorResponse ErrorResponse

		err = json.Unmarshal(resBody, &errorResponse)
		if err != nil {
			panic(clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in https://github.com/neo4j/cli", statusCode, os.Args[1:]))
		}

		messages := []string{}
		for _, e := range errorResponse.Errors {
			messages = append(messages, e.Message)
		}

		return clierr.NewUpstreamError("%s", messages)
	case http.StatusConflict:
		var errorResponse ErrorResponse

		err = json.Unmarshal(resBody, &errorResponse)
		if err != nil {
			panic(clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in https://github.com/neo4j/cli", statusCode, os.Args[1:]))
		}

		messages := []string{}
		for _, e := range errorResponse.Errors {
			messages = append(messages, e.Message)
		}

		return clierr.NewUpstreamError("%s", messages)
	case http.StatusUnsupportedMediaType:
		panic(clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in https://github.com/neo4j/cli", statusCode, os.Args[1:]))
	case http.StatusTooManyRequests:
		retryAfter := res.Header.Get("Retry-After")
		return clierr.NewUpstreamError("server rate limit exceeded, suggested cool-off period is %s seconds before rerunning the command", retryAfter)
	// server error responses
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		var errorResponse ErrorResponse

		err = json.Unmarshal(resBody, &errorResponse)
		if err != nil {
			panic(clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in https://github.com/neo4j/cli", statusCode, os.Args[1:]))
		}

		messages := []string{}
		for _, e := range errorResponse.Errors {
			messages = append(messages, e.Message)
		}

		return clierr.NewUpstreamError("%s", messages)
	default:
		panic(clierr.NewFatalError("unexpected status code %d and body %s running CLI with args %s, please report an issue in https://github.com/neo4j/cli", statusCode, resBody, os.Args[1:]))
	}
}

func getHeaders(credential *credentials.AuraCredential, cfg *clicfg.Config) (http.Header, error) {
	token, err := getToken(credential, cfg)

	if err != nil {
		return nil, err
	}

	version := cfg.Version

	return http.Header{
		"Content-Type":  {"application/json"},
		"Authorization": {fmt.Sprintf("Bearer %s", token)},
		"User-Agent":    {fmt.Sprintf(userAgent, version)},
	}, nil
}

// Response types

const (
	InstanceStatusCreating      string = "creating"
	InstanceStatusDestroying    string = "destroying"
	InstanceStatusRunning       string = "running"
	InstanceStatusPausing       string = "pausing"
	InstanceStatusPaused        string = "paused"
	InstanceStatusSuspending    string = "suspending"
	InstanceStatusSuspended     string = "suspended"
	InstanceStatusResuming      string = "resuming"
	InstanceStatusLoading       string = "loading"
	InstanceStatusLoadingFailed string = "loading failed"
	InstanceStatusRestoring     string = "restoring"
	InstanceStatusUpdating      string = "updating"
	InstanceStatusOverwriting   string = "overwriting"
)

const (
	SnapshotStatusPending    string = "Pending"
	SnapshotStatusCompleted  string = "Completed"
	SnapshotStatusInProgress string = "InProgress"
	SnapshotStatusFailed     string = "Failed"
)

// Response Body of Create and Get Instance for successful requests
type CreateInstanceResponse struct {
	Data struct {
		Id            string
		ConnectionUrl string `json:"connection_url"`
		Username      string
		Password      string
		TenantId      string `json:"tenant_id"`
		CloudProvider string `json:"cloud_provider"`
		Region        string
		Type          string
		Name          string
	}
}

const (
	CMKStatusReady   = "ready"
	CMKStatusPending = "pending"
)

// Response Body of Create and Get Instance for successful requests
type CreateCMKResponse struct {
	Data struct {
		Id     string
		Status string
	}
}

// Response Body of Create and Get Instance for successful requests
type CreateSnapshotResponse struct {
	Data struct {
		SnapshotId string `json:"snapshot_id"`
	}
}

// Response Body of Create GraphQL Data API for successful requests
type CreateGraphQLDataApiResponse struct {
	Data struct {
		Id                      string
		Name                    string
		Status                  string
		Url                     string
		AuthenticationProviders []struct {
			Id      string
			Name    string
			Type    string
			Enabled bool
			Key     string `json:"key,omitempty"`
			Url     string `json:"url,omitempty"`
		} `json:"authentication_providers"`
	}
}

const (
	GraphQLDataApiStatusReady    = "ready"
	GraphQLDataApiStatusCreating = "creating"
	GraphQLDataApiStatusUpdating = "updating"
	GraphQLDataApiStatusDeleting = "deleting"
	GraphQLDataApiStatusPausing  = "pausing"
	GraphQLDataApiStatusResuming = "resuming"
	GraphQLDataApiStatusPaused   = "paused"
	GraphQLDataApiStatusError    = "error"
)

const (
	GraphQLDataApiAuthProviderTypeJwks   = "jwks"
	GraphQLDataApiAuthProviderTypeApiKey = "api-key"
)

const (
	GraphAnalyticsSessionInitial  = ""
	GraphAnalyticsSessionCreating = "Creating"
	GraphAnalyticsSessionReady    = "Ready"
	GraphAnalyticsSessionExpired  = "Expired"
	GraphAnalyticsSessionFailed   = "Failed"
)

var GraphAnalyticsSessionWaitingStatus = []string{
	GraphAnalyticsSessionCreating,
	GraphAnalyticsSessionInitial,
}

// ResponseData is a type alias for common/output.ResponseData so that existing
// callers using api.ResponseData continue to compile without import changes.
type ResponseData = common_output.ResponseData

type ListResponseData struct {
	Data []map[string]any `json:"data"`
}

func (d ListResponseData) GetSingleOrError() (map[string]any, error) {
	if len(d.Data) != 1 {
		return nil, clierr.NewFatalError("expected 1 array value: %v", len(d.Data))
	}
	return d.Data[0], nil
}

func (d ListResponseData) AsArray() []map[string]any {
	return d.Data
}

type SingleValueResponseData struct {
	Data   map[string]any   `json:"data"`
	Errors []map[string]any `json:"errors,omitempty"`
}

func (d SingleValueResponseData) GetSingleOrError() (map[string]any, error) {
	return d.Data, nil
}

func (d SingleValueResponseData) AsArray() []map[string]any {
	return []map[string]any{d.Data}
}

func NewSingleValueResponseData(data map[string]any) ResponseData {
	return SingleValueResponseData{
		Data: data,
	}
}

func NewListResponseData(data []map[string]any) ResponseData {
	return ListResponseData{
		Data: data,
	}
}

func ParseBody(body []byte) ResponseData {
	var listResponseData ListResponseData
	err := json.Unmarshal(body, &listResponseData)

	// Try unmarshalling array first, if not it creates an array from the single item
	if err == nil {
		return listResponseData
	} else {
		var singleValueResponseData SingleValueResponseData
		err := json.Unmarshal(body, &singleValueResponseData)
		if err != nil {
			panic(err)
		}
		return singleValueResponseData
	}
}

func formatAuthorizationError(resBody []byte, statusCode int, credential *credentials.AuraCredential, cfg *clicfg.Config) error {
	var errorResponse ErrorResponse

	err := json.Unmarshal(resBody, &errorResponse)
	if err != nil {
		return clierr.NewUsageError("unexpected error [status %d] running CLI with args %s, please report an issue in https://github.com/neo4j/cli", statusCode, os.Args[1:])
	}

	messages := []string{}
	for _, e := range errorResponse.Errors {
		messages = append(messages, e.Message)
	}

	_, err = cfg.Credentials.Aura.ClearAccessToken(credential)
	if err != nil {
		messages = append(messages, "Request failed authorization - attempted to clear the access token but encountered an error, please report an issue in https://github.com/neo4j/cli")
	} else {
		messages = append(messages, "Request failed authorization - access token has been cleared and will be refreshed on next request - please retry the command")
	}

	return clierr.NewUsageError(`[
	%s
]`, strings.Join(messages, ",\n\t"))
}
