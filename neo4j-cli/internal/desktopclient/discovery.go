// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/afero"

	"github.com/neo4j/cli/common/clierr"
)

const (
	// ProbePortStart is the first port Desktop's relate server tries —
	// `detectPort(44222)` in `neo4j-desktop-2`. It walks up on conflict, so
	// we scan a fixed range of 11 ports.
	ProbePortStart = 44222
	ProbePortEnd   = 44232
	// ProbePath is the relate API docs endpoint that responds 200 on a live
	// instance. A raw TCP-open alone does NOT count — Desktop's port may be
	// in use by an unrelated service, so we require a 200 on this path.
	ProbePath = "/fastify/api-docs"
	// SaltFilename is the on-disk name of the relate auth salt. Written by
	// Desktop on first auth as a UUID v4 literal.
	SaltFilename = "relate.secret.key"

	// probeTimeoutPerPort caps each individual HTTP probe so the total
	// worst-case walk stays under the 2s budget (11 ports * 200ms).
	probeTimeoutPerPort = 200 * time.Millisecond
)

var homeDirFn = os.UserHomeDir

func SetHomeDirFnForTest(fn func() (string, error)) func() {
	prev := homeDirFn
	homeDirFn = fn
	return func() { homeDirFn = prev }
}

var goosFn = func() string { return runtime.GOOS }

func SetGOOSFnForTest(fn func() string) func() {
	prev := goosFn
	goosFn = fn
	return func() { goosFn = prev }
}

// ProbeResult is the outcome of a successful port probe — the chosen port +
// the canonical origin string the auth layer needs (matches Desktop's own
// auth-token key derivation).
type ProbeResult struct {
	Port int
	// Origin is "http://localhost:<port>" for port-scan / old-Desktop results
	// or "http://127.0.0.1:<port>" for mDNS / DNS-SD / new-Desktop results. The
	// host half is auth-coupled: it is folded verbatim into the JWT signing key
	// (signToken) so it MUST match the host Desktop signs with.
	Origin string
}

// AppInfo is the unauthenticated `GET /fastify/api/info/app` reply shape.
// Unknown future fields are ignored by the default JSON decoder.
type AppInfo struct {
	Platform   string `json:"platform"`
	Version    string `json:"version"`
	AppPath    string `json:"appPath"`
	LogsPath   string `json:"logsPath"`
	DataPath   string `json:"dataPath"`
	CachePath  string `json:"cachePath"`
	ConfigPath string `json:"configPath"`
}

const infoAppPath = "/fastify/api/info/app"

// FetchAppInfo issues an UNAUTHENTICATED `GET <probe.Origin>/fastify/api/info/app`
// and JSON-decodes the reply into an AppInfo. No `X-Client-Id` / `X-API-Token`
// headers are sent — the route is exempt from Desktop's auth middleware so a
// fresh CLI process (which can't yet sign a JWT, the salt lives in the dir
// we're discovering) can bootstrap. Non-2xx responses surface as fatal errors
// that the discovery-fallback ladder treats as "fall through to env-JSON / OS
// default" without reaching the user.
func FetchAppInfo(ctx context.Context, probe ProbeResult) (AppInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	url := probe.Origin + infoAppPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return AppInfo{}, clierr.NewFatalError("desktop: invalid request URL %q: %s", url, err.Error())
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpDoFn(req)
	if err != nil {
		// Same disambiguation as Client.doRaw — deadline-exceeded means
		// "Desktop is there but slow", everything else means "Desktop isn't
		// there" (probe miss, connection refused, EOF, reset).
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return AppInfo{}, clierr.NewFatalError(canonicalTimeoutFmt, requestTimeout)
		}
		return AppInfo{}, clierr.NewFatalError("%s", canonicalUnreachable)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close error is not actionable in a defer

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if readErr != nil {
		return AppInfo{}, clierr.NewFatalError("%s", canonicalUnreachable)
	}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode <= 299:
		// fallthrough to decode below
	case resp.StatusCode == http.StatusUnauthorized:
		// Older Desktop builds without the route-level auth exemption answer
		// 401 here. Surface as a typed auth error so the caller can fall
		// through to env-JSON / OS default.
		return AppInfo{}, clierr.NewAuthError("%s", canonicalAuthFailed)
	case resp.StatusCode >= 500:
		return AppInfo{}, clierr.NewFatalError(
			"Neo4j Desktop 2 local API returned %d. The response body was: %s. Try restarting Desktop; if the error persists, file a bug.",
			resp.StatusCode, truncateBody(respBody))
	default:
		return AppInfo{}, clierr.NewFatalError(
			"Neo4j Desktop 2 local API returned %d: %s", resp.StatusCode, truncateBody(respBody))
	}

	var out AppInfo
	if err := json.Unmarshal(respBody, &out); err != nil {
		return AppInfo{}, clierr.NewFatalError("desktop: failed to decode /info/app: %s", err.Error())
	}
	return out, nil
}

