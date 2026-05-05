// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/subosito/gotenv"

	"github.com/neo4j/cli/common/clicfg"
)

const (
	defaultURI      = "http://localhost:7474"
	defaultUsername = "neo4j"
	defaultDatabase = "neo4j"

	envURI      = "NEO4J_URI"
	envUsername = "NEO4J_USERNAME"
	envPassword = "NEO4J_PASSWORD"
	envDatabase = "NEO4J_DATABASE"
	envInsecure = "NEO4J_INSECURE"
)

// httpDoer is the minimal subset of *http.Client needed by runStatement; tests
// inject their own implementation rather than spinning up a server when they
// only want to assert request shape.
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// conn holds the resolved Neo4j connection details plus the HTTP client used to
// reach the server. Tests construct conn directly with a stub doer; production
// code goes through resolveConn.
type conn struct {
	uri       string
	username  string
	password  string
	database  string
	insecure  bool
	userAgent string
	doer      httpDoer
}

// queryResult is the parsed body of a successful POST /query/v2 response.
// Columns are in result order; rows hold the raw values returned by the API
// (positional, matching columns).
type queryResult struct {
	Columns []string
	Rows    [][]any
}

// queryResponse mirrors the JSON envelope returned by the HTTP Query API.
type queryResponse struct {
	Data struct {
		Fields []string `json:"fields"`
		Values [][]any  `json:"values"`
	} `json:"data"`
	Errors []queryError `json:"errors"`
}

type queryError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// resolveConn merges connection settings from .env, OS environment, and
// command-line flags (lowest → highest precedence). Defaults are applied last
// for any value still empty after the merge. The returned conn carries an
// *http.Client honouring --insecure.
func resolveConn(cmd *cobra.Command, cfg *clicfg.Config) (*conn, error) {
	envFlag := flagString(cmd, "env")
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("query: cannot determine current directory: %w", err)
	}

	dotenv, err := loadEnvFile(cfg.Aura.Fs(), envFlag, cwd)
	if err != nil {
		return nil, err
	}

	uri := overlay(dotenv[envURI], os.Getenv(envURI))
	username := overlay(dotenv[envUsername], os.Getenv(envUsername))
	password := overlay(dotenv[envPassword], os.Getenv(envPassword))
	database := overlay(dotenv[envDatabase], os.Getenv(envDatabase))
	insecureStr := overlay(dotenv[envInsecure], os.Getenv(envInsecure))

	if v := flagString(cmd, "uri"); v != "" {
		uri = v
	}
	if v := flagString(cmd, "username"); v != "" {
		username = v
	}
	if v := flagString(cmd, "password"); v != "" {
		password = v
	}
	if v := flagString(cmd, "database"); v != "" {
		database = v
	}

	insecure, _ := parseBool(insecureStr)
	if f := cmd.Flag("insecure"); f != nil && f.Changed {
		if b, perr := strconv.ParseBool(f.Value.String()); perr == nil {
			insecure = b
		}
	}

	if uri == "" {
		uri = defaultURI
	}
	if username == "" {
		username = defaultUsername
	}
	if database == "" {
		database = defaultDatabase
	}

	if rewritten, didRewrite, displayOrig := normalizeURI(uri); didRewrite {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"info: rewrote URI '%s' to '%s' (the query command uses Neo4j's HTTP Query API; pass --uri https://... to silence)\n",
			displayOrig, rewritten)
		uri = rewritten
	}

	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	userAgent := "neo4j-cli/v" + version

	return &conn{
		uri:       uri,
		username:  username,
		password:  password,
		database:  database,
		insecure:  insecure,
		userAgent: userAgent,
		doer:      newHTTPClient(insecure),
	}, nil
}

