// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/dbconn"
)

// pollInterval is the gap between consecutive TCP readiness probes (REQ-F-018).
// Exposed as a package var so tests can shrink it without exporting a parameter
// on WaitForBoltTCP. Production code never mutates it. The authenticated Bolt
// wait has its own interval in internal/dbconn.
var pollInterval = 500 * time.Millisecond

// WaitForBolt uses the shared readiness loop and adds Docker's diagnostic hint.
func WaitForBolt(ctx context.Context, uri, user, pass string, timeout time.Duration) error {
	err := dbconn.WaitForBolt(ctx, uri, user, pass, timeout)
	var timeoutErr *dbconn.BoltTimeoutError
	if errors.As(err, &timeoutErr) {
		return clierr.NewUsageError(
			"container started but Bolt did not become ready within %s; check 'docker logs <name>'",
			timeout,
		)
	}
	return err
}

// tcpDialer is the injectable seam WaitForBoltTCP uses to attempt a single TCP
// connection. Production wires net.DialTimeout; tests swap in a deterministic
// fake so the readiness loop can be exercised without a real listener.
type tcpDialer func(network, address string, timeout time.Duration) (net.Conn, error)

// tcpDialFn is the production dialer. Held as a package var so tests can
// substitute the real net.DialTimeout with a fake that returns canned
// connections / errors per call.
var tcpDialFn tcpDialer = net.DialTimeout

// tcpDialAttemptTimeout caps each individual Dial so a slow-failing connect
// (DNS hang, half-open SYN, etc.) cannot blow past the overall WaitForBoltTCP
// deadline. Kept generous enough for cold loopback on Windows CI yet far
// shorter than the smallest practical waitTimeout.
const tcpDialAttemptTimeout = 500 * time.Millisecond

// WaitForBoltTCP polls a TCP connection to host:port until it accepts (the
// server is listening) or timeout elapses. This is a WEAKER readiness check
// than WaitForBolt — Neo4j may bind the port briefly before it is ready to
// handle Bolt handshakes — but it is the best we can do without credentials
// to authenticate a real Bolt session. `docker start --wait` uses it as a
// fallback when no stored dbms credential is available.
//
// Reuses the same pollInterval seam as WaitForBolt so test setup that shrinks
// pollInterval applies to both probes uniformly.
func WaitForBoltTCP(ctx context.Context, host string, port int, timeout time.Duration) error {
	if tcpDialFn == nil {
		return errors.New("docker: tcp dialer not initialised")
	}

	deadline := time.Now().Add(timeout)
	probeCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	addr := fmt.Sprintf("%s:%d", host, port)

	// First probe fires immediately so a port that's already listening returns
	// nil without sleeping for pollInterval first (mirrors WaitForBolt).
	if conn, err := tcpDialFn("tcp", addr, tcpDialAttemptTimeout); err == nil {
		_ = conn.Close()
		return nil
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-probeCtx.Done():
			return clierr.NewUsageError(
				"container started but TCP port %d did not open within %s; check 'docker logs <name>'",
				port, timeout,
			)
		case <-ticker.C:
			if conn, err := tcpDialFn("tcp", addr, tcpDialAttemptTimeout); err == nil {
				_ = conn.Close()
				return nil
			}
		}
	}
}

// DefaultWaitTimeout is the fixed budget for a Docker readiness check.
const DefaultWaitTimeout = dbconn.DefaultWaitTimeout

// The loader uses these seams so tests can shorten the timeout and replace
// the readiness check without a running Neo4j server.
var (
	waitTimeout   = DefaultWaitTimeout
	waitForBoltFn = WaitForBolt
)