// httpClientFn returns a fresh *http.Client per probe call so timeout overrides
// don't leak across calls.
var httpClientFn = func() *http.Client {
	return &http.Client{
		Timeout: probeTimeoutPerPort,
	}
}

func SetHTTPClientFnForTest(fn func() *http.Client) func() {
	prev := httpClientFn
	httpClientFn = fn
	return func() { httpClientFn = prev }
}

var probeHostFn = func() string { return "localhost" }

func SetProbeHostFnForTest(fn func() string) func() {
	prev := probeHostFn
	probeHostFn = fn
	return func() { probeHostFn = prev }
}

// e2ePortOverride is read at the top of ProbePort; when non-zero it
// short-circuits the production scan and returns the override port verbatim
// WITHOUT calling /fastify/api-docs. Only seams_e2e.go (build tag
// `e2e_desktop_seams`) assigns to it.
var e2ePortOverride int

// e2eMDNSPortOverride is read at the top of DiscoverViaMDNS; when non-zero it
// short-circuits the mDNS browse and returns the override port verbatim with a
// 127.0.0.1 origin. Only seams_e2e.go (build tag `e2e_desktop_seams`) assigns
// to it.
var e2eMDNSPortOverride int

// ProbePort scans the 44222..44232 range looking for a relate server that
// responds 200 on /fastify/api-docs. Returns the first match. When `pinned`
// is non-zero, ProbePort tries only that port — supports the `--port`
// override on the desktop command tree.
func ProbePort(ctx context.Context, pinned int) (ProbeResult, error) {
	if e2ePortOverride != 0 {
		// e2e short-circuit: skip the scan AND the /api-docs validation.
		return ProbeResult{Port: e2ePortOverride, Origin: origin(e2ePortOverride)}, nil
	}
	if pinned != 0 {
		if ok := probeOne(ctx, pinned); ok {
			return ProbeResult{Port: pinned, Origin: origin(pinned)}, nil
		}
		return ProbeResult{}, ErrNoDesktop
	}

	for port := ProbePortStart; port <= ProbePortEnd; port++ {
		if err := ctx.Err(); err != nil {
			return ProbeResult{}, err
		}
		if ok := probeOne(ctx, port); ok {
			return ProbeResult{Port: port, Origin: origin(port)}, nil
		}
	}
	return ProbeResult{}, ErrNoDesktop
}

// ErrNoDesktop is returned by ProbePort when no port in the scanned range
// answers 200 on /fastify/api-docs. Callers map this to the canonical
// "Desktop unreachable" error text.
var ErrNoDesktop = errors.New("desktop: no relate server found on ports 44222..44232")

