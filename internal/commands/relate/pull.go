// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/config"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/log"
	"github.com/spf13/cobra"

	engine "github.com/neo4j/cli/internal/relate"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/dbconn"
	"github.com/neo4j/cli/internal/params"
)

// newPullCmd builds the `neo4j-cli relate pull <name-or-path>` leaf: it
// streams a subgraph from a remote Neo4j (the --query must return graph
// entities — nodes, relationships or paths) over Bolt and upserts it into
// the local store, keyed by source identity so repeated pulls update rather
// than duplicate. The local store is created when missing.
//
// The remote connection uses the same flag vocabulary as `query`
// (--uri/--username/--password/--database/--credential/--env), resolved
// through the shared dbconn layer. The pull's own flags keep Relate's
// names: --query, --param, --batch-size.
func newPullCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull <name-or-path>",
		Short: "Pull a subgraph from a remote Neo4j into a local Relate store",
		Long: "Pull a subgraph from a remote Neo4j database into a local embedded Relate store, over Bolt. " +
			"The remote is selected with the same connection flags as `query` (--uri, --username, --password, " +
			"--database for the remote database name, --credential, --env); the first argument is the LOCAL store " +
			"(a bare name under the managed directory, or a path — created when missing). " +
			"--query is the remote Cypher; it must return graph entities — nodes, relationships or paths " +
			"(e.g. `MATCH (n:Person) RETURN n` or `MATCH (a)-[r:KNOWS]->(b) RETURN a, r, b`); scalar-only " +
			"results are refused. --param binds remote query parameters (key=value, JSON-typed). " +
			"Pulled entities are upserted by source identity: a second pull from the same source updates " +
			"the entities in place instead of duplicating them (properties are replaced wholesale). " +
			"Relationships whose endpoints are not part of the pulled result are skipped and reported. " +
			"--batch-size bounds the rows per write statement (default 500; property values are inlined as " +
			"literals, so smaller batches mean smaller statements). The store is checkpointed after a " +
			"successful pull. Requires a build with the Relate engine (-tags relate).",
		Example: `# Pull the Person subgraph from a local Neo4j into a managed store
neo4j-cli relate pull people --uri neo4j://localhost:7687 -u neo4j -p pw \\
  --query "MATCH (n:Person) RETURN n"

# Pull a labeled neighborhood with relationships, using a stored credential
neo4j-cli relate pull movies --credential local \\
  --query "MATCH (a:Person)-[r:ACTED_IN]->(m:Movie) RETURN a, r, m"

# Pull with parameters and a smaller write batch
neo4j-cli relate pull recent --uri neo4j://localhost:7687 -u neo4j -p pw \\
  --query "MATCH (n:Event) WHERE n.date > $since RETURN n" --param since=\"2026-01-01\" --batch-size 100`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return runPull(cmd, cfg, args[0])
		},
	}

	// Remote connection flags — the same vocabulary and help as `query`.
	cmd.Flags().String("uri", "", "Neo4j Bolt URI of the remote to pull from. http://<host>[:p][/...] is auto-rewritten to neo4j://<host>:7687; https://<host>[:p][/...] is auto-rewritten to neo4j+s://<host>:7687. (default \"neo4j://localhost:7687\")")
	cmd.Flags().StringP("username", "u", "", "Neo4j username of the remote (default \"neo4j\")")
	cmd.Flags().StringP("password", "p", "", "Neo4j password of the remote; prompted on TTY if unset")
	cmd.Flags().StringP("database", "d", "", "Remote database name; defaults to the connecting user's home database when unset")
	cmd.Flags().StringP("credential", "c", "", "Credential to use for the remote connection. Forms: 'desktop' (the single running Neo4j Desktop 2 DBMS), 'desktop-connection:<uuid>', or '<name>' (a persisted dbms credential; see 'neo4j-cli credential dbms list')")
	cmd.Flags().String("env", "", "Path to a .env file (auto-discovered by walking up from cwd if unset)")

	// Pull flags — Relate's names verbatim.
	cmd.Flags().String("query", "", "Remote Cypher to pull; must return nodes, relationships or paths (not scalars)")
	cmd.Flags().StringArray("param", nil, "Remote query parameter as key=value (repeatable); JSON-typed when value parses as JSON, otherwise a string")
	cmd.Flags().Int("batch-size", engine.DefaultPullBatchSize, "Rows per write statement into the local store (property values are inlined as literals, so smaller batches mean smaller statements)")

	if err := cmd.MarkFlagRequired("query"); err != nil {
		panic(err) //nolint:errcheck // MarkFlagRequired on a registered flag cannot fail
	}

	return cmd
}

// pullDriverOpener is the test seam for constructing the remote Bolt driver,
// mirroring the query tree's driverOpener.
var pullDriverOpener = func(target, username, password, userAgent string, debug bool) (neo4j.Driver, error) {
	return neo4j.NewDriver(target, neo4j.BasicAuth(username, password, ""), func(c *config.Config) {
		if userAgent != "" {
			c.UserAgent = userAgent
		}
		if debug {
			c.Log = dbconn.NewStderrLogger(log.DEBUG)
		}
		c.ConnectionAcquisitionTimeout = 10 * time.Second
		c.MaxTransactionRetryTime = 10 * time.Second
	})
}

