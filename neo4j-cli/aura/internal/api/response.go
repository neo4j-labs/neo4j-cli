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

// upstreamDetail describes a response whose body did not match ErrorResponse —
// the normal case on v2beta1, where every documented 4xx/5xx except two billing
// 400s declares a description with no content schema. The body always goes
// through scrubbedBodyTrunc: the panic this replaced interpolated resBody raw,
// so a secret echoed back by the API could reach stdout.
func upstreamDetail(statusCode int, resBody []byte) string {
	if body := scrubbedBodyTrunc(resBody); body != "" {
		return fmt.Sprintf("upstream error [status %d]: %s", statusCode, body)
	}
	return fmt.Sprintf("upstream error [status %d] with no response body", statusCode)
}

// errorMessages extracts the upstream messages, returning nil when the body
// did not parse OR parsed to an empty errors[] — any valid JSON object
// unmarshals into ErrorResponse, and rendering that as "[\n\t\n]" tells the
// user nothing. Both cases fall back to the raw body via upstreamDetail.
// withField prefixes "<field>: " (the 400 branch's shape); others pass false.
func errorMessages(resBody []byte, withField bool) []string {
	var errorResponse ErrorResponse
	if err := json.Unmarshal(resBody, &errorResponse); err != nil {
		return nil
	}
	if len(errorResponse.Errors) == 0 {
		return nil
	}
	messages := make([]string, 0, len(errorResponse.Errors))
	for _, e := range errorResponse.Errors {
		if withField && e.Field != "" {
			messages = append(messages, fmt.Sprintf("%s: %s", e.Field, e.Message))
		} else {
			messages = append(messages, e.Message)
		}
	}
	return messages
}

func handleResponseError(res *http.Response, credential *credentials.AuraCredential, cfg *clicfg.Config) error {
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return clierr.NewUpstreamError("%s", upstreamDetail(res.StatusCode, nil))
	}

	switch statusCode := res.StatusCode; statusCode {
	// redirection messages
	case http.StatusPermanentRedirect:
		return clierr.NewUpstreamError("%s", upstreamDetail(statusCode, resBody))
	// client error responses
	case http.StatusBadRequest:
		if msgs := errorMessages(resBody, true); msgs != nil {
			return clierr.NewValidationError("%s", formatBracketedMessages(msgs)).WithSuggestion("See 'neo4j-cli aura <cmd> --help' for valid flags and values.")
		}
		return clierr.NewValidationError("%s", upstreamDetail(statusCode, resBody))
	case http.StatusUnauthorized:
		return formatAuthorizationError(resBody, statusCode, credential, cfg)
	case http.StatusForbidden:
		var serverError ServerError
		if err := json.Unmarshal(resBody, &serverError); err == nil && serverError.Error != "" {
			return clierr.NewAuthError("%s", serverError.Error).WithSuggestion(authSuggestion)
		}
		return formatAuthorizationError(resBody, statusCode, credential, cfg)
	case http.StatusNotFound:
		msgs := errorMessages(resBody, false)
		resourceType, resourceID := parseResourceFromRequest(res.Request)
		if msgs != nil {
			return clierr.NewNotFoundError("%s", formatBracketedMessages(msgs)).WithResource(resourceType, resourceID).WithSuggestion(suggestionForResource(resourceType))
		}
		return clierr.NewNotFoundError("%s", upstreamDetail(statusCode, resBody)).WithResource(resourceType, resourceID).WithSuggestion(suggestionForResource(resourceType))
	case http.StatusMethodNotAllowed:
		if msgs := errorMessages(resBody, false); msgs != nil {
			return clierr.NewUpstreamError("%s", formatBracketedMessages(msgs))
		}
		return clierr.NewUpstreamError("%s", upstreamDetail(statusCode, resBody))
	case http.StatusPaymentRequired:
		if msgs := errorMessages(resBody, false); msgs != nil {
			var eResp ErrorResponse
			_ = json.Unmarshal(resBody, &eResp) // safe: errorMessages confirmed parseable
			ce := clierr.NewConflictError("%s", formatBracketedMessages(msgs))
			if s := suggestionForPaymentRequired(eResp); s != "" {
				ce = ce.WithSuggestion(s)
			}
			return ce
		}
		return clierr.NewConflictError("%s", upstreamDetail(statusCode, resBody))
	case http.StatusConflict:
		if msgs := errorMessages(resBody, false); msgs != nil {
			return clierr.NewConflictError("%s", formatBracketedMessages(msgs))
		}
		return clierr.NewConflictError("%s", upstreamDetail(statusCode, resBody))
	// 413, 415, 422 are documented on POST .../graph-analytics/sessions,
	// POST .../instances/{instance_id}/databases, and
	// POST .../instances/{instance_id}/databases/{database_id}/restore.
	// GDSError and InvokeAgentError appear only on 2xx responses; BillingErrorResponse
	// covers two billing endpoints the CLI has no commands for —
	// the raw-body fallback is the primary path.
	case http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		if msgs := errorMessages(resBody, false); msgs != nil {
			return clierr.NewValidationError("%s", formatBracketedMessages(msgs))
		}
		return clierr.NewValidationError("%s", upstreamDetail(statusCode, resBody))
	case http.StatusTooManyRequests:
		retryAfter := res.Header.Get("Retry-After")
		return clierr.NewRateLimitError(retryAfter, "server rate limit exceeded, suggested cool-off period is %s seconds before rerunning the command", retryAfter).WithSuggestion(fmt.Sprintf("Retry after %s seconds.", retryAfter))
	// server error responses
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		if msgs := errorMessages(resBody, false); msgs != nil {
			return clierr.NewUpstreamError("%s", formatBracketedMessages(msgs))
		}
		return clierr.NewUpstreamError("%s", upstreamDetail(statusCode, resBody))
	default:
		if statusCode >= 400 && statusCode < 500 {
			// Unmapped client errors (408, 425, …): treated as permanent, so an
			// agent harness must not read them as retryable and loop. The class is
			// the only available signal here, so transient 4xx the spec never
			// documents are reported as permanent too.
			return clierr.NewValidationError("%s", upstreamDetail(statusCode, resBody))
		}
		// 5xx plus every other unmapped status (3xx, …).
		return clierr.NewUpstreamError("%s", upstreamDetail(statusCode, resBody))
	}
}

