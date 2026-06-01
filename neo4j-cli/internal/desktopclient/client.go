// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/neo4j/cli/common/clierr"
)

// Header names required by Desktop's relate API auth middleware on every
// authenticated call.
const (
	HeaderClientID = "X-Client-Id"
	HeaderAPIToken = "X-API-Token"
)

// 90s ceiling on every Desktop HTTP request — slow machines (Windows HDD,
// congested network) routinely overrun 30s on otherwise-healthy `GET /dbmss`,
// surfacing a misleading "Desktop not running" error.
const requestTimeout = 90 * time.Second

// Extended budget for plugin install/uninstall: relate copies the JAR and
// rewrites neo4j.conf during these POSTs.
const pluginWriteTimeout = 120 * time.Second

// Extended budget for `POST /desktop/dbmss`: Desktop does version download +
// unpack + install + initial-password setup; minutes are normal on slow nets.
const dbmsCreateTimeout = 10 * time.Minute

// Extended budget for `POST /dbmss/:id/upgrade`: Desktop downloads + unpacks
// the new version, runs config upgrade, optional data migration, and plugin
// handling before the POST resolves — well beyond create on a large store.
const dbmsUpgradeTimeout = 30 * time.Minute

// HS256 JWT exp window; matches Desktop's default.
const tokenLifetime = 7 * 24 * time.Hour

const errBodyTruncateAt = 200

// Defence-in-depth cap on the response body. A rogue local process that wins
// the 44222..44232 probe race can serve any HTTP 200 to pass the probe; without
// a cap a malicious server can exhaust CLI memory before the JWT-signed call
// fails. 4 MiB is well above any legitimate relate response shape.
const maxResponseBodyBytes = 4 << 20

// User-visible message for the "Desktop isn't there" case (probe miss,
// connection-refused, EOF, reset). NOT used for deadline-exceeded so a slow
// machine doesn't get told Desktop is missing when it's actually busy.
const canonicalUnreachable = "Neo4j Desktop 2 doesn't appear to be running (tried mDNS discovery, then the 44222..44232 port scan). Start Neo4j Desktop 2 from your applications menu; if it's running on a non-default port, pass --port; on macOS, the Local Network permission may block mDNS, so pass --port or run 'neo4j-cli desktop doctor' to scan."

// Shown when the request hit the CLI-side deadline. Distinguishes "took too
// long" from "isn't there". `%s` receives the elapsed budget.
const canonicalTimeoutFmt = "Neo4j Desktop 2 did not respond within %s. The operation may still be running in Desktop — check Desktop's UI before retrying. If Desktop is on a non-default port, pass --port."

const canonicalAuthFailed = "Auth failed against Neo4j Desktop 2 local API. The stored token state may be stale or out of sync — restart Neo4j Desktop 2 to regenerate."

// Plugin-endpoint 404 sentinels. When the 404 body is ambiguous, plugin
// methods default to `ErrPluginNotFound` (plugin operations are scoped under a
// DBMS, so a 404 almost always means the plugin name failed to resolve).
var (
	ErrDbmsNotFound   = errors.New("desktop: dbms not found")
	ErrPluginNotFound = errors.New("desktop: plugin not found")
)

