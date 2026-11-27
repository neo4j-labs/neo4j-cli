// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbconn

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

// DefaultWaitTimeout bounds a Bolt readiness check after a resource starts.
const DefaultWaitTimeout = 60 * time.Second

var boltPollInterval = 500 * time.Millisecond

// BoltProber attempts one authenticated query against a Neo4j endpoint.
type BoltProber func(ctx context.Context, uri, username, password string) error

var probeBoltFn BoltProber = probeBolt

// BoltTimeoutError lets each command add its own diagnostic hint without
// matching the text of the shared readiness error.
type BoltTimeoutError struct {
	Timeout time.Duration
}

func (e *BoltTimeoutError) Error() string {
	return fmt.Sprintf("Bolt did not become ready within %s", e.Timeout)
}

// WaitForBolt polls until Neo4j answers RETURN 1 with the supplied credentials.
// The first attempt starts at once. The caller keeps its resource when the
// wait expires because Neo4j can still finish startup after the CLI returns.
func WaitForBolt(ctx context.Context, uri, username, password string, timeout time.Duration) error {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := probeBoltFn(probeCtx, uri, username, password); err == nil {
		return nil
	}
	ticker := time.NewTicker(boltPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-probeCtx.Done():
			if err := ctx.Err(); err != nil {
				return err
			}
			return &BoltTimeoutError{Timeout: timeout}
		case <-ticker.C:
			if err := probeBoltFn(probeCtx, uri, username, password); err == nil {
				return nil
			}
		}
	}
}

// probeBolt does not expose the last driver error. Neo4j can reject a session
// while it starts, and the caller receives a timeout with a resource-specific
// diagnostic hint instead of a transient startup error.
func probeBolt(ctx context.Context, uri, username, password string) error {
	driver, err := neo4j.NewDriver(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return fmt.Errorf("bolt probe: open driver: %w", err)
	}
	defer driver.Close(ctx) //nolint:errcheck // A probe close error does not change readiness.
	session := driver.NewSession(ctx, neo4j.SessionConfig{})
	defer session.Close(ctx) //nolint:errcheck // A probe close error does not change readiness.
	result, err := session.Run(ctx, "RETURN 1", nil)
	if err != nil {
		return fmt.Errorf("bolt probe: run: %w", err)
	}
	_, err = result.Consume(ctx)
	return err
}
