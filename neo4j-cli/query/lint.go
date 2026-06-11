// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/query/linter"
)

// lintFields drives the column set and order for table/json rendering of
// lint diagnostics.
var lintFields = []string{"severity", "message", "line", "column", "offset", "end_line", "end_column", "end_offset"}

// lintFn is the test seam over the real linter engine: policy paths (engine
// failure → fatal, warnings-only → exit 0) are exercised deterministically by
// swapping it in lint_test.go.
var lintFn = linter.Lint

// lintDiagnostic is one rendered diagnostic row. Line/column are 1-indexed
// (matching how the Neo4j server reports positions); offsets are 0-indexed
// byte offsets as the analyzer emits them.
type lintDiagnostic struct {
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Offset    int    `json:"offset"`
	EndLine   int    `json:"end_line"`
	EndColumn int    `json:"end_column"`
	EndOffset int    `json:"end_offset"`
}

// lintDiagnostics implements commonoutput.ResponseData so PrintBodyMap can
// render the rows in any of the supported formats.
type lintDiagnostics []lintDiagnostic

// AsArray satisfies commonoutput.ResponseData: one row per diagnostic, keyed
// by the lintFields names.
func (d lintDiagnostics) AsArray() []map[string]any {
	rows := make([]map[string]any, 0, len(d))
	for _, diag := range d {
		rows = append(rows, map[string]any{
			"severity":   diag.Severity,
			"message":    diag.Message,
			"line":       diag.Line,
			"column":     diag.Column,
			"offset":     diag.Offset,
			"end_line":   diag.EndLine,
			"end_column": diag.EndColumn,
			"end_offset": diag.EndOffset,
		})
	}
	return rows
}

// MarshalJSON renders nil/empty as `[]` (never null), matching the
// embedVector precedent so a clean lint is an empty JSON array.
func (d lintDiagnostics) MarshalJSON() ([]byte, error) {
	if d == nil {
		return json.Marshal([]lintDiagnostic{})
	}
	return json.Marshal([]lintDiagnostic(d))
}

// newLintCmd builds the `:lint` cobra leaf. Offline by default: no Bolt
// connection is opened and no password is prompted — the embedded analysis
// engine does all the work in-process. `--fetch-schema` opts into one
// connection to pull the schema before linting; the lint itself still runs
// locally.
func newLintCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   ":lint [cypher]",
		Short: "Lint Cypher: report syntax and semantic errors, offline by default",
		Long: "Check Cypher for syntax and semantic problems using the same " +
			"analysis that powers Neo4j's language tooling. Offline by " +
			"default: no Bolt connection is opened and no credentials are " +
			"needed. With `--fetch-schema` the database schema (labels, " +
			"relationship types, property keys, graph shape, default Cypher " +
			"version) is fetched first — connection resolved like the other " +
			"query commands — enabling additional schema-aware warnings: " +
			"unknown labels or relationship types, and relationship patterns " +
			"that contradict the graph's actual direction. Schema warnings " +
			"never affect the exit code. Declaring parameters with `--param` " +
			"switches parameter checking on: any $parameter not declared is " +
			"an error; without `--param` parameter checks are skipped. " +
			"Cypher is taken from the positional " +
			"argument, or from stdin when no argument is provided and stdin " +
			"is piped. `--cypher-version` selects the language dialect (5 or " +
			"25; default 5); a `CYPHER 5`/`CYPHER 25` prologue in the query " +
			"always wins, and with `--fetch-schema` the database's default " +
			"language applies unless `--cypher-version` is set explicitly. " +
			"Each diagnostic renders as one row with a severity of `error` " +
			"or `warning`, a message, and 1-indexed line/column plus " +
			"0-indexed character offsets. Exit code is 6 when any " +
			"error-severity diagnostic is found; a clean or warnings-only " +
			"result exits 0. The first call in a process takes a few seconds " +
			"to initialize the analysis engine.",
		Example: `# Lint a Cypher statement offline; diagnostics as JSON, exit code 6 on errors
neo4j-cli query :lint "MATCH (n) RETURN m" --format json

# Lint against the connected database's schema (catches unknown labels/rel-types)
neo4j-cli query :lint "MATCH (n:Persn)-[:ACTED_IN]->(m) RETURN m" --fetch-schema --format json

# Declare parameters to catch misspelled ones (undeclared $unknown errors)
neo4j-cli query :lint "RETURN $known + $unknown" --param known=1 --format json

# Pipe Cypher from stdin and lint against Cypher 25 semantics
cat query.cypher | neo4j-cli query :lint --cypher-version 25 --format json

# Human-readable diagnostics table
neo4j-cli query :lint "MATCH (n) RETURN n" --format table`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLint(cmd, args, cfg)
		},
	}
	cmd.Flags().String("cypher-version", "5", "Cypher language version to lint against: 5 or 25")
	cmd.Flags().Bool("fetch-schema", false, "Fetch the schema from the database before linting, enabling schema-aware warnings (unknown labels/relationship types, path directionality)")
	return cmd
}

