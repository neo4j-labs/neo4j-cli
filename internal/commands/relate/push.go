// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/spf13/cobra"

	engine "github.com/neo4j/cli/internal/relate"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/dbconn"
)

// newPushCmd builds the `neo4j-cli relate push <name-or-path>` leaf: it
// reads the local store's subgraph and upserts it into a remote Neo4j over
// Bolt, MERGE-keyed by the remote's uniqueness constraints (or explicit
// --merge-key overrides). Additive by default: nothing is ever deleted.
func newPushCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push <name-or-path>",
		Short: "Push a local Relate store's subgraph into a remote Neo4j",
		Long: "Push a local embedded Relate store's subgraph into a remote Neo4j database, over Bolt. " +
			"The remote is selected with the same connection flags as `query`; the first argument is the LOCAL store. " +
			"Nodes MERGE on their label's identity key: the remote's single-property uniqueness constraint " +
			"(SHOW CONSTRAINTS) or an explicit `--merge-key Label.property` override (repeatable). A label in scope " +
			"with no key is refused — create a uniqueness constraint on the remote or pass --merge-key. " +
			"Relationships MATCH their endpoints by those keys and MERGE on the (start, type, end) pattern — " +
			"pass `--rel-key property` to key the MERGE on a relationship property instead (for multiple rels " +
			"of one type between the same pair). Properties are replaced wholesale; the push is additive: " +
			"nothing is deleted. Scope with --labels and --types (comma-separated; default: the whole store). " +
			"--batch-size bounds rows per remote statement (default 500). --dry-run prints the plan — labels, " +
			"keys, counts, batches — without touching the remote. Requires a build with the Relate engine " +
			"(-tags relate).",
		Example: `# Push the whole store into a local Neo4j (nodes MERGE on remote unique constraints)
neo4j-cli relate push movies --rw --uri neo4j://localhost:7687 -u neo4j -p pw

# Push one label with an explicit merge key
neo4j-cli relate push movies --rw --uri neo4j://localhost:7687 -u neo4j -p pw \\
  --labels Person --merge-key Person.email

# Preview what a push would do
neo4j-cli relate push movies --rw --uri neo4j://localhost:7687 -u neo4j -p pw --dry-run

# Push only some labels and relationship types, keyed rels
neo4j-cli relate push movies --rw --credential local \\
  --labels Person,Movie --types ACTED_IN --rel-key roles`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return runPush(cmd, cfg, args[0])
		},
	}

	// Remote connection flags — the same vocabulary as pull.
	cmd.Flags().String("uri", "", "Neo4j Bolt URI of the remote to push to. http://<host>[:p][/...] is auto-rewritten to neo4j://<host>:7687; https://<host>[:p][/...] is auto-rewritten to neo4j+s://<host>:7687. (default \"neo4j://localhost:7687\")")
	cmd.Flags().StringP("username", "u", "", "Neo4j username of the remote (default \"neo4j\")")
	cmd.Flags().StringP("password", "p", "", "Neo4j password of the remote; prompted on TTY if unset")
	cmd.Flags().StringP("database", "d", "", "Remote database name; defaults to the connecting user's home database when unset")
	cmd.Flags().StringP("credential", "c", "", "Credential to use for the remote connection. Forms: 'desktop' (the single running Neo4j Desktop 2 DBMS), 'desktop-connection:<uuid>', or '<name>' (a persisted dbms credential; see 'neo4j-cli credential dbms list')")
	cmd.Flags().String("env", "", "Path to a .env file (auto-discovered by walking up from cwd if unset)")

	// Push flags — Relate's names verbatim.
	cmd.Flags().StringArray("labels", nil, "Comma-separated node labels to push (default: every label in the store)")
	cmd.Flags().StringArray("types", nil, "Comma-separated relationship types to push (default: every type in the store)")
	cmd.Flags().StringArray("merge-key", nil, "Identity key override as Label.property (repeatable); default is the remote's uniqueness constraint per label")
	cmd.Flags().String("rel-key", "", "Relationship property to key the remote MERGE on (for multiple rels of one type between the same pair)")
	cmd.Flags().Int("batch-size", engine.DefaultPullBatchSize, "Rows per remote write statement")
	cmd.Flags().Bool("dry-run", false, "Print the push plan (labels, keys, counts, batches) without touching the remote")

	return cmd
}

