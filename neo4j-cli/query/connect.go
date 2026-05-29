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
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/config"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/log"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/subosito/gotenv"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clicfg/dotenv"
	"github.com/neo4j/cli/common/clierr"
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
	debug     bool
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

// stderrLogger is an in-package adapter implementing the neo4j/log.Logger
// interface that routes ALL four levels (Error / Warnf / Infof / Debugf) to
// stderr (default: os.Stderr). The driver-shipped log.ToConsole writes
// DEBUG / INFO / WARN to stdout, which would corrupt the query command's
// machine-readable output streams (--format json, --format toon); routing
// everything to stderr keeps stdout reserved for the rendered result.
type stderrLogger struct {
	w     io.Writer
	level log.Level
}

// newStderrLogger constructs a stderrLogger writing to os.Stderr filtered to
// the supplied level (DEBUG enables all levels). Tests that need to capture
// writes construct a stderrLogger literal pointing at a bytes.Buffer.
func newStderrLogger(level log.Level) *stderrLogger {
	return &stderrLogger{w: os.Stderr, level: level}
}

const stderrLoggerTimeFormat = "2006-01-02 15:04:05.000"

func (l *stderrLogger) Error(name, id string, err error) {
	if l.level < log.ERROR {
		return
	}
	_, _ = fmt.Fprintf(l.w, "%s  ERROR  [%s %s] %s\n", time.Now().Format(stderrLoggerTimeFormat), name, id, err.Error())
}

func (l *stderrLogger) Warnf(name, id, msg string, args ...any) {
	if l.level < log.WARNING {
		return
	}
	_, _ = fmt.Fprintf(l.w, "%s   WARN  [%s %s] %s\n", time.Now().Format(stderrLoggerTimeFormat), name, id, fmt.Sprintf(msg, args...))
}

func (l *stderrLogger) Infof(name, id, msg string, args ...any) {
	if l.level < log.INFO {
		return
	}
	_, _ = fmt.Fprintf(l.w, "%s   INFO  [%s %s] %s\n", time.Now().Format(stderrLoggerTimeFormat), name, id, fmt.Sprintf(msg, args...))
}

func (l *stderrLogger) Debugf(name, id, msg string, args ...any) {
	if l.level < log.DEBUG {
		return
	}
	_, _ = fmt.Fprintf(l.w, "%s  DEBUG  [%s %s] %s\n", time.Now().Format(stderrLoggerTimeFormat), name, id, fmt.Sprintf(msg, args...))
}

// buildDriverConfigurer returns the closure neo4j.NewDriver applies to its
// default *config.Config. Sets UserAgent when non-empty; attaches the in-package
// stderrLogger at DEBUG level when debug is true; otherwise leaves c.Log nil so
// the driver stays silent. Extracted from driverOpener so tests can exercise
// the wiring against a synthetic *config.Config without touching neo4j.NewDriver.
func buildDriverConfigurer(userAgent string, debug bool) func(*config.Config) {
	return func(c *config.Config) {
		if userAgent != "" {
			c.UserAgent = userAgent
		}
		if debug {
			c.Log = newStderrLogger(log.DEBUG)
		}
		// interactive CLI fails fast; the driver's 1m default reads as a hang
		c.ConnectionAcquisitionTimeout = 10 * time.Second
		// interactive CLI fail-fast; default 30s feels like a hang
		c.MaxTransactionRetryTime = 10 * time.Second
	}
}

// driverOpener is the test seam used to construct the Bolt driver. Production
// calls neo4j.NewDriver; tests can swap in a fake to bypass the real bolt://
// connection. When debug is true the configurer attaches an in-package
// stderrLogger at DEBUG level so the driver's wire activity goes to stderr;
// when false c.Log is left at its nil default.
var driverOpener = func(target string, username, password, userAgent string, debug bool) (neo4j.Driver, error) {
	return neo4j.NewDriver(target, neo4j.BasicAuth(username, password, ""), buildDriverConfigurer(userAgent, debug))
}

// runStatementResponseFn is the test seam used by runStatementResponse. It
// lets tests inject canned responses without booting a real Neo4j or
// constructing a Bolt driver. Production sets it to runStatementResponseImpl.
// The readOnly flag selects ExecuteRead vs ExecuteWrite in production; tests
// can assert on it to verify correct routing.
var runStatementResponseFn = runStatementResponseImpl

// runStatementsResponseFn is the batch counterpart of runStatementResponseFn.
// It lets tests inject canned per-statement responses for the single-transaction
// (--atomic) path without booting a real Neo4j. Production sets it to
// runStatementsResponseImpl. The readOnly flag selects ExecuteRead vs
// ExecuteWrite in production; tests can assert on it.
var runStatementsResponseFn = runStatementsResponseImpl