// probeOne sends a single GET to `<origin>/fastify/api-docs` and reports
// whether the response is an HTTP 200. The probe is a presence-test, not
// an identity-proof: any unprivileged local process can bind a loopback
// port in 44222..44232 and answer 200 trivially.
//
// Residual risk (inherited from Desktop's local-only protocol): when a
// rogue local process wins the bind race against Desktop, the CLI's
// authenticated calls (CreateDbms, CreateConnection, plugin install,
// etc.) still send their request bodies to the rogue server. The JWT in
// the `X-API-Token` header is HMAC'd with the salt from
// `<dataDir>/relate.secret.key`, so the rogue server CANNOT forge a
// response Desktop would accept — but it CAN read the request body
// (e.g. a fresh `--password`) before the JWT is even checked. The salt
// itself isn't sent over the wire.
//
// Mitigating this fully requires Desktop-side changes (mutual auth /
// signed responses / encrypted bodies) outside this CLI's control. The
// `--password` TTY-prompt on `desktop dbms create` ensures the password
// is at least not in argv, so the exposure window is exactly the
// outbound HTTP request body — same as Desktop's own UI client.
func probeOne(ctx context.Context, port int) bool {
	url := origin(port) + ProbePath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	client := httpClientFn()
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close() //nolint:errcheck // probe response body is unused; close errors are not actionable
	return resp.StatusCode == http.StatusOK
}

// e2eOriginOverride is read at the top of origin; when non-empty it replaces
// the default `http://<probeHost>:<port>` shape used both for the API URL
// prefix in client.do and for the JWT signing key derivation in signToken.
// Only seams_e2e.go (build tag `e2e_desktop_seams`) assigns to it.
var e2eOriginOverride string

func origin(port int) string {
	if e2eOriginOverride != "" {
		return e2eOriginOverride
	}
	return fmt.Sprintf("http://%s:%d", probeHostFn(), port)
}

