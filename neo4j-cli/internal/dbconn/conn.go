// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package dbconn provides the shared Neo4j connection-resolution logic used by
// both the query and admin command trees. It merges connection settings from
// .env files, OS environment variables, command-line flags, Desktop prefix
// credentials, and the persisted credential store into a single *Conn value.
package dbconn

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/subosito/gotenv"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clicfg/dotenv"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
)

const (
	DefaultURI      = "neo4j://localhost:7687"
	DefaultUsername = "neo4j"
	DefaultDatabase = "neo4j"
)

// Connection env-var names are single-sourced in the credentials package; these
// are re-exported here for the dbconn tests that drive resolution via t.Setenv.
const (
	EnvURI      = credentials.EnvURI
	EnvUsername = credentials.EnvUsername
	EnvPassword = credentials.EnvPassword
	EnvDatabase = credentials.EnvDatabase
)

// Conn holds the resolved Neo4j connection details. It does not carry an open
// Bolt driver — callers open their own driver using URI, Username, Password,
// UserAgent, and Debug.
type Conn struct {
	URI       string
	Username  string
	Password  string
	Database  string
	UserAgent string
	Debug     bool
}

// ResolveConn merges connection settings from .env, OS environment, and
// command-line flags (lowest → highest precedence) into a *Conn.
//
// When --credential is set, its value is dispatched on prefix: "desktop"
// resolves the single running Desktop DBMS; "desktop-connection:<uuid>"
// resolves a saved Desktop connection; anything else is a persisted-store
// lookup. Passing any of --uri/--username/--password (or --database when
// skipDatabase is false) alongside --credential is an error.
//
// When none of the connection params are explicitly provided, the stored
// default dbms credential (if any) is used. Partial explicit overrides are
// rejected with a descriptive error.
//
// When skipDatabase is true (admin mode), --database / NEO4J_DATABASE are
// never consulted; the returned Conn.Database is always empty.
func ResolveConn(cmd *cobra.Command, cfg *clicfg.Config, skipDatabase bool) (*Conn, error) {
	if f := cmd.Flag("credential"); f != nil && f.Changed {
		credName := f.Value.String()

		conflicting := []string{}
		for _, name := range []string{"uri", "username", "password"} {
			if cf := cmd.Flag(name); cf != nil && cf.Changed {
				conflicting = append(conflicting, "--"+name)
			}
		}
		if len(conflicting) > 0 {
			return nil, fmt.Errorf(
				"--credential cannot be used together with %s; use one or the other",
				strings.Join(conflicting, ", "))
		}

		if credName == desktopCredentialPrefix {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			desktopclient.SetDebug(ResolveDebug(cmd))
			match, err := resolveDesktopActiveDbmsCredentialFn(ctx, cfg.Aura.Fs())
			if err != nil {
				return nil, err
			}
			c, err := finishDesktopMatch(cmd, cfg, match)
			if err != nil {
				return nil, err
			}
			applyDBOverride(cmd, cfg, c, skipDatabase)
			return c, nil
		}

		if strings.HasPrefix(credName, desktopConnectionPrefix) {
			raw := strings.TrimPrefix(credName, desktopConnectionPrefix)
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			desktopclient.SetDebug(ResolveDebug(cmd))
			match, err := resolveDesktopConnectionCredentialFn(ctx, cfg.Aura.Fs(), raw)
			if err != nil {
				return nil, err
			}
			c, err := finishDesktopMatch(cmd, cfg, match)
			if err != nil {
				return nil, err
			}
			applyDBOverride(cmd, cfg, c, skipDatabase)
			return c, nil
		}

		cred, err := cfg.Credentials.Dbms.Get(credName)
		if err != nil {
			return nil, clierr.NewFatalError(
				"no persisted credential %q. "+
					"Run 'neo4j-cli credential dbms add' to register a connection, "+
					"or use '--credential desktop' / '--credential desktop-connection:<uuid>' "+
					"for a running Neo4j Desktop 2 DBMS or saved Neo4j Desktop 2 connection.",
				credName)
		}

		c := buildConnFromPersistedCred(cred, cfg, cmd)
		applyDBOverride(cmd, cfg, c, skipDatabase)
		return c, nil
	}

	envFlag := FlagString(cmd, "env")
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot determine current directory: %w", err)
	}
	dotenvVals, err := LoadEnvFile(cfg.Aura.Fs(), envFlag, cwd, cmd.ErrOrStderr())
	if err != nil {
		return nil, err
	}

	uri := Overlay(dotenvVals[credentials.EnvURI], cfg.GatedGetenv(credentials.EnvURI))
	username := Overlay(dotenvVals[credentials.EnvUsername], cfg.GatedGetenv(credentials.EnvUsername))
	password := Overlay(dotenvVals[credentials.EnvPassword], cfg.GatedGetenv(credentials.EnvPassword))
	database := ""
	if !skipDatabase {
		database = Overlay(dotenvVals[credentials.EnvDatabase], cfg.GatedGetenv(credentials.EnvDatabase))
	}

	if f := cmd.Flag("uri"); f != nil && f.Changed {
		uri = f.Value.String()
	}
	if f := cmd.Flag("username"); f != nil && f.Changed {
		username = f.Value.String()
	}
	if f := cmd.Flag("password"); f != nil && f.Changed {
		password = f.Value.String()
	}
	if !skipDatabase {
		if f := cmd.Flag("database"); f != nil && f.Changed {
			database = f.Value.String()
		}
	}

	// In env-var mode, reject a partial DBMS set naming the missing NEO4J_*
	// vars (REQ-F-010). The check runs against the resolved values so a piece
	// supplied by a flag or dotenv counts as present (e.g. env password with
	// flag uri/username is complete).
	if cfg.Global.AcceptEnvVars() {
		resolved := map[string]string{
			credentials.EnvURI:      uri,
			credentials.EnvUsername: username,
			credentials.EnvPassword: password,
		}
		if err := credentials.ValidateEnvCredentialSet(credentials.DBMSEnvSpec, func(name string) string {
			return resolved[name]
		}); err != nil {
			return nil, err
		}
	}

	// Completeness is decided on the three required params only
	// (uri/username/password). database is always optional: it is applied when
	// present but never required and never counted toward completeness, in both
	// query and admin (skipDatabase) modes (REQ-F-014).
	requiredCount := 0
	if uri != "" {
		requiredCount++
	}
	if username != "" {
		requiredCount++
	}
	if password != "" {
		requiredCount++
	}

	storedCred, _ := cfg.Credentials.Dbms.GetDefault()
	hasStoredCred := storedCred != nil

	switch {
	case !hasStoredCred:
		// No stored credential — apply whatever required params were given (or
		// none, falling through to built-in defaults).

	case requiredCount == 0:
		// Stored credential available, no explicit required params — use the
		// credential. An explicit database override is layered on below.
		uri = storedCred.URI
		username = storedCred.Username
		password = storedCred.Password
		if !skipDatabase && database == "" {
			database = storedCred.DatabaseName
		}

	case requiredCount == 3:
		// All three required params provided — bypass stored credential entirely.

	default:
		// Partial override of a stored credential — reject. In env-var mode the
		// completeness check above already named the missing NEO4J_* vars, so this
		// only fires for explicit --flag subsets. When the gate is off, gated env
		// vars contribute nothing, so name only the --flags rather than the
		// misleading --flag/NEO4J_* dual form (REQ-F-018).
		if cfg.Global.AcceptEnvVars() {
			return nil, fmt.Errorf(
				"partial connection params: when any of --uri/NEO4J_URI, --username/NEO4J_USERNAME, " +
					"or --password/NEO4J_PASSWORD is provided, all three (--uri, --username, --password) are required")
		}
		return nil, fmt.Errorf(
			"partial connection params: when any of --uri, --username, or --password is provided, " +
				"all three (--uri, --username, --password) are required")
	}

	if uri == "" {
		uri = DefaultURI
	}
	if username == "" {
		username = DefaultUsername
	}
	// Database is intentionally NOT defaulted: when left empty the session is
	// opened with no DatabaseName so the server resolves the connecting user's
	// home database. Forcing DefaultDatabase ("neo4j") breaks instances whose
	// home database has a different name (e.g. AuraDB Free).

	uri = applyURINorm(cmd, uri)

	version := cfg.Version
	if version == "" {
		version = "dev"
	}

	return &Conn{
		URI:       uri,
		Username:  username,
		Password:  password,
		Database:  database,
		UserAgent: "neo4j-cli/v" + version,
		Debug:     ResolveDebug(cmd),
	}, nil
}