// runLint is the `:lint` RunE body: resolve input, map the version flag,
// optionally fetch the schema, run the embedded analyzer, render all
// diagnostics, then fail with a validation error iff any error-severity
// diagnostic was found (warnings alone exit 0).
func runLint(cmd *cobra.Command, args []string, cfg *clicfg.Config) error {
	cmd.SilenceUsage = true

	cypher, err := readPositionalOrStdin(cmd, args, "Cypher")
	if err != nil {
		return err
	}

	var version linter.Version
	switch v, _ := cmd.Flags().GetString("cypher-version"); v {
	case "5":
		version = linter.Cypher5
	case "25":
		version = linter.Cypher25
	default:
		return clierr.NewUsageError("invalid --cypher-version %q: must be 5 or 25", v)
	}

	var schema *linter.DbSchema
	if fetch, _ := cmd.Flags().GetBool("fetch-schema"); fetch {
		schema, err = fetchSchemaForLint(cmd, cfg)
		if err != nil {
			return err
		}
		// An explicit --cypher-version beats the database's default language;
		// a CYPHER prologue in the query itself beats both (resolved by the
		// analyzer).
		if cmd.Flags().Changed("cypher-version") {
			schema.DefaultLanguage = string(version)
		}
	}

	// Declared --param entries switch parameter checking on: every $param
	// absent from the declarations becomes an error. Without any --param,
	// parameter errors are suppressed (parameters are assumed external).
	// `key:embed=` entries declare the key without computing the embedding —
	// only existence matters to the analyzer and :lint must not call out to
	// an embedding provider.
	if rawParams, _ := cmd.Flags().GetStringArray("param"); len(rawParams) > 0 {
		params, embeds, err := parseParams(rawParams)
		if err != nil {
			return err
		}
		for _, e := range embeds {
			params[e.Name] = nil
		}
		if schema == nil {
			schema = &linter.DbSchema{}
		}
		schema.Parameters = params
	}

	diags, err := lintFn(cypher, version, schema)
	if err != nil {
		return clierr.NewFatalError("query: lint: %s", err.Error())
	}

	rows := make(lintDiagnostics, 0, len(diags))
	errorCount := 0
	for _, d := range diags {
		if d.Severity == "error" {
			errorCount++
		}
		rows = append(rows, lintDiagnostic{
			Severity:  d.Severity,
			Message:   d.Message,
			Line:      d.Start.Line + 1,
			Column:    d.Start.Column + 1,
			Offset:    d.Start.Offset,
			EndLine:   d.End.Line + 1,
			EndColumn: d.End.Column + 1,
			EndOffset: d.End.Offset,
		})
	}

	commonoutput.PrintBodyMap(cmd, cfg, rows, lintFields)

	if errorCount > 0 {
		return clierr.NewValidationError("query: lint: %d error(s) found", errorCount)
	}
	return nil
}

// fetchSchemaForLint resolves the connection exactly like `:schema` does
// (flags, --credential, .env, env vars; password prompt on a TTY), fetches
// the lint schema, and closes the driver before returning — linting stays
// offline-after-fetch. Connection and query failures carry the standard
// categorized exit codes (6 client / 8 transport).
func fetchSchemaForLint(cmd *cobra.Command, cfg *clicfg.Config) (*linter.DbSchema, error) {
	c, err := resolveConn(cmd, cfg)
	if err != nil {
		return nil, err
	}
	if c.password == "" {
		pw, err := promptPassword(cmd)
		if err != nil {
			return nil, err
		}
		c.password = pw
	}
	if err := c.openDriver(); err != nil {
		return nil, err
	}
	ctx := cmd.Context()
	defer c.driver.Close(ctx) //nolint:errcheck // driver close error not actionable in defer
	return fetchLintSchema(ctx, c)
}
