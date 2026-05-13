// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/config"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/subosito/gotenv"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/dotenv"
)

const (
	defaultURI      = "neo4j://localhost:7687"
	defaultUsername = "neo4j"
	defaultDatabase = "neo4j"

	envURI      = "NEO4J_URI"
	envUsername = "NEO4J_USERNAME"
	envPassword = "NEO4J_PASSWORD"
	envDatabase = "NEO4J_DATABASE"
)

// conn holds the resolved Neo4j connection details. The opened Bolt driver
// is attached lazily via openDriver after the password (if any) has been
// resolved or prompted. Callers MUST close the driver via defer once they are
// done using the connection. TLS is selected exclusively by the URI scheme
// (e.g. neo4j+s:// for verified TLS, neo4j+ssc:// for self-signed certs).
type conn struct {
	uri       string
	username  string
	password  string
	database  string
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
// by the Bolt driver Result + ResultSummary. QueryType is taken straight
// from ResultSummary.QueryType() and is what the --rw classifier inspects
// for EXPLAIN preflight runs (QueryTypeReadOnly → safe; everything else
// requires --rw). The driver also exposes the equivalent (deprecated)
// StatementType() / StatementTypeReadOnly aliases — we use the QueryType
// names so staticcheck does not flag them.
type queryResponse struct {
	Data struct {
		Fields []string
		Values [][]any
	}
	Bookmarks []string
	QueryType neo4j.QueryType
}

// driverOpener is the test seam used to construct the Bolt driver. Production
// calls neo4j.NewDriver; tests can swap in a fake to bypass the real bolt://
// connection.
var driverOpener = func(target string, username, password, userAgent string) (neo4j.Driver, error) {
	configurer := func(c *config.Config) {
		if userAgent != "" {
			c.UserAgent = userAgent
		}
	}
	return neo4j.NewDriver(target, neo4j.BasicAuth(username, password, ""), configurer)
}

// runStatementResponseFn is the test seam used by runStatementResponse. It
// lets tests inject canned responses without booting a real Neo4j or
// constructing a Bolt driver. Production sets it to runStatementResponseImpl.
// The readOnly flag selects ExecuteRead vs ExecuteWrite in production; tests
// can assert on it to verify correct routing.
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
	// --credential: when set, look up the named credential and use it directly.
	// Dotenv / env vars are skipped entirely. None of --uri/--username/
	// --password/--database may be set alongside it.
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

		uri := cred.URI
		rewritten, didRewrite, displayOrig, warning := normalizeURI(uri)
		if didRewrite {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
				"info: rewrote URI '%s' to '%s' (the query command speaks Bolt; pass --uri neo4j://... or neo4j+s://... to silence)\n",
				displayOrig, rewritten)
			uri = rewritten
		}
		if warning != "" {
			cmd.PrintErrln(warning)
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
			userAgent: "neo4j-cli/v" + version,
		}, nil
	}

	// --credential not set: load dotenv and use the standard resolution path.
	envFlag := flagString(cmd, "env")
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("query: cannot determine current directory: %w", err)
	}
	dotenvVals, err := loadEnvFile(cfg.Aura.Fs(), envFlag, cwd, cmd.ErrOrStderr())
	if err != nil {
		return nil, err
	}

	// Collect values from dotenv + OS environment (before flags).
	uri := overlay(dotenvVals[envURI], os.Getenv(envURI))
	username := overlay(dotenvVals[envUsername], os.Getenv(envUsername))
	password := overlay(dotenvVals[envPassword], os.Getenv(envPassword))
	database := overlay(dotenvVals[envDatabase], os.Getenv(envDatabase))

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

	rewritten, didRewrite, displayOrig, warning := normalizeURI(uri)
	if didRewrite {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"info: rewrote URI '%s' to '%s' (the query command speaks Bolt; pass --uri neo4j://... or neo4j+s://... to silence)\n",
			displayOrig, rewritten)
		uri = rewritten
	}
	if warning != "" {
		cmd.PrintErrln(warning)
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
		userAgent: userAgent,
	}, nil
}

// openDriver opens a Bolt driver using the resolved connection params and
// stores it on c.driver. Idempotent when already opened. Caller is
// responsible for closing the driver via c.driver.Close(ctx) (typically
// `defer`).
//
// Driver-construction errors (connectivity, malformed URI, TLS handshake)
// surface as upstream errors so the process exits with code 8 — they are
// transport-level failures, not user-input validation.
func (c *conn) openDriver() error {
	if c == nil {
		return errors.New("query: nil connection")
	}
	if c.driver != nil {
		return nil
	}
	d, err := driverOpener(c.uri, c.username, c.password, c.userAgent)
	if err != nil {
		return categorizeBoltError(fmt.Errorf("query: open driver: %w", err))
	}
	c.driver = d
	return nil
}

