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
	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/common/output"
)

// authSuggestion is the next-action hint attached to every 401/403 *CLIError
// produced by handleResponseError / formatAuthorizationError. Centralised so
// the four constructor sites stay in lock-step.
const authSuggestion = "Run 'neo4j-cli credential aura-client add' to refresh credentials, then retry."

// formatBracketedMessages renders a slice of upstream error messages in the
// multi-line bracket shape used across every handleResponseError envelope
// site (and formatAuthorizationError). Output is byte-identical to the
// long-standing 401/403 format so all seven sites stay in lock-step rather
// than relying on Go's default []string stringification.
func formatBracketedMessages(messages []string) string {
	return fmt.Sprintf("[\n\t%s\n]", strings.Join(messages, ",\n\t"))
}

// extractEmbeddedErrors decodes a 2xx response body into the shared
// ErrorResponse shape and returns each Error.Message verbatim. Returns nil
// when the body cannot be parsed as an ErrorResponse or when Errors is empty
// so callers can treat the absence of embedded errors as the happy path.
// Matches the 404 branch (no `field:` prefix) because the 2xx-with-errors
// shape is the single-resource SingleValueResponseData get path.
func extractEmbeddedErrors(body []byte) []string {
	var errorResponse ErrorResponse
	if err := json.Unmarshal(body, &errorResponse); err != nil {
		return nil
	}
	if len(errorResponse.Errors) == 0 {
		return nil
	}
	messages := make([]string, 0, len(errorResponse.Errors))
	for _, e := range errorResponse.Errors {
		messages = append(messages, e.Message)
	}
	return messages
}

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
		panic(clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in %s", statusCode, clievents.RedactArgs(os.Args[1:]), clierr.IssuesURL))
	// client error responses
	case http.StatusBadRequest:
		var errorResponse ErrorResponse

		err = json.Unmarshal(resBody, &errorResponse)
		if err != nil {
			panic(clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in %s", statusCode, clievents.RedactArgs(os.Args[1:]), clierr.IssuesURL))
		}

		messages := []string{}
		for _, e := range errorResponse.Errors {
			message := e.Message
			if e.Field != "" {
				message = fmt.Sprintf("%s: %s", e.Field, e.Message)
			}
			messages = append(messages, message)
		}

		return clierr.NewValidationError("%s", formatBracketedMessages(messages)).WithSuggestion("See 'neo4j-cli aura <cmd> --help' for valid flags and values.")
	case http.StatusUnauthorized:
		return formatAuthorizationError(resBody, statusCode, credential, cfg)
	case http.StatusForbidden:
		// Requested endpoint is forbidden
		var serverError ServerError
		err := json.Unmarshal(resBody, &serverError)
		if err != nil {
			panic(clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in %s", statusCode, clievents.RedactArgs(os.Args[1:]), clierr.IssuesURL))
		}
		if serverError.Error != "" {
			return clierr.NewAuthError("%s", serverError.Error).WithSuggestion(authSuggestion)
		}

		return formatAuthorizationError(resBody, statusCode, credential, cfg)
	case http.StatusNotFound:
		var errorResponse ErrorResponse

		if err = json.Unmarshal(resBody, &errorResponse); err != nil {
			return clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in %s", statusCode, clievents.RedactArgs(os.Args[1:]), clierr.IssuesURL)
		}

		messages := []string{}
		for _, e := range errorResponse.Errors {
			messages = append(messages, e.Message)
		}

		resourceType, resourceID := parseResourceFromRequest(res.Request)
		return clierr.NewNotFoundError("%s", formatBracketedMessages(messages)).WithResource(resourceType, resourceID).WithSuggestion(suggestionForResource(resourceType))
	case http.StatusMethodNotAllowed:
		var errorResponse ErrorResponse

		if err = json.Unmarshal(resBody, &errorResponse); err != nil {
			return clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in %s", statusCode, clievents.RedactArgs(os.Args[1:]), clierr.IssuesURL)
		}

		messages := []string{}
		for _, e := range errorResponse.Errors {
			messages = append(messages, e.Message)
		}

		return clierr.NewUpstreamError("%s", formatBracketedMessages(messages))
	case http.StatusPaymentRequired:
		var errorResponse ErrorResponse

		if err = json.Unmarshal(resBody, &errorResponse); err != nil {
			return clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in %s", statusCode, clievents.RedactArgs(os.Args[1:]), clierr.IssuesURL)
		}

		messages := []string{}
		for _, e := range errorResponse.Errors {
			messages = append(messages, e.Message)
		}

		cliErr := clierr.NewConflictError("%s", formatBracketedMessages(messages))
		if s := suggestionForPaymentRequired(errorResponse); s != "" {
			cliErr = cliErr.WithSuggestion(s)
		}
		return cliErr
	case http.StatusConflict:
		var errorResponse ErrorResponse

		if err = json.Unmarshal(resBody, &errorResponse); err != nil {
			return clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in %s", statusCode, clievents.RedactArgs(os.Args[1:]), clierr.IssuesURL)
		}

		messages := []string{}
		for _, e := range errorResponse.Errors {
			messages = append(messages, e.Message)
		}

		return clierr.NewConflictError("%s", formatBracketedMessages(messages))
	case http.StatusUnsupportedMediaType:
		panic(clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in %s", statusCode, clievents.RedactArgs(os.Args[1:]), clierr.IssuesURL))
	case http.StatusTooManyRequests:
		retryAfter := res.Header.Get("Retry-After")
		return clierr.NewRateLimitError(retryAfter, "server rate limit exceeded, suggested cool-off period is %s seconds before rerunning the command", retryAfter).WithSuggestion(fmt.Sprintf("Retry after %s seconds.", retryAfter))
	// server error responses
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		var errorResponse ErrorResponse

		if err = json.Unmarshal(resBody, &errorResponse); err != nil {
			return clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, please report an issue in %s", statusCode, clievents.RedactArgs(os.Args[1:]), clierr.IssuesURL)
		}

		messages := []string{}
		for _, e := range errorResponse.Errors {
			messages = append(messages, e.Message)
		}

		return clierr.NewUpstreamError("%s", formatBracketedMessages(messages))
	default:
		panic(clierr.NewFatalError("unexpected status code %d and body %s running CLI with args %s, please report an issue in %s", statusCode, resBody, clievents.RedactArgs(os.Args[1:]), clierr.IssuesURL))
	}
}

// parseResourceFromRequest extracts a (resourceType, resourceID) pair from
// the request URL path so the JSON error envelope can surface them on a 404.
// Paths follow the Aura shape `/<version>/<plural-resource>/<id>[/...]` (e.g.
// `/v1/instances/abc123` or `/v1beta5/tenants/abc123/metrics-integration`).
// Returns ("", "") when the request, URL, or path doesn't fit the shape so
// the envelope omitempty drops both fields rather than emitting noise.
func parseResourceFromRequest(req *http.Request) (string, string) {
	if req == nil || req.URL == nil {
		return "", ""
	}
	segments := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	// Need at least <version> / <plural> / <id> — three segments.
	if len(segments) < 3 {
		return "", ""
	}
	// Skip the version segment (segments[0]); segments[1] is the plural
	// resource name, segments[2] is the resource id.
	return singularise(segments[1]), segments[2]
}

// suggestionForResource returns the per-resource next-action hint attached to
// 404 *CLIErrors via .WithSuggestion(...). The lookup is keyed on the singular
// resourceType produced by parseResourceFromRequest. Unknown / empty types
// return "" so the envelope omitempty drops the field rather than emitting
// noise (e.g. nested-path 404s where parseResourceFromRequest mis-segments —
// those are enriched at the call site via utils.WithNotFoundContext).
func suggestionForResource(resourceType string) string {
	switch resourceType {
	case "instance":
		return "Run 'neo4j-cli aura instance list' to see available instances."
	case "project":
		return "Run 'neo4j-cli aura project list --organization-id <id>' to see available projects."
	case "organization":
		return "Run 'neo4j-cli aura organization list' to see available organizations."
	case "customer-managed-key":
		return "Run 'neo4j-cli aura customer-managed-key list' to see customer-managed keys."
	case "tenant":
		return "Run 'neo4j-cli aura project list' to see available projects (tenants are now called projects)."
	default:
		return ""
	}
}

// suggestionForPaymentRequired returns the next-action hint attached to 402
// *CLIErrors via .WithSuggestion(...). The 402 body's errors[].reason field is
// inspected; when any entry signals `quota-exceeded` (the stable Aura v2beta1
// enum value), a type-agnostic instance-quota suggestion is returned. Any
// other reason (or an empty errors[]) returns "" so the envelope omitempty
// drops the field and the API's own message remains the primary signal.
func suggestionForPaymentRequired(resp ErrorResponse) string {
	for _, e := range resp.Errors {
		if e.Reason == "quota-exceeded" {
			return "You've reached your quota for this instance type. Delete an existing instance with 'neo4j-cli aura instance list' then 'neo4j-cli aura instance delete <id>', or pick a different --type."
		}
	}
	return ""
}

// singularise turns the plural resource path segment (e.g. "instances",
// "tenants", "customer-managed-keys") into its singular form. The Aura API
// consistently uses simple `-s` plurals so a trailing-s strip is enough;
// unknown shapes are returned unchanged so the envelope still carries
// something meaningful.
func singularise(plural string) string {
	if strings.HasSuffix(plural, "s") && len(plural) > 1 {
		return strings.TrimSuffix(plural, "s")
	}
	return plural
}

func getHeaders(credential *credentials.AuraCredential, cfg *clicfg.Config, warnW io.Writer) (http.Header, error) {
	token, err := getToken(credential, cfg, warnW)

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

type ResponseData interface {
	output.ResponseData
	GetSingleOrError() (map[string]any, error)
}

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

// ParseRawBody parses a bare-JSON response body (no `{"data": ...}` envelope)
// into a ResponseData. It tries `[]map[string]any` first then `map[string]any`,
// panicking if neither matches. Used by endpoints (e.g. the Aura Agents API)
// whose response shape is a bare array or a bare object at the top level.
func ParseRawBody(body []byte) ResponseData {
	var listData []map[string]any
	if err := json.Unmarshal(body, &listData); err == nil {
		return NewListResponseData(listData)
	}

	var singleData map[string]any
	if err := json.Unmarshal(body, &singleData); err == nil {
		return NewSingleValueResponseData(singleData)
	}

	panic("could not parse raw response body")
}

func formatAuthorizationError(resBody []byte, statusCode int, credential *credentials.AuraCredential, cfg *clicfg.Config) error {
	var errorResponse ErrorResponse

	err := json.Unmarshal(resBody, &errorResponse)
	if err != nil {
		return clierr.NewAuthError("unexpected error [status %d] running CLI with args %s, please report an issue in %s", statusCode, clievents.RedactArgs(os.Args[1:]), clierr.IssuesURL).WithSuggestion(authSuggestion)
	}

	messages := []string{}
	for _, e := range errorResponse.Errors {
		messages = append(messages, e.Message)
	}

	_, err = cfg.Credentials.Aura.ClearAccessToken(credential)
	if err != nil {
		messages = append(messages, fmt.Sprintf("Request failed authorization - attempted to clear the access token but encountered an error, please report an issue in %s", clierr.IssuesURL))
	} else {
		messages = append(messages, "Request failed authorization - access token has been cleared and will be refreshed on next request - please retry the command")
	}

	return clierr.NewAuthError("%s", formatBracketedMessages(messages)).WithSuggestion(authSuggestion)
}
