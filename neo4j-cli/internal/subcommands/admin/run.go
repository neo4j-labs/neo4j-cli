// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package admin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/config"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

const (
	systemDatabase = "system"
	auraKBURL      = "support.neo4j.com"

	unsupportedAdminCode = "Neo.ClientError.Statement.UnsupportedAdministrationCommand"
	argumentErrorCode    = "Neo.ClientError.Statement.ArgumentError"
	executionFailedCode  = "Neo.DatabaseError.Statement.ExecutionFailed"
)

// adminStderrLogger routes all Bolt driver log output to stderr so it doesn't
// contaminate stdout (which carries the CLI's machine-readable output).
type adminStderrLogger struct{}

const adminLogTimeFormat = "2006-01-02 15:04:05.000"

func newAdminStderrLogger() *adminStderrLogger { return &adminStderrLogger{} }

func (l *adminStderrLogger) Error(name, id string, err error) {
	_, _ = fmt.Fprintf(os.Stderr, "%s  ERROR  [%s %s] %s\n", time.Now().Format(adminLogTimeFormat), name, id, err.Error())
}

func (l *adminStderrLogger) Warnf(name, id, msg string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "%s   WARN  [%s %s] %s\n", time.Now().Format(adminLogTimeFormat), name, id, fmt.Sprintf(msg, args...))
}

func (l *adminStderrLogger) Infof(name, id, msg string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "%s   INFO  [%s %s] %s\n", time.Now().Format(adminLogTimeFormat), name, id, fmt.Sprintf(msg, args...))
}

func (l *adminStderrLogger) Debugf(name, id, msg string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "%s  DEBUG  [%s %s] %s\n", time.Now().Format(adminLogTimeFormat), name, id, fmt.Sprintf(msg, args...))
}

// queryRunner is the test seam for the admin execution path. Production code
// uses boltAdminRunner; tests substitute a fakeQueryRunner.
type queryRunner interface {
	run(ctx context.Context, conn *dbconn.Conn, cypher string, params map[string]any) ([]map[string]any, error)
}

// boltAdminRunner is the default queryRunner. It opens a Bolt driver using
// the supplied connection params, executes the Cypher via ExecuteWrite targeting
// the system database, and returns the result rows.
type boltAdminRunner struct{}

// adminRunnerFn is the package-level test seam. Tests replace it to inject a
// fakeQueryRunner without opening a real Bolt connection.
var adminRunnerFn = func(_ *clicfg.Config) queryRunner {
	return &boltAdminRunner{}
}

func (r *boltAdminRunner) run(ctx context.Context, conn *dbconn.Conn, cypher string, params map[string]any) ([]map[string]any, error) {
	driverOpts := []func(*config.Config){
		func(c *config.Config) {
			c.UserAgent = conn.UserAgent
			c.ConnectionAcquisitionTimeout = 10 * time.Second
			c.MaxTransactionRetryTime = 10 * time.Second
		},
	}
	if conn.Debug {
		driverOpts = append(driverOpts, func(c *config.Config) {
			c.Log = newAdminStderrLogger()
		})
		fmt.Fprintf(os.Stderr, "[debug] admin cypher: %s\n", cypher)
		if len(params) > 0 {
			fmt.Fprintf(os.Stderr, "[debug] admin params: %v\n", redactParams(params))
		}
	}
	driver, err := neo4j.NewDriver(
		conn.URI,
		neo4j.BasicAuth(conn.Username, conn.Password, ""),
		driverOpts...,
	)
	if err != nil {
		return nil, clierr.NewUpstreamError("admin: open driver: %w", err)
	}
	defer driver.Close(ctx) //nolint:errcheck // driver close error not actionable in defer

	session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: systemDatabase})
	defer session.Close(ctx) //nolint:errcheck // session close error not actionable in defer

	out, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		records, err := result.Collect(ctx)
		if err != nil {
			return nil, err
		}
		if len(records) == 0 {
			return []map[string]any{}, nil
		}
		keys := records[0].Keys
		rows := make([]map[string]any, 0, len(records))
		for _, rec := range records {
			row := make(map[string]any, len(keys))
			for _, k := range keys {
				v, _ := rec.Get(k)
				row[k] = v
			}
			rows = append(rows, row)
		}
		return rows, nil
	})
	if err != nil {
		return nil, err
	}

	rows, ok := out.([]map[string]any)
	if !ok {
		return nil, clierr.NewFatalError("admin: unexpected nil response from managed transaction")
	}
	return rows, nil
}

// redactParams returns a copy of params with values redacted for any key that
// matches a known secret word (password, passwd, secret, token, key, credential).
// The comparison is case-insensitive.
func redactParams(params map[string]any) map[string]any {
	if len(params) == 0 {
		return params
	}
	out := make(map[string]any, len(params))
	for k, v := range params {
		lower := strings.ToLower(k)
		if strings.Contains(lower, "password") || strings.Contains(lower, "passwd") ||
			strings.Contains(lower, "secret") || strings.Contains(lower, "token") ||
			strings.Contains(lower, "key") || strings.Contains(lower, "credential") {
			out[k] = "***"
		} else {
			out[k] = v
		}
	}
	return out
}

// RunAdminStatement executes a Cypher statement against the system database
// and translates well-known Neo4j errors into actionable CLI errors.
func RunAdminStatement(ctx context.Context, cfg *clicfg.Config, conn *dbconn.Conn, cypher string, params map[string]any) ([]map[string]any, error) {
	runner := adminRunnerFn(cfg)
	rows, err := runner.run(ctx, conn, cypher, params)
	if err != nil {
		return nil, translateAdminError(err)
	}
	return rows, nil
}

// translateAdminError converts Neo4j Bolt errors that have well-known
// admin-command semantics into targeted CLI errors.
func translateAdminError(err error) error {
	if err == nil {
		return nil
	}

	// Already a CLIError — leave it alone.
	var ce *clierr.CLIError
	if errors.As(err, &ce) {
		return err
	}

	var ne *neo4j.Neo4jError
	if errors.As(err, &ne) {
		return translateNeo4jError(ne)
	}

	return clierr.NewValidationError("%w", err)
}

func translateNeo4jError(ne *neo4j.Neo4jError) error {
	switch ne.Code {
	case unsupportedAdminCode:
		return translateUnsupportedAdmin(ne.Msg)
	case argumentErrorCode:
		if strings.Contains(ne.Msg, "non-native") || strings.Contains(ne.Msg, "authentication provider apart from native") {
			return clierr.NewValidationError("renaming users is not supported on Aura connections (Aura uses a non-native authentication provider)")
		}
		return clierr.NewValidationError("%w", ne)
	case executionFailedCode:
		if strings.Contains(ne.Msg, "not available in community edition") {
			return clierr.NewValidationError("%s", ne.Msg)
		}
	}

	if ne.Classification() == "ClientError" {
		msg := ne.Msg
		if (strings.Contains(msg, "SET STATUS") || strings.Contains(msg, "HOME DATABASE")) &&
			strings.Contains(msg, "community edition") {
			return clierr.NewValidationError("%s", msg)
		}
		return clierr.NewValidationError("%w", ne)
	}

	return clierr.NewUpstreamError("%w", ne)
}

// translateUnsupportedAdmin selects between the Aura-specific and
// Enterprise-edition hint based on whether the error message contains the
// Aura support KB URL.
func translateUnsupportedAdmin(msg string) error {
	if strings.Contains(msg, auraKBURL) {
		return clierr.NewValidationError("%s (not supported on Aura — use the Aura Console or API)", msg)
	}
	return clierr.NewValidationError("%s (requires Enterprise edition)", msg)
}