var httpDoFn = func(req *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

func SetHTTPDoFnForTest(fn func(*http.Request) (*http.Response, error)) func() {
	prev := httpDoFn
	httpDoFn = fn
	return func() { httpDoFn = prev }
}

var uuidNewFn = func() string { return uuid.NewString() }

func SetUUIDFnForTest(fn func() string) func() {
	prev := uuidNewFn
	uuidNewFn = fn
	return func() { uuidNewFn = prev }
}

var nowFn = func() time.Time { return time.Now() }

func SetNowFnForTest(fn func() time.Time) func() {
	prev := nowFn
	nowFn = fn
	return func() { nowFn = prev }
}

// Client is the authenticated HTTP client for Desktop's local relate API. One
// Client per CLI invocation — the X-Client-Id is fixed at construction and
// folded into the JWT signing key so all calls from this process share one
// token.
type Client struct {
	origin   string // "http://localhost:<port>" (legacy port-scan / older Desktop) or "http://127.0.0.1:<port>" (mDNS-discovered / newer Desktop); folded into the JWT signing key, so it must match Desktop's environment.httpOrigin
	salt     string // contents of <dataDir>/relate.secret.key
	clientID string // fresh UUID v4 per Client; sent as X-Client-Id
	token    string // signed once at construction; reused on every call
}

// UnreachableError returns the canonical "Desktop is not running" error.
func UnreachableError() error {
	return clierr.NewFatalError("%s", canonicalUnreachable)
}

// NewClient builds an authenticated Client for the given Desktop instance.
// The signing key composition `<salt>-<origin>-<clientId>` mirrors Desktop's
// own derivation so generated tokens validate against the running instance.
func NewClient(probe ProbeResult, salt string) (*Client, error) {
	clientID := uuidNewFn()
	token, err := signToken(salt, probe.Origin, clientID)
	if err != nil {
		return nil, clierr.NewFatalError("desktop: failed to sign auth token: %s", err.Error())
	}
	return &Client{
		origin:   probe.Origin,
		salt:     salt,
		clientID: clientID,
		token:    token,
	}, nil
}

func signToken(salt, origin, clientID string) (string, error) {
	now := nowFn()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(tokenLifetime).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	key := fmt.Sprintf("%s-%s-%s", salt, origin, clientID)
	return tok.SignedString([]byte(key))
}

func (c *Client) ClientID() string { return c.clientID }

// do executes an authenticated request with the default `requestTimeout`.
// Plugin endpoints route through `doPlugin` for 404 disambiguation.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	return c.doWithTimeout(ctx, method, path, body, requestTimeout)
}

func (c *Client) doWithTimeout(ctx context.Context, method, path string, body any, timeout time.Duration) ([]byte, error) {
	status, respBody, err := c.doRaw(ctx, method, path, body, timeout)
	if err != nil {
		return nil, err
	}
	switch {
	case status >= 200 && status <= 299:
		return respBody, nil
	case status == http.StatusUnauthorized:
		return nil, clierr.NewAuthError("%s", canonicalAuthFailed)
	case status >= 500:
		return nil, clierr.NewFatalError(
			"Neo4j Desktop 2 local API returned %d. The response body was: %s. Try restarting Desktop; if the error persists, file a bug.",
			status, truncateBody(respBody))
	default:
		return nil, clierr.NewFatalError(
			"Neo4j Desktop 2 local API returned %d: %s", status, truncateBody(respBody))
	}
}

// doRaw issues an authenticated request, applies the per-call timeout, and
// returns the raw `(status, body)` pair on a completed transport round-trip.
// Transport-level failures (probe miss, connection-refused, mid-request EOF,
// deadline exceeded) collapse to the canonical unreachable error here.
// Status-code mapping is left to the caller so plugin methods can disambiguate
// 404s.
func (c *Client) doRaw(ctx context.Context, method, path string, body any, timeout time.Duration) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, nil, clierr.NewFatalError("desktop: failed to encode request body: %s", err.Error())
		}
		reqBody = bytes.NewReader(buf)
	}

	url := c.origin + "/fastify/api" + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return 0, nil, clierr.NewFatalError("desktop: invalid request URL %q: %s", url, err.Error())
	}
	req.Header.Set(HeaderClientID, c.clientID)
	req.Header.Set(HeaderAPIToken, c.token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpDoFn(req)
	if err != nil {
		// Disambiguate "took too long" (Desktop is there, just busy) from
		// "Desktop isn't there" (probe miss, connection refused, EOF, reset).
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return 0, nil, clierr.NewFatalError(canonicalTimeoutFmt, timeout)
		}
		return 0, nil, clierr.NewFatalError("%s", canonicalUnreachable)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close error is not actionable in a defer

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if readErr != nil {
		// Mid-read EOF means Desktop dropped the socket — treat as unreachable.
		return 0, nil, clierr.NewFatalError("%s", canonicalUnreachable)
	}
	return resp.StatusCode, respBody, nil
}

func truncateBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > errBodyTruncateAt {
		return s[:errBodyTruncateAt] + "…"
	}
	return s
}

// ListDbmss returns the lightweight `GET /dbmss` shape (no `status` field —
// see ListDbmssInfo).
func (c *Client) ListDbmss(ctx context.Context) ([]DbmsInfo, error) {
	body, err := c.do(ctx, http.MethodGet, "/dbmss", nil)
	if err != nil {
		return nil, err
	}
	var out []DbmsInfo
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode dbms list: %s", err.Error())
	}
	return out, nil
}

