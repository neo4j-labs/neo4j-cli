// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"fmt"
	"io"
	"os"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/query/embed"
)

// passwordReader is the test seam for the no-echo TTY password prompt. The
// production implementation calls golang.org/x/term.ReadPassword on
// os.Stdin's file descriptor; tests substitute a stub that returns a fixed
// value without touching the real terminal.
var passwordReader = func() (string, error) {
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	return string(pw), nil
}

// stdinIsTTY is the test seam for terminal detection on stdin. Production
// uses term.IsTerminal; tests override to simulate either piped input or
// an interactive session.
var stdinIsTTY = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// stdinReader is the test seam for reading piped Cypher from stdin. Production
// reads from os.Stdin; tests substitute an in-memory reader.
var stdinReader = func() io.Reader {
	return os.Stdin
}

// runQuery is the parent RunE body. It resolves the connection, the Cypher
// statement (positional arg or piped stdin), and any prompted password,
// executes the statement, applies array truncation and row-limit truncation,
// and renders the result.
func runQuery(cmd *cobra.Command, args []string, cfg *clicfg.Config) error {
	cmd.SilenceUsage = true

	cypher, err := resolveCypher(cmd, args)
	if err != nil {
		return err
	}

	atomicFlag := cmd.Flag("atomic")
	atomic := atomicFlag != nil && atomicFlag.Value.String() == "true"

	continueFlag := cmd.Flag("continue-on-error")
	continueOnError := continueFlag != nil && continueFlag.Value.String() == "true"

	// Reject the incoherent combination before doing any work (no connection,
	// no DB calls): --atomic is all-or-nothing in one transaction, so
	// "continue on error" has no meaning there.
	if atomic && continueOnError {
		return clierr.NewUsageError(
			"--atomic and --continue-on-error are mutually exclusive: --atomic runs all " +
				"statements in one all-or-nothing transaction. Drop --continue-on-error to keep " +
				"atomic rollback, or drop --atomic to run each statement in its own transaction " +
				"and continue past failures.")
	}

	rawParams, _ := cmd.Flags().GetStringArray("param")
	params, embeds, err := parseParams(rawParams)
	if err != nil {
		return err
	}

	if err := resolveEmbedJobs(cmd, cfg, params, embeds); err != nil {
		return err
	}

	c, err := resolveConn(cmd, cfg)
	if err != nil {
		return err
	}

	if c.password == "" {
		pw, err := promptPassword(cmd)
		if err != nil {
			return err
		}
		c.password = pw
	}

	if err := c.openDriver(); err != nil {
		return err
	}
	defer c.driver.Close(cmd.Context()) //nolint:errcheck // driver close error not actionable in defer

	statements := splitStatements(cypher)

	rwFlag := cmd.Flag("rw")
	allowWrite := rwFlag != nil && rwFlag.Value.String() == "true"

	truncOver, _ := cmd.Flags().GetInt("truncate-arrays-over")
	maxRows, _ := cmd.Flags().GetInt("max-rows")
	multi := len(statements) > 1

	var results []renderResult

	if atomic {
		// Run the write-guard preflight over every statement first (read-only,
		// outside the transaction) so a write statement is blocked before any
		// batch transaction is opened.
		if !allowWrite {
			if err := preflightAll(cmd, c, statements, params); err != nil {
				return err
			}
		}
		batch, err := runStatementsWithMode(cmd.Context(), c, statements, params, !allowWrite)
		if err != nil {
			return err
		}
		for i, res := range batch {
			results = append(results, truncateResult(cmd, res, truncOver, maxRows, multi, i+1))
		}
	} else {
		// Default mode keeps preflight interleaved with execution (not hoisted via
		// preflightAll) so statement N executes before statement N+1 is classified,
		// preserving fail-fast ordering across separate transactions.
		failures := 0
		for i, stmt := range statements {
			res, err := runOneStatement(cmd, c, stmt, params, allowWrite)
			if err != nil {
				if !continueOnError {
					return err
				}
				failures++
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "statement %d: %s\n", i+1, err)
				results = append(results, renderResult{errMsg: err.Error()})
				continue
			}
			results = append(results, truncateResult(cmd, res, truncOver, maxRows, multi, i+1))
		}

		if failures > 0 {
			// Render the full array first so the caller sees every statement's
			// outcome, then signal overall failure with a non-zero exit.
			renderResults(cmd, cfg, results)
			return clierr.NewValidationError("%d of %d statements failed", failures, len(statements))
		}
	}

	renderResults(cmd, cfg, results)
	return nil
}

// runOneStatement runs the non-rw EXPLAIN write-guard preflight then executes a
// single statement: read-only (ExecuteRead) when writes are not allowed, write
// (ExecuteWrite) when --rw opted in. It is the unit the default-mode loop
// fails-fast or continues past per --continue-on-error.
func runOneStatement(cmd *cobra.Command, c *conn, stmt string, params map[string]any, allowWrite bool) (*queryResult, error) {
	if !allowWrite {
		if err := rejectWriteCypher(cmd, c, stmt, params); err != nil {
			return nil, err
		}
		return runStatement(cmd.Context(), c, stmt, params)
	}
	return runStatementWrite(cmd.Context(), c, stmt, params)
}