// streamRemoteRecords is the seam over the remote streaming read: it runs
// query with params on the remote database and invokes onRow once per
// record with the record's positional values. Production streams through
// the driver result (no full-result buffering on the remote side); tests
// inject canned record streams.
var streamRemoteRecords = func(ctx context.Context, driver neo4j.Driver, database, query string, queryParams map[string]any, onRow func(values []any) error) error {
	session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: database})
	defer session.Close(ctx) //nolint:errcheck // session close error not actionable in defer

	result, err := session.Run(ctx, query, queryParams)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	for result.Next(ctx) {
		rec := result.Record()
		if err := onRow(rec.Values); err != nil {
			return err
		}
	}
	return result.Err()
}

// runPull is the pull leaf's body: resolve the local store and the remote
// connection, stream the query's entities into a subgraph, materialize it
// through the identity map, checkpoint, and report.
func runPull(cmd *cobra.Command, cfg *clicfg.Config, storeArg string) error {
	if engine.OpenStore == nil {
		return engine.NotCompiledError("`relate pull`")
	}

	query, _ := cmd.Flags().GetString("query")
	rawParams, _ := cmd.Flags().GetStringArray("param")
	queryParams, embeds, err := params.ParseParams(rawParams)
	if err != nil {
		return err
	}
	if len(embeds) > 0 {
		return clierr.NewUsageError("--param :embed is not supported by `relate pull` (embed vectors cannot be pulled); drop the modifier")
	}
	batchSize, _ := cmd.Flags().GetInt("batch-size")

	// Local store: created when missing (the pull itself is the creation).
	storePath := engine.ResolveStorePath(storeArg)
	store, err := engine.OpenStore(storePath, "")
	if err != nil {
		return clierr.NewFatalError("cannot open the local Relate store at %s: %w", storePath, err)
	}
	defer store.Close()

	// Remote connection, through the shared dbconn resolution.
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
		return clierr.NewUpstreamError("pull: open driver: %w", err)
	}
	defer driver.Close(cmd.Context()) //nolint:errcheck // driver close error not actionable in defer

	// Stream the remote result into the subgraph.
	subgraph := engine.NewSubgraph()
	rows, entityRows := 0, 0
	ctx := cmd.Context()
	err = streamRemoteRecords(ctx, driver, conn.Database, query, queryParams, func(values []any) error {
		rows++
		if found := engine.ExtractEntities(subgraph, values); found > 0 {
			entityRows++
		}
		return nil
	})
	if err != nil {
		var ce *clierr.CLIError
		if errors.As(err, &ce) {
			return err
		}
		return clierr.NewUpstreamError("%w", err)
	}
	if rows > 0 && entityRows == 0 {
		return clierr.NewUsageError(
			"the --query returned only scalar values; `relate pull` copies graph entities — " +
				"return nodes, relationships or paths (e.g. `MATCH (n:Person) RETURN n` or " +
				"`MATCH (a)-[r]->(b) RETURN a, r, b`)")
	}

	// Materialize through the identity map, checkpoint, persist the map.
	identityMap, err := engine.LoadIdentityMap(cfg.Fs(), storePath)
	if err != nil {
		return err
	}
	source := identitySourceOf(conn)
	stats, err := engine.Materialize(store, subgraph, identityMap, source, batchSize)
	if err != nil {
		return err
	}
	if err := store.Checkpoint(); err != nil {
		return err
	}
	if err := identityMap.Save(cfg.Fs(), storePath); err != nil {
		return clierr.NewFatalError("cannot save the pull identity map beside the store: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"Pulled %d nodes (%d new, %d reused) and %d relationships (%d new, %d reused, %d skipped: endpoint not pulled) from %s into %s\n",
		stats.NodesCreated+stats.NodesReused, stats.NodesCreated, stats.NodesReused,
		stats.RelsCreated+stats.RelsReused+stats.RelsSkipped, stats.RelsCreated, stats.RelsReused, stats.RelsSkipped,
		source, storePath)
	return nil
}

// identitySourceOf derives the identity-map key for one remote: URI and
// database name, so pulling from two remotes into one store keeps separate
// identity tables.
func identitySourceOf(c *dbconn.Conn) string {
	db := c.Database
	if db == "" {
		db = "neo4j"
	}
	return c.URI + "/" + db
}

// promptRemotePassword mirrors the query tree's password prompt: read from
// the controlling terminal with no echo, or a clear usage error when stdin
// is not a TTY.
func promptRemotePassword(cmd *cobra.Command, cfg *clicfg.Config) (string, error) {
	if !dbconn.StdinIsTTY() {
		return "", clierr.NewUsageError(
			"%s", dbconn.PasswordRequiredMessage(cfg.Global.AcceptEnvVars()))
	}
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
	pw, err := dbconn.PasswordReader()
	_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", fmt.Errorf("relate pull: read password: %w", err)
	}
	return pw, nil
}
