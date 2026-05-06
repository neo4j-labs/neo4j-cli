// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/config"
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

// conn holds the resolved Neo4j connection details. The opened Bolt driver
// is attached lazily via openDriver after the password (if any) has been
// resolved or prompted. Callers MUST close the driver via defer once they are
// done using the connection.
type conn struct {
	uri       string
	username  string
	password  string
	database  string
	insecure  bool
	userAgent string
	driver    neo4j.Driver
}

// queryResult is the parsed tabular payload of a successful Cypher run. The
// shape matches what callers (run.go, schema.go) expect: positional rows where
// each row's order matches Columns.
type queryResult struct {
	Columns []string
	Rows    [][]any
}

// queryResponse is the structured envelope around a Cypher response. Backed
// by the Bolt driver Result + ResultSummary; QueryPlan is derived from the
// driver's ResultSummary.Plan() tree (only non-nil for EXPLAIN/PROFILE).
type queryResponse struct {
	Data struct {
		Fields []string
		Values [][]any
	}
	Bookmarks []string
	QueryPlan *queryPlan
}

// queryPlan captures the operator tree produced by EXPLAIN/PROFILE. Mapped
// from the driver's neo4j.Plan tree returned via ResultSummary.Plan(). The
// top-level walking code in run.go inspects OperatorType and Children only.
type queryPlan struct {
	OperatorType string
	Children     []queryPlan
}

// driverOpener is the test seam used to construct the Bolt driver. Production
// calls neo4j.NewDriverWithContext; tests can swap in a fake to bypass the
// real bolt:// connection.
var driverOpener = func(target string, username, password, userAgent string, insecure bool) (neo4j.Driver, error) {
	configurer := func(c *config.Config) {
		if userAgent != "" {
			c.UserAgent = userAgent
		}
	}
	_ = insecure // insecure handling is selected by URI scheme (e.g. neo4j+ssc://) post-Bolt; kept on conn for compatibility.
	return neo4j.NewDriver(target, neo4j.BasicAuth(username, password, ""), configurer)
}

// runStatementResponseFn is the test seam used by runStatementResponse. It
// lets tests inject canned responses without booting a real Neo4j or
// constructing a Bolt driver. Production sets it to runStatementResponseImpl.
var runStatementResponseFn = runStatementResponseImpl

// resolveConn merges connection settings from .env, OS environment, and
// command-line flags (lowest → highest precedence). When --credential is set,
// the named stored credential is used directly; passing any of --uri/--username/
// --password/--database alongside --credential is an error. When none of the
// four connection params (uri, username, password, database) are explicitly
// provided, the stored default database credential (if any) is used instead.
// Partial explicit overrides (some but not all of the four params) are
// rejected with a descriptive error. The returned conn does NOT hold an open
// driver — callers should fill in the password (prompt if needed) and then
// call c.openDriver(ctx) before issuing queries, and defer c.driver.Close(ctx)
// for cleanup.
func resolveConn(cmd *cobra.Command, cfg *clicfg.Config) (*conn, error) {
	insecureExplicit := cmd.Flag("insecure") != nil && cmd.Flag("insecure").Changed

	// --credential: when set, look up the named credential and use it directly.
	// Dotenv / env vars are skipped entirely; only --insecure may be combined.
	// None of --uri/--username/--password/--database may be set alongside it.
	if f := cmd.Flag("credential"); f != nil && f.Changed {
		credName := f.Value.String()

		// Conflict check: --credential is mutually exclusive with the four
		// individual connection params.
		conflicting := []string{}
		for _, name := range []string{"uri", "username", "password", "database"} {
			if cf := cmd.Flag(name); cf != nil && cf.Changed {
				conflicting = append(conflicting, "--"+name)
			}
		}
		if len(conflicting) > 0 {
			return nil, fmt.Errorf(
				"query: --credential cannot be used together with %s; use one or the other",
				strings.Join(conflicting, ", "))
		}

		cred, err := cfg.Credentials.Dbms.Get(credName)
		if err != nil {
			return nil, fmt.Errorf(
				"query: credential %q not found; run 'neo4j-cli credential dbms list' to see available credentials",
				credName)
		}

		insecure := cred.Insecure
		if insecureExplicit {
			if b, perr := strconv.ParseBool(cmd.Flag("insecure").Value.String()); perr == nil {
				insecure = b
			}
		}

		uri := cred.URI
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
		return &conn{
			uri:       uri,
			username:  cred.Username,
			password:  cred.Password,
			database:  cred.DatabaseName,
			insecure:  insecure,
			userAgent: "neo4j-cli/v" + version,
		}, nil
	}

	// --credential not set: load dotenv and use the standard resolution path.
	envFlag := flagString(cmd, "env")
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("query: cannot determine current directory: %w", err)
	}
	dotenv, err := loadEnvFile(cfg.Aura.Fs(), envFlag, cwd)
	if err != nil {
		return nil, err
	}

	// Collect values from dotenv + OS environment (before flags).
	uri := overlay(dotenv[envURI], os.Getenv(envURI))
	username := overlay(dotenv[envUsername], os.Getenv(envUsername))
	password := overlay(dotenv[envPassword], os.Getenv(envPassword))
	database := overlay(dotenv[envDatabase], os.Getenv(envDatabase))
	insecureStr := overlay(dotenv[envInsecure], os.Getenv(envInsecure))

	// Apply flags (highest precedence — only when the flag was explicitly set).
	if f := cmd.Flag("uri"); f != nil && f.Changed {
		uri = f.Value.String()
	}
	if f := cmd.Flag("username"); f != nil && f.Changed {
		username = f.Value.String()
	}
	if f := cmd.Flag("password"); f != nil && f.Changed {
		password = f.Value.String()
	}
	if f := cmd.Flag("database"); f != nil && f.Changed {
		database = f.Value.String()
	}

	insecure, _ := parseBool(insecureStr)
	if f := cmd.Flag("insecure"); f != nil && f.Changed {
		if b, perr := strconv.ParseBool(f.Value.String()); perr == nil {
			insecure = b
		}
	}

	// Determine how many of the four connection params were explicitly provided
	// (non-empty after merging dotenv + OS env + flags).
	explicitCount := 0
	if uri != "" {
		explicitCount++
	}
	if username != "" {
		explicitCount++
	}
	if password != "" {
		explicitCount++
	}
	if database != "" {
		explicitCount++
	}

	// Try to load the stored default database credential.
	storedCred, _ := cfg.Credentials.Dbms.GetDefault()
	hasStoredCred := storedCred != nil

	switch {
	case !hasStoredCred:
		// No stored credential — existing behaviour: apply what was given and
		// let built-in defaults fill in the blanks below.

	case explicitCount == 0:
		// Stored credential available and no explicit params — use the credential.
		uri = storedCred.URI
		username = storedCred.Username
		password = storedCred.Password
		database = storedCred.DatabaseName
		// Only override insecure from the credential when the flag was not
		// explicitly set by the caller.
		if !insecureExplicit && storedCred.Insecure {
			insecure = true
		}

	case explicitCount == 4:
		// All four explicitly provided — bypass stored credential entirely.

	default:
		// Stored credential exists but only some params were provided — reject
		// the ambiguous partial override.
		return nil, fmt.Errorf(
			"query: partial connection params: when any of --uri/NEO4J_URI, --username/NEO4J_USERNAME, " +
				"--password/NEO4J_PASSWORD, or --database/NEO4J_DATABASE is provided, all four are required")
	}

	// Apply built-in defaults for any param still empty after all sources.
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
	}, nil
}