// mdnsOrigin is the origin for an mDNS / DNS-SD discovered instance. New
// Desktop builds advertise over mDNS and sign with the explicit 127.0.0.1 host
// (NOT the "localhost" alias the legacy port scan uses), so the auth layer must
// derive its JWT key from 127.0.0.1 here or the token won't validate.
func mdnsOrigin(port int) string {
	if e2eOriginOverride != "" {
		return e2eOriginOverride
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// advertisedPort returns the port Desktop advertises over mDNS — in-process
// multicast first, then the macOS dns-sd fallback. (0,false) when none answers.
func advertisedPort(ctx context.Context) (int, bool) {
	if port, ok := mdnsBrowseFn(ctx); ok {
		return port, true
	}
	if goosFn() == "darwin" {
		return dnssdLookupFn(ctx)
	}
	return 0, false
}

// Discover is the high-level entry point that resolves a running Desktop relate
// API. When pinned is non-zero it confirms that exact port; otherwise it tries,
// in order: (1) in-process mDNS multicast, (2) the macOS dns-sd fallback, and
// (3) the legacy 44222..44232 port scan. Every tier fails gracefully and falls
// through; ErrNoDesktop is returned only after the legacy scan also misses.
func Discover(ctx context.Context, pinned int) (ProbeResult, error) {
	if e2ePortOverride != 0 {
		return ProbeResult{Port: e2ePortOverride, Origin: origin(e2ePortOverride)}, nil
	}
	if pinned != 0 {
		return discoverPinned(ctx, pinned)
	}
	if res, err := DiscoverViaMDNS(ctx); err == nil {
		return res, nil
	}
	return ProbePort(ctx, 0)
}

// DiscoverViaMDNS resolves the relate API via mDNS — in-process multicast then
// the macOS dns-sd fallback — yielding a 127.0.0.1 origin. Returns ErrNoDesktop
// when no responder answers.
func DiscoverViaMDNS(ctx context.Context) (ProbeResult, error) {
	if e2eMDNSPortOverride != 0 {
		return ProbeResult{Port: e2eMDNSPortOverride, Origin: mdnsOrigin(e2eMDNSPortOverride)}, nil
	}
	if port, ok := advertisedPort(ctx); ok {
		return ProbeResult{Port: port, Origin: mdnsOrigin(port)}, nil
	}
	return ProbeResult{}, ErrNoDesktop
}

// discoverPinned confirms the caller-pinned port. A new Desktop advertises over
// mDNS, so if an mDNS / dns-sd responder reports the pinned port we use the
// 127.0.0.1 origin; otherwise we fall back to the HTTP port probe, which yields
// the localhost origin an old Desktop signs with.
func discoverPinned(ctx context.Context, pinned int) (ProbeResult, error) {
	if port, ok := advertisedPort(ctx); ok && port == pinned {
		return ProbeResult{Port: pinned, Origin: mdnsOrigin(pinned)}, nil
	}
	return ProbePort(ctx, pinned)
}

// e2eDataDirOverride is read at the top of ResolveDataDir; when non-empty it
// short-circuits the env walk + per-OS default and returns the override path
// verbatim. Only seams_e2e.go (build tag `e2e_desktop_seams`) assigns to it.
var e2eDataDirOverride string

// ResolveDataDir computes the relate data directory in this precedence:
//
//  1. `NEO4J_DESKTOP_DATA_PATH` env var (joined with `Application/Data`).
//  2. `FetchAppInfo(ctx, probe).DataPath` — Desktop's own `/info/app` reply.
//     Any error (401, 5xx, timeout, transport, decode) or empty `DataPath`
//     falls through to step 3 silently. Skipped entirely when `probe.Origin`
//     is empty (caller did not run a port probe).
//  3. The env JSON's `relateDataPath` when an active (or NEO4J_DESKTOP_ENV
//     -named) env exists and ships that field.
//  4. Per-OS Desktop-2 default (last-resort).
//
// fs is consulted for env JSON discovery only; the returned path is NOT
// checked for existence — callers that need the salt file inside it use
// LoadSalt, which does the existence check.
func ResolveDataDir(ctx context.Context, fs afero.Fs, probe ProbeResult) (string, error) {
	if e2eDataDirOverride != "" {
		return e2eDataDirOverride, nil
	}
	if custom := strings.TrimSpace(os.Getenv("NEO4J_DESKTOP_DATA_PATH")); custom != "" {
		return filepath.Join(custom, "Application", "Data"), nil
	}

	// Step 2: ask Desktop directly via the unauthenticated /info/app endpoint.
	// Only attempted when the caller supplied a probe result; otherwise we'd
	// be GETing the empty string. Any error (auth, transport, decode, timeout)
	// and an empty DataPath both fall through to step 3 silently — older
	// Desktop builds without the route-level auth exemption still work via
	// env-JSON / OS default.
	if probe.Origin != "" {
		if info, err := FetchAppInfo(ctx, probe); err == nil && info.DataPath != "" {
			return info.DataPath, nil
		}
	}

	envs, err := LoadEnvs(fs)
	if err == nil {
		nameOverride := strings.TrimSpace(os.Getenv("NEO4J_DESKTOP_ENV"))
		if active := ActiveEnv(envs, nameOverride); active != nil && active.RelateDataPath != "" {
			return active.RelateDataPath, nil
		}
	}

	return defaultDataDir()
}

func defaultDataDir() (string, error) {
	home, err := homeDirFn()
	if err != nil {
		return "", err
	}
	switch goosFn() {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "neo4j-desktop", "Application", "Data"), nil
	case "windows":
		// Desktop-2 special-cases Windows: `applicationPath` is just
		// `<home>\.Neo4jDesktop2` (no `\Application` suffix). See
		// neo4j-desktop-2/packages/electron/src/app-paths.ts:42-52. Then
		// `data` = applicationPath + `\Data`.
		return filepath.Join(home, ".Neo4jDesktop2", "Data"), nil
	default:
		// linux + everything else
		return filepath.Join(home, ".config", "neo4j-desktop", "Application", "Data"), nil
	}
}

// e2eSaltOverride is read at the top of LoadSalt; when non-empty it
// short-circuits the disk read and returns the override value verbatim. Only
// seams_e2e.go (build tag `e2e_desktop_seams`) assigns to it.
var e2eSaltOverride string

// LoadSalt reads `<dataDir>/relate.secret.key` and returns its raw contents
// (a UUID v4 literal Desktop writes on first auth). Used as the key prefix in
// the HS256 JWT signing key.
func LoadSalt(fs afero.Fs, dataDir string) (string, error) {
	if e2eSaltOverride != "" {
		return e2eSaltOverride, nil
	}
	path := filepath.Join(dataDir, SaltFilename)
	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return "", fmt.Errorf("desktop: reading relate salt at %q: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}