// ListDbmssInfo returns the full-shape DBMS list including `status` /
// `serverStatus`. `GET /dbmss` returns the lightweight shape without those
// fields.
func (c *Client) ListDbmssInfo(ctx context.Context) ([]DbmsInfo, error) {
	body, err := c.do(ctx, http.MethodGet, "/dbmss/info", nil)
	if err != nil {
		return nil, err
	}
	var out []DbmsInfo
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode dbms info list: %s", err.Error())
	}
	return out, nil
}

func (c *Client) GetDbms(ctx context.Context, id string) (*DbmsInfo, error) {
	body, err := c.do(ctx, http.MethodGet, "/dbmss/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	var out DbmsInfo
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode dbms: %s", err.Error())
	}
	return &out, nil
}

// CreateDbmsRequest is the input to CreateDbms. No `edition` — Desktop 2
// ships enterprise-only.
type CreateDbmsRequest struct {
	Name     string
	Version  string
	Password string
}

// CreateDbms posts a new DBMS to Desktop. We call the Desktop-override route
// `/desktop/dbmss` (rather than relate's plain `/dbmss`) because the override
// invokes `installDbms()` which calls `credentialsManager.setCredentials` after
// install — same path the GUI takes. The plain `/dbmss` route only installs
// and never persists the credential, so a subsequent
// `GET /credentials/dbms:<id>` returns `null` (Mac) or 500 (Windows).
func (c *Client) CreateDbms(ctx context.Context, req CreateDbmsRequest) (*DbmsInfo, error) {
	payload := map[string]any{
		"name":        req.Name,
		"version":     req.Version,
		"credentials": req.Password,
	}
	body, err := c.doWithTimeout(ctx, http.MethodPost, "/desktop/dbmss", payload, dbmsCreateTimeout)
	if err != nil {
		return nil, err
	}
	var out DbmsInfo
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode created dbms: %s", err.Error())
	}
	return &out, nil
}

// UpgradeDbms posts `POST /dbmss/:id/upgrade` to upgrade a stopped DBMS to
// `version`. Only the `options` keys actually set by the caller are sent —
// `options` is omitted entirely when empty — so Desktop's server-side defaults
// (`migrate=true`, `pluginUpgradeMode=UPGRADABLE`) apply for unset keys. The
// POST resolves after the upgrade completes and returns the upgraded DbmsInfo
// (left stopped). Uses the extended 30-minute timeout.
func (c *Client) UpgradeDbms(ctx context.Context, id, version string, opts UpgradeDbmsOptions) (*DbmsInfo, error) {
	payload := map[string]any{"version": version}
	options := map[string]any{}
	if opts.Backup != nil {
		options["backup"] = *opts.Backup
	}
	if opts.Migrate != nil {
		options["migrate"] = *opts.Migrate
	}
	if opts.PluginUpgradeMode != "" {
		options["pluginUpgradeMode"] = opts.PluginUpgradeMode
	}
	if len(options) > 0 {
		payload["options"] = options
	}
	body, err := c.doWithTimeout(ctx, http.MethodPost, "/dbmss/"+url.PathEscape(id)+"/upgrade", payload, dbmsUpgradeTimeout)
	if err != nil {
		return nil, err
	}
	var out DbmsInfo
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode upgraded dbms: %s", err.Error())
	}
	return &out, nil
}

// DeleteDbms removes one DBMS by ID. Desktop owns the credential lifecycle —
// neo4j-cli's credentials store is NOT touched here.
func (c *Client) DeleteDbms(ctx context.Context, id string) (*DbmsInfo, error) {
	body, err := c.do(ctx, http.MethodDelete, "/dbmss/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	var out DbmsInfo
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode deleted dbms: %s", err.Error())
	}
	return &out, nil
}

// StartDbms issues `POST /dbmss/:id/start`. Desktop's body is a stringified
// shell output, discarded here — callers poll GetDbms for `status=started`.
func (c *Client) StartDbms(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodPost, "/dbmss/"+url.PathEscape(id)+"/start", nil)
	return err
}

// StopDbms calls the Desktop-override stop route (rather than relate's plain
// `/dbmss/:id/stop`) so Windows users get a graceful shutdown: the override
// uses the stored credentials to invoke a Neo4j shutdown procedure, since
// non-admin Windows users can't signal the JVM directly. On macOS / Linux the
// override falls through to the same path.
func (c *Client) StopDbms(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodPost, "/desktop/dbmss/"+url.PathEscape(id)+"/stop", nil)
	return err
}