// truncateResult applies array and row truncation to a single statement's
// result and emits the corresponding stderr warnings, returning the
// renderResult ready for output. When multi is true (more than one statement
// ran) each warning is prefixed with "statement N: " (1-based index); a single
// statement keeps the unprefixed wording byte-identical to today.
func truncateResult(cmd *cobra.Command, res *queryResult, truncOver, maxRows int, multi bool, index int) renderResult {
	values, arraysTruncated := truncateValues(res.Rows, truncOver)
	values, truncated := capRows(values, maxRows)

	prefix := ""
	if multi {
		prefix = fmt.Sprintf("statement %d: ", index)
	}
	if truncated {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"%swarning: truncated to %d rows (use --max-rows 0 for unlimited)\n",
			prefix, len(values))
	}
	if arraysTruncated > 0 && truncOver > 0 {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"%swarning: truncated %d arrays larger than %d items (use --truncate-arrays-over 0 to disable)\n",
			prefix, arraysTruncated, truncOver)
	}

	return renderResult{
		columns:         res.Columns,
		rows:            rowsFromValues(res.Columns, values),
		truncated:       truncated,
		arraysTruncated: arraysTruncated,
		stats:           res.Stats,
	}
}

// resolveCypher returns the Cypher statement from the positional arg or, if
// no arg was supplied and stdin is piped, reads it from stdin. A missing
// argument with a TTY stdin is a usage error. Thin wrapper around the shared
// readPositionalOrStdin helper so the `:embed` leaf can reuse the same
// stdin/TTY logic without duplicating it.
func resolveCypher(cmd *cobra.Command, args []string) (string, error) {
	return readPositionalOrStdin(cmd, args, "Cypher")
}

// promptPassword reads a password from the controlling terminal with no echo,
// or returns a clear usage error when stdin is not a TTY (so scripted use
// must supply the password via flag/env/.env).
func promptPassword(cmd *cobra.Command) (string, error) {
	if !stdinIsTTY() {
		return "", clierr.NewUsageError(
			"password is required: set --password, NEO4J_PASSWORD, or add it to a .env file")
	}
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
	pw, err := passwordReader()
	// Always print a newline after the (echo-less) prompt so subsequent output
	// starts on its own line.
	_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("query: read password: %w", err)
	}
	return pw, nil
}

// rejectWriteCypher runs an EXPLAIN preflight against the supplied cypher and
// returns a usage error unless the driver's ResultSummary classifies it as
// QueryTypeReadOnly. EXPLAIN never mutates state, so it always runs inside
// ExecuteRead.
func rejectWriteCypher(cmd *cobra.Command, c *conn, cypher string, params map[string]any) error {
	resp, err := runStatementResponse(cmd.Context(), c, "EXPLAIN "+cypher, params, true)
	if err != nil {
		return err
	}
	if resp.QueryType != neo4j.QueryTypeReadOnly {
		return clierr.NewUsageError("this command writes; pass --rw to allow it")
	}
	return nil
}

// preflightAll runs the rejectWriteCypher write-guard over every statement and
// returns the first error, blocking a write statement before any execution.
func preflightAll(cmd *cobra.Command, c *conn, statements []string, params map[string]any) error {
	for _, stmt := range statements {
		if err := rejectWriteCypher(cmd, c, stmt, params); err != nil {
			return err
		}
	}
	return nil
}

// truncateValues applies truncateArrays to each row's positional values. The
// returned slice is a freshly allocated outer slice; inner slices are
// reallocated only where truncation actually changes the data. The second
// return value is the aggregate count of slices elided across all rows;
// each over-limit slice (including nested ones) increments the count by 1.
func truncateValues(values [][]any, max int) ([][]any, int) {
	if max <= 0 {
		return values, 0
	}
	out := make([][]any, len(values))
	total := 0
	for i, row := range values {
		newRow := make([]any, len(row))
		for j, v := range row {
			truncated, c := truncateArrays(v, max)
			newRow[j] = truncated
			total += c
		}
		out[i] = newRow
	}
	return out, total
}

// capRows enforces --max-rows. A maxRows <= 0 means unlimited; a positive
// limit caps the slice and reports truncated=true when the original was
// longer than the limit.
func capRows(values [][]any, maxRows int) ([][]any, bool) {
	if maxRows <= 0 {
		return values, false
	}
	if len(values) <= maxRows {
		return values, false
	}
	return values[:maxRows], true
}

// resolveEmbedJobs runs each pending EmbedJob through the resolved embed
// provider and inserts the resulting vector into params under the job's name.
// The same params map then feeds both the EXPLAIN preflight and the real
// statement execution, so the vector is computed exactly once per invocation.
//
// A no-op when embeds is empty: the embed package is not consulted, so a
// query that uses no `:embed` params never pays the cost of resolving an
// embed config or constructing a provider.
func resolveEmbedJobs(cmd *cobra.Command, cfg *clicfg.Config, params map[string]any, embeds []EmbedJob) error {
	if len(embeds) == 0 {
		return nil
	}
	ec, err := embed.Resolve(cmd, cfg)
	if err != nil {
		return err
	}
	provider, err := embed.Factory()(ec)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	for _, j := range embeds {
		vec, err := provider.Embed(ctx, j.Text)
		if err != nil {
			return fmt.Errorf("query: embed %q: %w", j.Name, err)
		}
		params[j.Name] = vec
	}
	return nil
}