// openDriver opens a Bolt driver using the resolved connection params and
// stores it on c.driver. Idempotent when already opened. Caller is
// responsible for closing the driver via c.driver.Close(ctx) (typically
// `defer`).
func (c *conn) openDriver() error {
	if c == nil {
		return errors.New("query: nil connection")
	}
	if c.driver != nil {
		return nil
	}
	d, err := driverOpener(c.uri, c.username, c.password, c.userAgent, c.insecure)
	if err != nil {
		return fmt.Errorf("query: open driver: %w", err)
	}
	c.driver = d
	return nil
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

// runStatementResponse executes a Cypher statement against the Bolt driver
// attached to c and returns the parsed envelope (rows + bookmarks + plan).
// Routes through runStatementResponseFn so tests can override.
func runStatementResponse(ctx context.Context, c *conn, statement string, params map[string]any) (*queryResponse, error) {
	return runStatementResponseFn(ctx, c, statement, params)
}

// runStatementResponseImpl is the real Bolt-backed implementation. Opens a
// session targeted at c.database, runs the statement, collects all records,
// and converts the resulting summary's Plan() tree (if present) into the
// internal queryPlan shape.
func runStatementResponseImpl(ctx context.Context, c *conn, statement string, params map[string]any) (*queryResponse, error) {
	if c == nil {
		return nil, errors.New("query: nil connection")
	}
	if c.driver == nil {
		return nil, errors.New("query: connection driver not opened (call openDriver first)")
	}

	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.database})
	defer session.Close(ctx) //nolint:errcheck // session close error not actionable in defer

	result, err := session.Run(ctx, statement, params)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	records, err := result.Collect(ctx)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	summary, err := result.Consume(ctx)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	resp := &queryResponse{}
	if len(records) > 0 {
		resp.Data.Fields = append([]string(nil), records[0].Keys...)
		resp.Data.Values = make([][]any, 0, len(records))
		for _, rec := range records {
			row := make([]any, len(rec.Values))
			copy(row, rec.Values)
			resp.Data.Values = append(resp.Data.Values, row)
		}
	} else {
		// Even with zero rows the result keys are available via the result
		// metadata so downstream renderers see the column header. Fall back to
		// an empty (but non-nil) slice when nothing came back.
		keys, _ := result.Keys()
		resp.Data.Fields = append([]string(nil), keys...)
		resp.Data.Values = [][]any{}
	}

	if summary != nil {
		if plan := summary.Plan(); plan != nil {
			converted := convertPlan(plan)
			resp.QueryPlan = &converted
		}
	}

	return resp, nil
}

// runStatement executes a Cypher statement and returns the tabular result.
func runStatement(ctx context.Context, c *conn, statement string, params map[string]any) (*queryResult, error) {
	parsed, err := runStatementResponse(ctx, c, statement, params)
	if err != nil {
		return nil, err
	}

	return &queryResult{
		Columns: parsed.Data.Fields,
		Rows:    parsed.Data.Values,
	}, nil
}

// convertPlan recursively maps a neo4j.Plan tree into the local queryPlan
// shape. The classifier in run.go walks queryPlan.Children + OperatorType.
func convertPlan(p neo4j.Plan) queryPlan {
	out := queryPlan{OperatorType: p.Operator()}
	children := p.Children()
	if len(children) > 0 {
		out.Children = make([]queryPlan, 0, len(children))
		for _, child := range children {
			out.Children = append(out.Children, convertPlan(child))
		}
	}
	return out
}