// ListDbmsVersions returns the full version catalog Desktop knows about
// (both cached and online entries).
func (c *Client) ListDbmsVersions(ctx context.Context) ([]DbmsVersion, error) {
	body, err := c.do(ctx, http.MethodGet, "/dbmss/versions", nil)
	if err != nil {
		return nil, err
	}
	var out []DbmsVersion
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode dbms versions: %s", err.Error())
	}
	return out, nil
}

// GetCredentialsByKey looks up the stored username/password for a Desktop
// credential namespace key (e.g. `dbms:<dbmsId>`, `connection:<connectionId>`).
// Returns `(nil, nil)` when Desktop emits the JSON literal `null` — legacy
// DBMSes that pre-date `storePasswords` or `safeStorage` unavailable — so
// callers treat this as a soft miss and fall through to prompt / persisted
// paths rather than surfacing an error.
func (c *Client) GetCredentialsByKey(ctx context.Context, key string) (*Credentials, error) {
	// url.PathEscape preserves `:` (the namespace separator) but escapes `/`,
	// `?`, `#`, etc., so a caller-supplied id can't punch through to a
	// different route.
	body, err := c.do(ctx, http.MethodGet, "/credentials/"+url.PathEscape(key), nil)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, nil
	}
	var out Credentials
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode credentials: %s", err.Error())
	}
	return &out, nil
}

// ListConnections returns every saved remote DB connection profile.
func (c *Client) ListConnections(ctx context.Context) ([]Connection, error) {
	body, err := c.do(ctx, http.MethodGet, "/connections", nil)
	if err != nil {
		return nil, err
	}
	var out []Connection
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode connection list: %s", err.Error())
	}
	return out, nil
}

// CreateConnection posts a new saved connection. Desktop stores the password
// via safeStorage behind the `connection:<id>` key.
func (c *Client) CreateConnection(ctx context.Context, args ConnectionCreateArgs) (*Connection, error) {
	payload := map[string]any{
		"name":          args.Name,
		"connectionUri": args.ConnectionURI,
		"username":      args.Username,
		"password":      args.Password,
	}
	if args.Description != "" {
		payload["description"] = args.Description
	}
	body, err := c.do(ctx, http.MethodPost, "/connections", payload)
	if err != nil {
		return nil, err
	}
	var out Connection
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode created connection: %s", err.Error())
	}
	return &out, nil
}

// UpdateConnection PATCHes only the populated keys — empty string is an
// intentional update (e.g. clearing a description), not "omit this field".
// `nil` pointer fields are dropped from the body.
func (c *Client) UpdateConnection(ctx context.Context, id string, args ConnectionUpdateArgs) (*Connection, error) {
	payload := map[string]any{}
	if args.Name != nil {
		payload["name"] = *args.Name
	}
	if args.ConnectionURI != nil {
		payload["connectionUri"] = *args.ConnectionURI
	}
	if args.Username != nil {
		payload["username"] = *args.Username
	}
	if args.Password != nil {
		payload["password"] = *args.Password
	}
	if args.Description != nil {
		payload["description"] = *args.Description
	}
	body, err := c.do(ctx, http.MethodPatch, "/connections/"+url.PathEscape(id), payload)
	if err != nil {
		return nil, err
	}
	var out Connection
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode updated connection: %s", err.Error())
	}
	return &out, nil
}

func (c *Client) DeleteConnection(ctx context.Context, id string) (*Connection, error) {
	body, err := c.do(ctx, http.MethodDelete, "/connections/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	var out Connection
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode deleted connection: %s", err.Error())
	}
	return &out, nil
}

// pluginRequest is the JSON body relate accepts for install/uninstall. The
// value is forwarded verbatim; relate decides server-side whether to treat it
// as a name from the `available` catalog or as a filesystem path to a local
// `.jar`.
type pluginRequest struct {
	PluginName string `json:"pluginName"`
}

type pluginUninstallResponse struct {
	Name string `json:"name"`
}

// classifyPluginNotFound inspects a 404 response body to decide between
// `ErrDbmsNotFound` and `ErrPluginNotFound`. Relate's `NotFoundError`
// surfaces a JSON body of the form `{"message":"Could not find DBMS \"X\""}`
// or `{"message":"Could not find plugin \"Y\""}`; we string-match the
// distinguishing token. When the body shape is unexpected (e.g. plain text),
// we default to `ErrPluginNotFound` because plugin endpoints are scoped
// under a DBMS — a 404 there almost always means the plugin name failed to
// resolve.
func classifyPluginNotFound(body []byte) error {
	// Cheap substring check rather than full JSON decode: relate's
	// NotFoundError shape is stable, and parsing on the error path adds no
	// value when the surrounding switch already routed us here.
	msg := strings.ToLower(string(body))
	if strings.Contains(msg, "dbms") {
		return ErrDbmsNotFound
	}
	if strings.Contains(msg, "plugin") {
		return ErrPluginNotFound
	}
	return ErrPluginNotFound
}