// finishDesktopMatch turns a *DesktopMatch into a *Conn, applying the
// null-creds prompt/fatal branch when Desktop returned a match but no
// stored credentials.
func finishDesktopMatch(cmd *cobra.Command, cfg *clicfg.Config, match *DesktopMatch) (*Conn, error) {
	if match == nil {
		return nil, clierr.NewFatalError("internal: desktop resolver returned nil match without error")
	}
	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	debug := ResolveDebug(cmd)
	if match.Creds != nil {
		return buildConnFromDesktopMatch(match, version, debug), nil
	}
	name, id := desktopMatchIdentity(match)
	c := buildConnFromDesktopMatch(match, version, debug)
	if !StdinIsTTY() {
		return nil, clierr.NewFatalError(
			"Neo4j Desktop 2 has no stored credentials for %q (%s). "+
				"Pass --password (and optionally --username) explicitly, "+
				"or run 'credential dbms add' to register a connection, "+
				"or open Desktop and use 'Reset password' on this resource.",
			name, id)
	}
	pw, perr := PromptPassword(cmd, cfg.Global.AcceptEnvVars())
	if perr != nil {
		return nil, perr
	}
	c.Password = pw
	return c, nil
}

// applyDBOverride layers a --database flag or NEO4J_DATABASE env override onto
// c when skipDatabase is false. Used in the credential path so a persisted or
// Desktop credential's database can be overridden at the call site. The
// NEO4J_DATABASE override is gated behind accept-env-vars (REQ-F-013); the
// explicit --database flag always wins.
func applyDBOverride(cmd *cobra.Command, cfg *clicfg.Config, c *Conn, skipDatabase bool) {
	if skipDatabase {
		return
	}
	if f := cmd.Flag("database"); f != nil && f.Changed {
		c.Database = f.Value.String()
	} else if v := cfg.GatedGetenv(credentials.EnvDatabase); v != "" {
		c.Database = v
	}
}

