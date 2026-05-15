// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"

	"github.com/neo4j/cli/common/clierr"
)

// pollInterval is the gap between consecutive Bolt readiness probes (REQ-F-018).
// Exposed as a package var so tests can shrink it without exporting a parameter
// on WaitForBolt; production code never mutates it.
var pollInterval = 500 * time.Millisecond

// boltProber is the injectable seam WaitForBolt uses to attempt a single Bolt
// handshake. Production wires probeBolt (open driver, run `RETURN 1`, close);
// tests swap in a func that counts calls or returns canned errors so the
// readiness loop can be exercised without a real Bolt server.
//
// The returned error is treated as "not ready yet, keep polling"; a nil return
// means the Bolt endpoint accepted a session and answered RETURN 1.
type boltProber func(ctx context.Context, uri, user, pass string) error

// probeBoltFn is the production prober. Held as a package var so tests can
// substitute the real driver call with a deterministic fake.
var probeBoltFn boltProber = probeBolt

// WaitForBolt blocks until a Bolt handshake against uri succeeds or timeout
// elapses, polling every pollInterval. Reuses the neo4j-go-driver/v6 already
// vendored by neo4j-cli/query/ (REQ-F-018, REQ-NF-001) — no separate raw Bolt
// frame implementation is needed because the driver is already on the import
// graph and imports cleanly from this package (no cycle with clicfg).
//
// On a timeout WaitForBolt returns a clierr.UsageError naming the duration and
// pointing the user at `docker logs <name>` for triage. The caller (typically
// `docker create --wait` in task-006) is responsible for NOT tearing down the
// container on timeout — the partially-started Neo4j may still finish booting
// after the CLI returns.
func WaitForBolt(ctx context.Context, uri, user, pass string, timeout time.Duration) error {
	if probeBoltFn == nil {
		return errors.New("docker: bolt prober not initialised")
	}

	// Honour an already-cancelled context before even attempting the first
	// probe so tests with a zero/short deadline get the timeout message
	// deterministically.
	deadline := time.Now().Add(timeout)
	probeCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// First attempt fires immediately so a Bolt endpoint that is already up
	// returns nil without sleeping for pollInterval first.
	if err := probeBoltFn(probeCtx, uri, user, pass); err == nil {
		return nil
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-probeCtx.Done():
			return clierr.NewUsageError(
				"container started but Bolt did not become ready within %s; check 'docker logs <name>'",
				timeout,
			)
		case <-ticker.C:
			if err := probeBoltFn(probeCtx, uri, user, pass); err == nil {
				return nil
			}
		}
	}
}

// probeBolt is the production boltProber: open a short-lived driver, start a
// session, run `RETURN 1`, close. Any non-nil return is interpreted as
// "not ready yet" by WaitForBolt's polling loop. We deliberately do NOT
// distinguish auth failures from transport failures here — by construction the
// caller (create --wait) generated the password itself, so any error during
// startup is a transient handshake failure worth retrying within the timeout.
func probeBolt(ctx context.Context, uri, user, pass string) error {
	driver, err := neo4j.NewDriver(uri, neo4j.BasicAuth(user, pass, ""))
	if err != nil {
		return fmt.Errorf("docker: bolt probe: open driver: %w", err)
	}
	defer driver.Close(ctx) //nolint:errcheck // close error during readiness probe is not actionable

	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx) //nolint:errcheck // session close error during probe is not actionable

	result, err := session.Run(ctx, "RETURN 1", nil)
	if err != nil {
		return fmt.Errorf("docker: bolt probe: run: %w", err)
	}
	if _, err := result.Consume(ctx); err != nil {
		return fmt.Errorf("docker: bolt probe: consume: %w", err)
	}
	return nil
}