// loadEnvFile reads a .env file from explicitPath if non-empty, otherwise walks
// up from startDir using the shared dotenv.Find helper (stops at the first
// .git ancestor or the $HOME boundary). Returns an empty (non-nil) map if no
// file is found and no explicit path was requested. An explicit path that
// does not exist is an error. When the discovered .env lives in a directory
// strictly above startDir an `info: loading .env from <path>` line is written
// to stderr so the overlay isn't silent.
func loadEnvFile(fs afero.Fs, explicitPath, startDir string, stderr io.Writer) (map[string]string, error) {
	path := explicitPath
	if path == "" {
		var (
			ok       bool
			aboveCWD bool
		)
		path, ok, aboveCWD = dotenv.Find(fs, startDir)
		if !ok {
			return map[string]string{}, nil
		}
		if aboveCWD && stderr != nil {
			_, _ = fmt.Fprintf(stderr, "info: loading .env from %s\n", path)
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

// runStatementResponse executes a Cypher statement against the Bolt driver
// attached to c and returns the parsed envelope (rows + bookmarks + plan).
// Routes through runStatementResponseFn so tests can override. The readOnly
// flag drives ExecuteRead vs ExecuteWrite selection inside the production
// impl; preflight EXPLAIN and read-only execution pass true, post-classified
// write execution passes false.
//
// Driver errors are categorised here (the single dispatch boundary that both
// production and the test seam flow through) so callers further up never have
// to deal with raw Bolt-driver errors: Cypher ClientError-class failures map
// to validation errors (exit 6); transport / TransientError / DatabaseError
// failures map to upstream errors (exit 8).
func runStatementResponse(ctx context.Context, c *conn, statement string, params map[string]any, readOnly bool) (*queryResponse, error) {
	resp, err := runStatementResponseFn(ctx, c, statement, params, readOnly)
	if err != nil {
		return nil, categorizeBoltError(err)
	}
	return resp, nil
}

// runStatementResponseImpl is the real Bolt-backed implementation. Opens a
// session targeted at c.database, runs the statement inside a managed
// transaction (ExecuteRead when readOnly is true, ExecuteWrite otherwise),
// collects all records, and pulls summary.QueryType() (used by the --rw
// classifier on EXPLAIN preflight runs) onto the response. The session is
// closed via defer; the driver retains pooling.
func runStatementResponseImpl(ctx context.Context, c *conn, statement string, params map[string]any, readOnly bool) (*queryResponse, error) {
	if c == nil {
		return nil, errors.New("query: nil connection")
	}
	if c.driver == nil {
		return nil, errors.New("query: connection driver not opened (call openDriver first)")
	}

	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.database})
	defer session.Close(ctx) //nolint:errcheck // session close error not actionable in defer

	work := func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, statement, params)
		if err != nil {
			return nil, err
		}

		records, err := result.Collect(ctx)
		if err != nil {
			return nil, err
		}

		summary, err := result.Consume(ctx)
		if err != nil {
			return nil, err
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
			// metadata so downstream renderers see the column header. Fall back
			// to an empty (but non-nil) slice when nothing came back.
			keys, _ := result.Keys()
			resp.Data.Fields = append([]string(nil), keys...)
			resp.Data.Values = [][]any{}
		}

		if summary != nil {
			resp.QueryType = summary.QueryType()
		}

		return resp, nil
	}

	var (
		out any
		err error
	)
	if readOnly {
		out, err = session.ExecuteRead(ctx, work)
	} else {
		out, err = session.ExecuteWrite(ctx, work)
	}
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	resp, ok := out.(*queryResponse)
	if !ok || resp == nil {
		return nil, errors.New("query: unexpected nil response from managed transaction")
	}
	return resp, nil
}

// runStatement executes a Cypher statement and returns the tabular result.
// Defaults to readOnly=true; callers that classify a statement as a write
// (post-EXPLAIN or with --rw) MUST use runStatementWrite instead.
func runStatement(ctx context.Context, c *conn, statement string, params map[string]any) (*queryResult, error) {
	return runStatementWithMode(ctx, c, statement, params, true)
}

// runStatementWrite executes a Cypher statement on the write path, routing
// through ExecuteWrite. Used by the query run leaf when --rw is set (the
// user has opted in) or when the post-classification path determines the
// statement mutates state.
func runStatementWrite(ctx context.Context, c *conn, statement string, params map[string]any) (*queryResult, error) {
	return runStatementWithMode(ctx, c, statement, params, false)
}

func runStatementWithMode(ctx context.Context, c *conn, statement string, params map[string]any, readOnly bool) (*queryResult, error) {
	parsed, err := runStatementResponse(ctx, c, statement, params, readOnly)
	if err != nil {
		return nil, err
	}

	return &queryResult{
		Columns: parsed.Data.Fields,
		Rows:    parsed.Data.Values,
	}, nil
}