// remoteExecute is the seam over one remote statement: write selects
// ExecuteWrite (the MERGE upserts), anything else ExecuteRead (the SHOW
// CONSTRAINTS probe). It returns the statement's columns and rows so writes
// can report what actually landed. Tests inject canned responses.
var remoteExecute = func(ctx context.Context, driver neo4j.Driver, database, query string, params map[string]any, write bool) ([]string, [][]any, error) {
	session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(ctx) //nolint:errcheck // session close error not actionable in defer

	work := func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		records, err := result.Collect(ctx)
		if err != nil {
			return nil, err
		}
		columns := []string{}
		rows := [][]any{}
		if len(records) > 0 {
			columns = append(columns, records[0].Keys...)
			for _, rec := range records {
				rows = append(rows, append([]any(nil), rec.Values...))
			}
		}
		return []any{columns, rows}, nil
	}
	var out any
	var err error
	if write {
		out, err = session.ExecuteWrite(ctx, work)
	} else {
		out, err = session.ExecuteRead(ctx, work)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("push: %w", err)
	}
	pair, _ := out.([]any)
	if len(pair) != 2 {
		return nil, nil, errors.New("push: unexpected remote session result")
	}
	columns, _ := pair[0].([]string)
	rows, _ := pair[1].([][]any)
	return columns, rows, nil
}

// pushStats is the outcome of one push.
type pushStats struct {
	NodesPushed   int64
	RelsPushed    int64
	RelsUnmatched int64
}

// runPush is the push leaf's body: resolve keys from the remote's
// constraints and the overrides, plan against the local store, then
// (unless --dry-run) upsert label by label and group by group.
func runPush(cmd *cobra.Command, cfg *clicfg.Config, storeArg string) error {
	if engine.OpenStore == nil {
		return engine.NotCompiledError("`relate push`")
	}

	storePath := engine.ResolveStorePath(storeArg)
	if !engine.IsStore(cfg.Fs(), storePath) {
		return unknownStoreError(storePath)
	}
	store, err := engine.OpenStore(storePath, "")
	if err != nil {
		return clierr.NewFatalError("cannot open the local Relate store at %s: %w", storePath, err)
	}
	defer store.Close()

	// Remote connection (driver never opened for --dry-run without keys —
	// see below; the constraint probe needs it in every other case).
	conn, err := dbconn.ResolveConn(cmd, cfg, false)
	if err != nil {
		return err
	}
	if conn.Password == "" {
		pw, err := promptRemotePassword(cmd, cfg)
		if err != nil {
			return err
		}
		conn.Password = pw
	}
	driver, err := pullDriverOpener(conn.URI, conn.Username, conn.Password, conn.UserAgent, conn.Debug)
	if err != nil {
		return clierr.NewUpstreamError("push: open driver: %w", err)
	}
	defer driver.Close(cmd.Context()) //nolint:errcheck // driver close error not actionable in defer
	ctx := cmd.Context()

	// Resolve identity keys: remote uniqueness constraints, then overrides.
	mergeKeySpecs, _ := cmd.Flags().GetStringArray("merge-key")
	keys, err := resolveRemoteKeys(ctx, driver, conn.Database, mergeKeySpecs)
	if err != nil {
		return err
	}

	labels, _ := cmd.Flags().GetStringArray("labels")
	types, _ := cmd.Flags().GetStringArray("types")
	plan, err := engine.BuildPushPlan(store, splitCSV(labels), splitCSV(types), keys)
	if err != nil {
		return err
	}

	batchSize, _ := cmd.Flags().GetInt("batch-size")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	relKey, _ := cmd.Flags().GetString("rel-key")

	if dryRun {
		printPushPlan(cmd, plan, batchSize, relKey)
		return nil
	}
	if len(plan.UnkeyedLabels) > 0 {
		return clierr.NewUsageError(
			"no identity key for label(s) %s: create a uniqueness constraint on the remote, or pass "+
				"--merge-key <Label>.<property> for each",
			strings.Join(plan.UnkeyedLabels, ", "))
	}

	stats := &pushStats{}
	for _, l := range plan.Labels {
		rows, err := engine.ReadPushNodes(store, l.Label, l.Key)
		if err != nil {
			return err
		}
		if err := upsertRemote(ctx, driver, conn.Database,
			engine.PushNodesStatement(l.Label, l.Key), rows, batchSize,
			func(matched int) { stats.NodesPushed += int64(matched) }); err != nil {
			return err
		}
	}
	for _, g := range plan.Rels {
		rows, err := engine.ReadPushRels(store, g)
		if err != nil {
			return err
		}
		if err := upsertRemote(ctx, driver, conn.Database,
			engine.PushRelsStatement(g, relKey), rows, batchSize,
			func(matched int) {
				stats.RelsPushed += int64(matched)
				stats.RelsUnmatched += int64(len(rows) - matched)
			}); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"Pushed %d nodes and %d relationships to %s (%d relationships skipped: endpoints not matched)%s\n",
		stats.NodesPushed, stats.RelsPushed, identitySourceOf(conn), stats.RelsUnmatched,
		pushSkippedNote(plan))
	return nil
}