// loadEnvFile reads a .env file from explicitPath if non-empty, otherwise walks
// up from startDir looking for a .env file in the current dir or any parent.
// Returns an empty (non-nil) map if no file is found and no explicit path was
// requested. An explicit path that does not exist is an error.
func loadEnvFile(fs afero.Fs, explicitPath, startDir string) (map[string]string, error) {
	path := explicitPath
	if path == "" {
		var ok bool
		path, ok = findDotenv(fs, startDir)
		if !ok {
			return map[string]string{}, nil
		}
	}

	f, err := fs.Open(path)
	if err != nil {
		return nil, fmt.Errorf("query: cannot read env file %q: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only close error is not actionable in a defer

	parsed := gotenv.Parse(f)
	out := make(map[string]string, len(parsed))
	for k, v := range parsed {
		out[k] = v
	}
	return out, nil
}

// findDotenv walks up from startDir looking for a `.env` file. Returns the
// absolute path of the first match, or ("", false) if none is found.
func findDotenv(fs afero.Fs, startDir string) (string, bool) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, ".env")
		if exists, _ := afero.Exists(fs, candidate); exists {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// overlay applies values left → right with each non-empty entry overriding the
// earlier accumulator. This implements the documented `.env` < env < flag
// precedence: pass values in increasing-precedence order.
func overlay(values ...string) string {
	out := ""
	for _, v := range values {
		if v != "" {
			out = v
		}
	}
	return out
}

// flagString returns the string value of the named flag whether it lives on
// cmd's local FlagSet or on a persistent FlagSet up the parent chain. Returns
// an empty string when the flag does not exist.
func flagString(cmd *cobra.Command, name string) string {
	if f := cmd.Flag(name); f != nil {
		return f.Value.String()
	}
	return ""
}

// parseBool parses a NEO4J_INSECURE-style value tolerantly: "1", "true", "yes",
// "on" (case-insensitive) → true; everything else → false. Returns whether the
// input was a recognised truthy form so callers can distinguish "explicitly
// false" from "unset".
func parseBool(s string) (bool, bool) {
	if s == "" {
		return false, false
	}
	if b, err := strconv.ParseBool(s); err == nil {
		return b, true
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "on":
		return true, true
	case "no", "off":
		return false, true
	}
	return false, false
}

// newHTTPClient returns a new *http.Client. When insecure is true, TLS server
// certificate verification is disabled — for development against self-signed
// servers only.
func newHTTPClient(insecure bool) *http.Client {
	if !insecure {
		return &http.Client{}
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // --insecure is a documented dev-only escape hatch
	}
	return &http.Client{Transport: tr}
}

// runStatement POSTs a single Cypher statement to <uri>/db/<database>/query/v2
// and parses the response into a queryResult. Non-2xx responses or non-empty
// errors[] arrays produce a Go error containing the upstream code+message.
//
// Note: a `txMetadata` body field would let server logs (query.log /
// SHOW TRANSACTIONS) tag CLI traffic as e.g. {app: "neo4j-cli", type:
// "user-direct" | "schema"}. The field is only accepted on the v2 endpoint
// from Neo4j 2026.04 onward — earlier servers (5.x, 2025.x) reject the
// request with HTTP 400. Re-enable once that is the minimum supported
// server, ideally gated by a server-version probe so older servers keep
// working.
func runStatement(ctx context.Context, c *conn, statement string, params map[string]any) (*queryResult, error) {
	if c == nil {
		return nil, errors.New("query: nil connection")
	}

	body := map[string]any{"statement": statement}
	if params != nil {
		body["parameters"] = params
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("query: encode request: %w", err)
	}

	url := strings.TrimRight(c.uri, "/") + "/db/" + c.database + "/query/v2"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("query: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(c.username, c.password)
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query: HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close error is not actionable in a defer

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("query: read response: %w", err)
	}

	var parsed queryResponse
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return nil, fmt.Errorf("query: parse response (status %d): %w", resp.StatusCode, err)
		}
	}

	if len(parsed.Errors) > 0 {
		first := parsed.Errors[0]
		return nil, fmt.Errorf("query: server error [%s] %s", first.Code, first.Message)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("query: HTTP %d from %s", resp.StatusCode, url)
	}

	return &queryResult{
		Columns: parsed.Data.Fields,
		Rows:    parsed.Data.Values,
	}, nil
}