// resolveConn merges connection settings from .env, OS environment, and
// command-line flags (lowest → highest precedence). When --credential is set,
// the value is dispatched on its prefix: `desktop` resolves the single
// running Desktop DBMS; `desktop-connection:<uuid>` resolves a saved
// Desktop connection by UUID; any other value is a persisted-store lookup
// (no implicit Desktop fallthrough). Passing any of
// --uri/--username/--password/--database alongside --credential is an error.
// When none of the four connection params (uri, username, password,
// database) are explicitly provided, the stored default database credential
// (if any) is used instead. Partial explicit overrides (some but not all of
// the four params) are rejected with a descriptive error. The returned conn
// does NOT hold an open driver — callers should fill in the password
// (prompt if needed) and then call c.openDriver(ctx) before issuing
// queries, and defer c.driver.Close(ctx) for cleanup.
func resolveConn(cmd *cobra.Command, cfg *clicfg.Config) (*conn, error) {
	// --credential: when set, dispatch on the value's prefix. Dotenv / env
	// vars are skipped entirely. None of --uri/--username/--password/
	// --database may be set alongside it.
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

		if credName == desktopCredentialPrefix {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			match, err := resolveDesktopActiveDbmsCredentialFn(ctx, cfg.Aura.Fs())
			if err != nil {
				return nil, err
			}
			return finishDesktopMatch(cmd, cfg, match)
		}

		// Prefix sniff happens BEFORE the persisted lookup so the literal
		// `desktop-connection:` namespace is reserved regardless of any
		// persisted entry that happens to share the name.
		if strings.HasPrefix(credName, desktopConnectionPrefix) {
			raw := strings.TrimPrefix(credName, desktopConnectionPrefix)
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			match, err := resolveDesktopConnectionCredentialFn(ctx, cfg.Aura.Fs(), raw)
			if err != nil {
				return nil, err
			}
			return finishDesktopMatch(cmd, cfg, match)
		}

		// Anything else is a persisted-store lookup. No Desktop fallthrough —
		// a miss surfaces a single error pointing at `credential dbms add`
		// and the two Desktop prefix forms.
		cred, err := cfg.Credentials.Dbms.Get(credName)
		if err != nil {
			return nil, clierr.NewFatalError(
				"no persisted credential %q. "+
					"Run 'neo4j-cli credential dbms add' to register a connection, "+
					"or use '--credential desktop' / '--credential desktop-connection:<uuid>' "+
					"for a running Neo4j Desktop 2 DBMS or saved Neo4j Desktop 2 connection.",
				credName)
		}

		return buildConnFromPersistedCred(cred, cfg, cmd), nil
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
	case !hasStoredCred && explicitCount == 0:
		// No stored credential, no explicit params — fall through to
		// built-in defaults. Users opt into a Desktop DBMS via
		// `--credential desktop` or `--credential desktop-connection:<uuid>`.

	case !hasStoredCred:
		// No stored credential and some explicit params provided — apply
		// what was given and let built-in defaults fill in the blanks below.

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
		debug:     resolveDebug(cmd),
	}, nil
}

// finishDesktopMatch turns a *desktopMatch into a *conn, applying the
// null-creds prompt/fatal branch when Desktop returned a match but no
// stored credentials. A nil match is a programming error — resolvers MUST
// return either a non-nil match or an error.
func finishDesktopMatch(cmd *cobra.Command, cfg *clicfg.Config, match *desktopMatch) (*conn, error) {
	if match == nil {
		return nil, clierr.NewFatalError("query: internal: desktop resolver returned nil match without error")
	}
	if match.creds != nil {
		return buildConnFromDesktopMatch(match, cfg, cmd), nil
	}
	// Desktop knows the resource but has no stored credentials (legacy DBMS
	// / safeStorage unavailable). On a TTY prompt for the password; on a
	// non-TTY fatal with the 3-option hint.
	name, id := desktopMatchIdentity(match)
	c := buildConnFromDesktopMatch(match, cfg, cmd)
	if !stdinIsTTY() {
		return nil, clierr.NewFatalError(
			"Neo4j Desktop 2 has no stored credentials for %q (%s). "+
				"Pass --password (and optionally --username) explicitly, "+
				"or run 'credential dbms add' to register a connection, "+
				"or open Desktop and use 'Reset password' on this resource.",
			name, id)
	}
	pw, perr := promptPassword(cmd)
	if perr != nil {
		return nil, perr
	}
	c.password = pw
	return c, nil
}