// pushSkippedNote renders the unkeyed-skip note when the plan carried any.
func pushSkippedNote(plan *engine.PushPlan) string {
	switch {
	case plan.UnkeyedRels > 0 && len(plan.UnkeyedLabels) > 0:
		return fmt.Sprintf("; %d relationships and %d label(s) left out for missing identity keys", plan.UnkeyedRels, len(plan.UnkeyedLabels))
	case plan.UnkeyedRels > 0:
		return fmt.Sprintf("; %d relationships left out for missing identity keys", plan.UnkeyedRels)
	default:
		return ""
	}
}

// resolveRemoteKeys builds the label → key spec: the remote's
// single-property NODE UNIQUENESS constraints, overridden by any
// --merge-key entries.
func resolveRemoteKeys(ctx context.Context, driver neo4j.Driver, database string, mergeKeySpecs []string) (engine.PushKeySpec, error) {
	keys := engine.PushKeySpec{}
	_, rows, err := remoteExecute(ctx, driver, database,
		"SHOW CONSTRAINTS YIELD name, type, entityType, labelsOrTypes, properties", nil, false)
	if err != nil {
		var ce *clierr.CLIError
		if errors.As(err, &ce) {
			return nil, err
		}
		return nil, clierr.NewUpstreamError("%w", err)
	}
	for _, row := range rows {
		typ, _ := row[1].(string)
		entity, _ := row[2].(string)
		labels, _ := row[3].([]any)
		props, _ := row[4].([]any)
		if entity != "NODE" || !strings.Contains(typ, "UNIQUENESS") || len(labels) != 1 || len(props) != 1 {
			continue
		}
		label, _ := labels[0].(string)
		prop, _ := props[0].(string)
		if label != "" && prop != "" {
			keys[label] = prop
		}
	}
	for _, spec := range mergeKeySpecs {
		label, key, err := engine.ParseMergeKey(spec)
		if err != nil {
			return nil, clierr.NewUsageError("%w", err)
		}
		keys[label] = key
	}
	return keys, nil
}

// upsertRemote runs one statement over the rows in batches, reporting how
// many rows the remote actually matched/merged per batch.
func upsertRemote(ctx context.Context, driver neo4j.Driver, database, stmt string, rows []map[string]any, batchSize int, onMatched func(matched int)) error {
	if batchSize <= 0 {
		batchSize = engine.DefaultPullBatchSize
	}
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := make([]any, 0, end-i)
		for _, r := range rows[i:end] {
			batch = append(batch, r)
		}
		_, resultRows, err := remoteExecute(ctx, driver, database, stmt, map[string]any{"rows": batch}, true)
		if err != nil {
			var ce *clierr.CLIError
			if errors.As(err, &ce) {
				return err
			}
			return clierr.NewUpstreamError("%w", err)
		}
		onMatched(len(resultRows))
	}
	return nil
}

// printPushPlan renders the --dry-run report.
func printPushPlan(cmd *cobra.Command, plan *engine.PushPlan, batchSize int, relKey string) {
	w := cmd.ErrOrStderr()
	_, _ = fmt.Fprintf(w, "Push plan (dry run — nothing is written)\n")
	for _, l := range plan.Labels {
		_, _ = fmt.Fprintf(w, "  label %s: %d nodes, merge key %s, %d batch(es)\n",
			l.Label, l.Count, l.Key, batchesOf(int(l.Count), batchSize))
	}
	for _, label := range plan.UnkeyedLabels {
		_, _ = fmt.Fprintf(w, "  label %s: REFUSED — no identity key (create a remote uniqueness constraint or pass --merge-key)\n", label)
	}
	for _, g := range plan.Rels {
		key := relKey
		if key == "" {
			key = "(start, type, end)"
		}
		_, _ = fmt.Fprintf(w, "  rel (%s)-[:%s]->(%s): %d rels, merge on %s, endpoints matched by %s/%s, %d batch(es)\n",
			g.StartLabel, g.Type, g.EndLabel, g.Count, key, g.StartKey, g.EndKey, batchesOf(int(g.Count), batchSize))
	}
	if plan.UnkeyedRels > 0 {
		_, _ = fmt.Fprintf(w, "  %d relationships skipped: endpoints have no keyed label\n", plan.UnkeyedRels)
	}
}

func batchesOf(total, batchSize int) int {
	if batchSize <= 0 || total == 0 {
		return 0
	}
	return (total + batchSize - 1) / batchSize
}

// splitCSV flattens comma-separated flag values into one list.
func splitCSV(values []string) []string {
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}
