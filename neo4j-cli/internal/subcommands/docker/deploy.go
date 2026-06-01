// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"fmt"
	"regexp"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
)

// databaseNamePattern is a conservative Neo4j database-name rule: an ASCII
// letter followed by letters, digits, dots, or dashes (1-63 chars total). It is
// stricter than the full Neo4j grammar on purpose — the name is concatenated
// into admin Cypher (STOP/START DATABASE) and passed to neo4j-admin, so
// rejecting anything outside this set closes the injection seam without needing
// to quote.
var databaseNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9.-]{0,62}$`)

// validateDatabaseName guards the source --database value before it is
// interpolated into admin Cypher. A name carrying spaces, semicolons, backticks,
// or any other non-identifier character cannot reach the statement builder.
func validateDatabaseName(database string) error {
	if !databaseNamePattern.MatchString(database) {
		return clierr.NewUsageError(
			"invalid database name %q: must start with a letter and contain only letters, digits, dots, or dashes (1-63 characters)",
			database,
		)
	}
	return nil
}

// dumpPath is the in-container scratch directory neo4j-admin dumps into and
// uploads from. Lives under /tmp so it never collides with a bind-mounted
// /data volume and is cleaned up best-effort once the upload finishes.
const dumpPath = "/tmp/neo4j-cli-deploy"

// AuraTarget is the destination for a database upload: the Aura instance's
// Bolt URI plus the admin credentials neo4j-admin authenticates with.
type AuraTarget struct {
	URI      string
	Username string
	Password string
}

// stopStartFn toggles a database on the source container's Bolt endpoint by
// running `STOP DATABASE <db>` / `START DATABASE <db>` against the system
// database. Production wires stopStartDatabase (a short-lived vendored driver
// session); tests swap in a recorder so PushToAura's ordering and the deferred
// START-on-failure path can be exercised without a live Bolt server.
//
// uri/user/pass identify the source container's admin connection; statement is
// the full admin Cypher (e.g. "STOP DATABASE neo4j").
var stopStartFn = stopStartDatabase

// PushToAura dumps a database from a running local Neo4j container and uploads
// it into an Aura instance via neo4j-admin (REQ-F-013..F-016). The source DB
// password is read from the stored dbms credential named after the container
// (the container-name == credential-name convention established by
// `docker create`).
//
// Order of operations:
//  1. STOP DATABASE <database> so neo4j-admin can take a consistent dump.
//  2. neo4j-admin database dump --to-path=<dumpPath>.
//  3. neo4j-admin database upload --from-path=<dumpPath> --to-uri/--to-user/
//     --to-password --overwrite-destination.
//  4. START DATABASE <database> (DEFERRED — always runs, even if dump/upload
//     fails, to restore the source container's prior state).
//  5. best-effort cleanup of the scratch dump dir (errors ignored).
//
// The Aura target password is passed to neo4j-admin via argv; on a non-zero
// exit the docker Exec path runs the captured stderr through redactArgs/
// redactString, which masks `--to-password=<v>` (the regex matches the
// PASSWORD substring), so the secret never reaches stderr or logs.
func PushToAura(ctx context.Context, cfg *clicfg.Config, client dockerClient, containerName, database string, target AuraTarget) error {
	if err := validateDatabaseName(database); err != nil {
		return err
	}

	if cfg == nil || cfg.Credentials == nil || cfg.Credentials.Dbms == nil {
		return clierr.NewUsageError(
			"credential storage is not available; cannot resolve the source password for container %q",
			containerName,
		)
	}

	cred, err := cfg.Credentials.Dbms.Get(containerName)
	if err != nil {
		return clierr.NewUsageError(
			"no stored dbms credential named %q for the source container; create the container with `neo4j-cli docker create --name %s` (which stores a credential) or add one via `neo4j-cli credential dbms add`",
			containerName, containerName,
		)
	}

	// STOP the source database for a consistent dump.
	if err := stopStartFn(ctx, cred.URI, cred.Username, cred.Password, "STOP DATABASE "+database); err != nil {
		return fmt.Errorf("docker deploy: stop database %q: %w", database, err)
	}

	// Always restore the source database to a started state, even if the dump
	// or upload below fails — deferred so a mid-flight error never leaves the
	// container's database stopped.
	defer func() {
		_ = stopStartFn(ctx, cred.URI, cred.Username, cred.Password, "START DATABASE "+database)
	}()

	// best-effort scratch cleanup once we're done, regardless of outcome.
	defer func() {
		_, _ = client.Exec(ctx, containerName, []string{"rm", "-rf", dumpPath})
	}()

	if _, err := client.Exec(ctx, containerName, []string{
		"neo4j-admin", "database", "dump", database, "--to-path=" + dumpPath,
	}); err != nil {
		return err
	}

	if _, err := client.Exec(ctx, containerName, []string{
		"neo4j-admin", "database", "upload", database,
		"--from-path=" + dumpPath,
		"--to-uri=" + target.URI,
		"--to-user=" + target.Username,
		"--to-password=" + target.Password,
		"--overwrite-destination",
	}); err != nil {
		return err
	}

	return nil
}

// stopStartDatabase is the production stopStartFn: open a short-lived driver
// against the source container's Bolt endpoint, run the admin statement on the
// system database, and close. Reuses the neo4j-go-driver/v6 already vendored by
// neo4j-cli/query/ (same import the WaitForBolt prober uses), so no extra Bolt
// machinery is pulled in. STOP/START DATABASE are admin statements that must
// run against the system database, hence the explicit DatabaseName.
func stopStartDatabase(ctx context.Context, uri, user, pass, statement string) error {
	driver, err := neo4j.NewDriver(uri, neo4j.BasicAuth(user, pass, ""))
	if err != nil {
		return fmt.Errorf("open driver: %w", err)
	}
	defer driver.Close(ctx) //nolint:errcheck // close error after admin statement is not actionable

	session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "system"})
	defer session.Close(ctx) //nolint:errcheck // session close error is not actionable

	result, err := session.Run(ctx, statement, nil)
	if err != nil {
		return fmt.Errorf("run %q: %w", statement, err)
	}
	if _, err := result.Consume(ctx); err != nil {
		return fmt.Errorf("consume %q: %w", statement, err)
	}
	return nil
}
