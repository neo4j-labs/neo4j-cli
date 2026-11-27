// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neo4j/cli/internal/clierr"
)

var waitPollInterval = 2 * time.Second

// WaitForReady polls until the operator reports Ready=True for the current
// generation. If the phase is Failed, it returns the condition message at once.
func WaitForReady(ctx context.Context, client Client, name string, timeout time.Duration) (Neo4j, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	lastPhase := "unknown"
	for {
		instance, err := client.GetNeo4j(ctx, name)
		if err != nil {
			return Neo4j{}, timeoutError(ctx, err, timeout, "Neo4j %q to become ready (last phase: %s)", name, lastPhase)
		}
		if instance.Phase == "Failed" {
			message := instance.Message
			if message == "" {
				message = "the operator reported no condition message"
			}
			return Neo4j{}, clierr.NewUpstreamError("Neo4j %q failed: %s", name, message)
		}
		if instance.Ready {
			return instance, nil
		}
		if instance.Phase != "" {
			lastPhase = instance.Phase
		}
		if err := sleepContext(ctx, waitPollInterval); err != nil {
			return Neo4j{}, timeoutError(ctx, err, timeout, "Neo4j %q to become ready (last phase: %s)", name, lastPhase)
		}
	}
}

// WaitForDelete polls until kubectl reports that the resource is absent.
func WaitForDelete(ctx context.Context, client Client, name string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		_, err := client.GetNeo4j(ctx, name)
		if IsNotFound(err) {
			return nil
		}
		if err != nil {
			return timeoutError(ctx, err, timeout, "Neo4j %q to be deleted", name)
		}
		if err := sleepContext(ctx, waitPollInterval); err != nil {
			return timeoutError(ctx, err, timeout, "Neo4j %q to be deleted", name)
		}
	}
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// timeoutError formats a timeout message when the wait deadline expires during
// a read or a sleep. It returns any other error, such as an interrupt, unchanged.
func timeoutError(ctx context.Context, err error, timeout time.Duration, format string, args ...any) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return clierr.NewUpstreamError("timed out after %s waiting for %s", timeout, fmt.Sprintf(format, args...))
	}
	return err
}