// applyURINorm normalises uri via NormalizeURI, prints the rewrite info line
// and any cleartext warning to cmd's stderr, and returns the (possibly
// rewritten) URI.
func applyURINorm(cmd *cobra.Command, uri string) string {
	rewritten, didRewrite, displayOrig, warning := NormalizeURI(uri)
	if didRewrite {
		rewrittenDisplay := rewritten
		if u2, _ := url.Parse(rewritten); u2 != nil {
			rewrittenDisplay = u2.Redacted()
		}
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"info: rewrote URI '%s' to '%s' (the command speaks Bolt; pass --uri neo4j://... or neo4j+s://... to silence)\n",
			displayOrig, rewrittenDisplay)
		uri = rewritten
	}
	if warning != "" {
		cmd.PrintErrln(warning)
	}
	return uri
}

// buildConnFromPersistedCred turns a persisted DbmsCredential into a *Conn,
// applying URI normalisation.
func buildConnFromPersistedCred(cred *credentials.DbmsCredential, cfg *clicfg.Config, cmd *cobra.Command) *Conn {
	uri := applyURINorm(cmd, cred.URI)

	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	return &Conn{
		URI:       uri,
		Username:  cred.Username,
		Password:  cred.Password,
		Database:  cred.DatabaseName,
		UserAgent: "neo4j-cli/v" + version,
		Debug:     ResolveDebug(cmd),
	}
}

// ResolveDebug merges the `--debug` flag with the `NEO4J_DEBUG` env var.
// When `--debug` is explicitly set on the command line, its boolean value wins.
// Otherwise debug is enabled iff `NEO4J_DEBUG == "1"`.
func ResolveDebug(cmd *cobra.Command) bool {
	if f := cmd.Flag("debug"); f != nil && f.Changed {
		return f.Value.String() == "true"
	}
	return os.Getenv("NEO4J_DEBUG") == "1"
}

// LoadEnvFile reads a .env file from explicitPath if non-empty, otherwise
// walks up from startDir using the shared dotenv.Find helper. Returns an empty
// (non-nil) map if no file is found and no explicit path was requested.
func LoadEnvFile(fs afero.Fs, explicitPath, startDir string, stderr io.Writer) (map[string]string, error) {
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
		return nil, fmt.Errorf("cannot read env file %q: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only close error is not actionable in a defer

	parsed := gotenv.Parse(f)
	out := make(map[string]string, len(parsed))
	for k, v := range parsed {
		out[k] = v
	}
	return out, nil
}

// Overlay applies values left → right with each non-empty entry overriding
// the earlier accumulator.
func Overlay(values ...string) string {
	out := ""
	for _, v := range values {
		if v != "" {
			out = v
		}
	}
	return out
}

// FlagString returns the string value of the named flag whether it lives on
// cmd's local FlagSet or on a persistent FlagSet up the parent chain.
func FlagString(cmd *cobra.Command, name string) string {
	if f := cmd.Flag(name); f != nil {
		return f.Value.String()
	}
	return ""
}