// nonIDActionSuffixes are trailing path segments that name an action rather
// than a resource id (e.g. `.../instances/{id}/pause`). When the last segment
// is one of these, parseResourceFromRequest treats the preceding
// `<plural>/<id>` pair as the resource so the action is not mis-reported as an
// id.
var nonIDActionSuffixes = map[string]struct{}{
	"pause":     {},
	"resume":    {},
	"overwrite": {},
	"metrics":   {},
	"invoke":    {},
	// `.../virtual-graphs/allowed-configs` lists the selectable memory
	// configurations for the collection, so the trailing segment is a
	// sub-collection rather than a virtual graph id.
	"allowed-configs": {},
}

// parseResourceFromRequest extracts a (resourceType, resourceID) pair from
// the request URL path so the JSON error envelope can surface them on a 404.
// Flat v1/v1beta5 paths follow `/<version>/<plural-resource>/<id>[/...]` (e.g.
// `/v1/instances/abc123` or `/v1beta5/tenants/abc123/metrics-integration`) and
// resolve to the first `<plural>/<id>` pair. Nested v2beta1 paths scope every
// resource under `/<version>/organizations/{org}/projects/{proj}/<plural>/<id>`;
// for these the scoping prefix is skipped and the *trailing* `<plural>/<id>`
// pair is returned so 404 envelopes carry the real resource (e.g. `instance`),
// not `organization`. Returns ("", "") when the request, URL, or path doesn't
// fit either shape so the envelope omitempty drops both fields.
func parseResourceFromRequest(req *http.Request) (string, string) {
	if req == nil || req.URL == nil {
		return "", ""
	}
	segments := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	// Need at least <version> / <plural> / <id> — three segments.
	if len(segments) < 3 {
		return "", ""
	}

	// Nested v2beta1 shape: skip the org/project scoping prefix and parse the
	// remaining resource segments so the trailing resource wins.
	if len(segments) >= 6 && segments[1] == "organizations" && segments[3] == "projects" {
		return parseTrailingResource(segments[5:])
	}

	// Flat shape: segments[1] is the plural resource, segments[2] its id.
	return singularise(segments[1]), segments[2]
}

// parseTrailingResource resolves the trailing `<plural>/<id>` pair from the
// resource segments that follow the v2beta1 scoping prefix. A trailing action
// segment (see nonIDActionSuffixes) is dropped so it is not mistaken for an id.
// Returns ("", "") when there is no `<plural>/<id>` pair (e.g. a bare list path
// with only a plural segment) so the envelope omitempty drops both fields.
func parseTrailingResource(segments []string) (string, string) {
	if len(segments) > 0 {
		if _, isAction := nonIDActionSuffixes[segments[len(segments)-1]]; isAction {
			segments = segments[:len(segments)-1]
		}
	}
	if len(segments) < 2 {
		return "", ""
	}
	return singularise(segments[len(segments)-2]), segments[len(segments)-1]
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
	case "session":
		return "Run 'neo4j-cli aura graph-analytics session list --project-id <id>' to see sessions in this project."
	case "virtual-graph":
		return "Run 'neo4j-cli aura virtual-graph list --project-id <id>' to see virtual graphs in this project."
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

// Virtual graph lifecycle states, in the lowercase form the v2beta1 API
// returns. Casing is not guaranteed, so comparisons against these constants are
// case-insensitive (see PollVirtualGraph).
const (
	VirtualGraphStatusCreating string = "creating"
	VirtualGraphStatusRunning  string = "running"
	VirtualGraphStatusUpdating string = "updating"
	VirtualGraphStatusError    string = "error"
	VirtualGraphStatusDeleted  string = "deleted"
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