// doPlugin is the plugin-aware sibling of `do`: same status-code mapping for
// 2xx / 401 / 5xx / 4xx-other, but 404 disambiguates into the sentinels above.
func (c *Client) doPlugin(ctx context.Context, method, path string, body any, timeout time.Duration) ([]byte, error) {
	status, respBody, err := c.doRaw(ctx, method, path, body, timeout)
	if err != nil {
		return nil, err
	}
	switch {
	case status >= 200 && status <= 299:
		return respBody, nil
	case status == http.StatusUnauthorized:
		return nil, clierr.NewAuthError("%s", canonicalAuthFailed)
	case status == http.StatusNotFound:
		return nil, classifyPluginNotFound(respBody)
	case status >= 500:
		return nil, clierr.NewFatalError(
			"Neo4j Desktop 2 local API returned %d. The response body was: %s. Try restarting Desktop; if the error persists, file a bug.",
			status, truncateBody(respBody))
	default:
		return nil, clierr.NewFatalError(
			"Neo4j Desktop 2 local API returned %d: %s", status, truncateBody(respBody))
	}
}

// ListInstalledPlugins returns the installed-plugin list for one DBMS. An
// empty result is returned as `[]DbmsPlugin{}` (not `nil`) so JSON renderers
// emit `[]`.
func (c *Client) ListInstalledPlugins(ctx context.Context, dbmsID string) ([]DbmsPlugin, error) {
	body, err := c.doPlugin(ctx, http.MethodGet, "/dbmss/"+url.PathEscape(dbmsID)+"/plugins/installed", nil, requestTimeout)
	if err != nil {
		return nil, err
	}
	out := []DbmsPlugin{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode installed plugin list: %s", err.Error())
	}
	return out, nil
}

// ListAvailablePlugins returns the installable-plugin catalog for one DBMS.
// Catalog source is relate's scan of the DBMS's `products/` + `labs/`
// directories. Empty result is returned as `[]DbmsPlugin{}` (not `nil`).
func (c *Client) ListAvailablePlugins(ctx context.Context, dbmsID string) ([]DbmsPlugin, error) {
	body, err := c.doPlugin(ctx, http.MethodGet, "/dbmss/"+url.PathEscape(dbmsID)+"/plugins/available", nil, requestTimeout)
	if err != nil {
		return nil, err
	}
	out := []DbmsPlugin{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode available plugin list: %s", err.Error())
	}
	return out, nil
}

// InstallPlugin installs `pluginName` on the given DBMS. `pluginName` is
// forwarded verbatim — relate dispatches name-vs-path server-side. Uses the
// extended 120s timeout since relate's install copies a JAR and rewrites
// neo4j.conf.
func (c *Client) InstallPlugin(ctx context.Context, dbmsID, pluginName string) (*DbmsPlugin, error) {
	body, err := c.doPlugin(ctx, http.MethodPost, "/dbmss/"+url.PathEscape(dbmsID)+"/plugins/install",
		pluginRequest{PluginName: pluginName}, pluginWriteTimeout)
	if err != nil {
		return nil, err
	}
	var out DbmsPlugin
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode installed plugin: %s", err.Error())
	}
	return &out, nil
}

// UninstallPlugin removes `pluginName` from the given DBMS. Idempotent on the
// relate side — uninstalling an already-removed plugin still returns 200 with
// the name. Uses the extended 120s timeout.
func (c *Client) UninstallPlugin(ctx context.Context, dbmsID, pluginName string) (string, error) {
	body, err := c.doPlugin(ctx, http.MethodPost, "/dbmss/"+url.PathEscape(dbmsID)+"/plugins/uninstall",
		pluginRequest{PluginName: pluginName}, pluginWriteTimeout)
	if err != nil {
		return "", err
	}
	var out pluginUninstallResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", clierr.NewFatalError("desktop: failed to decode uninstalled plugin: %s", err.Error())
	}
	return out.Name, nil
}
