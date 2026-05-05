// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
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

	rawParams, _ := cmd.Flags().GetStringArray("param")
	params, err := parseParams(rawParams)
	if err != nil {
		return clierr.NewUsageError("%s", err.Error())
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

	res, err := runStatement(cmd.Context(), c, cypher, params)
	if err != nil {
		return err
	}

	truncOver, _ := cmd.Flags().GetInt("truncate-arrays-over")
	values, arraysTruncated := truncateValues(res.Rows, truncOver)

	maxRows, _ := cmd.Flags().GetInt("max-rows")
	values, truncated := capRows(values, maxRows)
	if truncated {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: truncated to %d rows (use --max-rows 0 for unlimited)\n",
			len(values))
	}
	if arraysTruncated > 0 && truncOver > 0 {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: truncated %d arrays larger than %d items (use --truncate-arrays-over 0 to disable)\n",
			arraysTruncated, truncOver)
	}

	rows := rowsFromValues(res.Columns, values)
	renderRows(cmd, cfg, res.Columns, rows, truncated, arraysTruncated)
	return nil
}

// resolveCypher returns the Cypher statement from the positional arg or, if
// no arg was supplied and stdin is piped, reads it from stdin. A missing
// argument with a TTY stdin is a usage error.
func resolveCypher(_ *cobra.Command, args []string) (string, error) {
	if len(args) == 1 {
		s := strings.TrimSpace(args[0])
		if s == "" {
			return "", clierr.NewUsageError("cypher statement is empty")
		}
		return s, nil
	}
	if stdinIsTTY() {
		return "", clierr.NewUsageError(
			"no Cypher provided: pass a positional argument or pipe a statement on stdin")
	}
	b, err := io.ReadAll(stdinReader())
	if err != nil {
		return "", fmt.Errorf("query: read stdin: %w", err)
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", clierr.NewUsageError("no Cypher provided on stdin")
	}
	return s, nil
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