// buildConnFromPersistedCred turns a persisted DbmsCredential into the *conn
// shape resolveConn returns, applying the same URI-normalisation +
// stderr-info-line wiring the inline --credential path used to do.
func buildConnFromPersistedCred(cred *credentials.DbmsCredential, cfg *clicfg.Config, cmd *cobra.Command) *conn {
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
		debug:     resolveDebug(cmd),
	}
}

// resolveDebug merges the `--debug` flag with the `NEO4J_DEBUG` env var. Flag
// precedence: when `--debug` is explicitly set on the command line, its boolean
// value wins (so `--debug=false` overrides `NEO4J_DEBUG=1`). Otherwise debug is
// enabled iff `NEO4J_DEBUG == "1"` (strict — any other value, including `true`
// / `yes` / `on` / `0`, leaves debug OFF). Dotenv is intentionally not
// consulted; only `os.Getenv` is read.
func resolveDebug(cmd *cobra.Command) bool {
	if f := cmd.Flag("debug"); f != nil && f.Changed {
		return f.Value.String() == "true"
	}
	return os.Getenv("NEO4J_DEBUG") == "1"
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
	d, err := driverOpener(c.uri, c.username, c.password, c.userAgent, c.debug)
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
				for i, v := range rec.Values {
					row[i] = coerceDriverValue(v)
				}
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

// runStatementsResponse executes a batch of Cypher statements inside a single
// managed transaction and returns one parsed envelope per statement, in source
// order. Routes through runStatementsResponseFn so tests can override. The
// readOnly flag drives ExecuteRead vs ExecuteWrite selection inside the
// production impl.
//
// Driver errors are categorised here (the single dispatch boundary that both
// production and the test seam flow through), mirroring runStatementResponse:
// Cypher ClientError-class failures map to validation errors (exit 6);
// transport / TransientError / DatabaseError failures map to upstream errors
// (exit 8). An error from any statement aborts the transaction, so the managed
// transaction rolls back automatically and no partial result is returned.
func runStatementsResponse(ctx context.Context, c *conn, statements []string, params map[string]any, readOnly bool) ([]*queryResponse, error) {
	resps, err := runStatementsResponseFn(ctx, c, statements, params, readOnly)
	if err != nil {
		return nil, categorizeBoltError(err)
	}
	return resps, nil
}

// runStatementsResponseImpl is the real Bolt-backed batch implementation. Opens
// ONE session targeted at c.database and runs ONE managed transaction
// (ExecuteRead when readOnly is true, ExecuteWrite otherwise) whose work
// callback loops tx.Run → Collect → Consume per statement, appending a
// *queryResponse each (reusing coerceDriverValue, identical to the
// single-statement impl). Any error returned from the callback aborts the
// transaction, so the driver rolls it back automatically. The session is closed
// via defer; the driver retains pooling.
func runStatementsResponseImpl(ctx context.Context, c *conn, statements []string, params map[string]any, readOnly bool) ([]*queryResponse, error) {
	if c == nil {
		return nil, errors.New("query: nil connection")
	}
	if c.driver == nil {
		return nil, errors.New("query: connection driver not opened (call openDriver first)")
	}

	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.database})
	defer session.Close(ctx) //nolint:errcheck // session close error not actionable in defer

	work := func(tx neo4j.ManagedTransaction) (any, error) {
		responses := make([]*queryResponse, 0, len(statements))
		for _, statement := range statements {
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
					for i, v := range rec.Values {
						row[i] = coerceDriverValue(v)
					}
					resp.Data.Values = append(resp.Data.Values, row)
				}
			} else {
				keys, _ := result.Keys()
				resp.Data.Fields = append([]string(nil), keys...)
				resp.Data.Values = [][]any{}
			}

			if summary != nil {
				resp.QueryType = summary.QueryType()
			}

			responses = append(responses, resp)
		}
		return responses, nil
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

	resps, ok := out.([]*queryResponse)
	if !ok {
		return nil, errors.New("query: unexpected nil response from managed transaction")
	}
	return resps, nil
}

// runStatementsWithMode executes a batch of statements in a single managed
// transaction and unwraps the envelopes into []*queryResult (Columns/Rows),
// mirroring runStatementWithMode. Results are returned in source order.
func runStatementsWithMode(ctx context.Context, c *conn, statements []string, params map[string]any, readOnly bool) ([]*queryResult, error) {
	parsed, err := runStatementsResponse(ctx, c, statements, params, readOnly)
	if err != nil {
		return nil, err
	}

	results := make([]*queryResult, 0, len(parsed))
	for _, p := range parsed {
		results = append(results, &queryResult{
			Columns: p.Data.Fields,
			Rows:    p.Data.Values,
		})
	}
	return results, nil
}
