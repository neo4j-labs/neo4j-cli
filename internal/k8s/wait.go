// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"time"

	"github.com/neo4j/cli/internal/clierr"
)

var waitPollInterval = 2 * time.Second

// WaitForReady polls until the operator reports Ready=True for the current
// generation. A Failed phase is terminal, so the caller gets the condition
// message immediately instead of waiting for the timeout.
func WaitForReady(ctx context.Context, client Client, name string, timeout time.Duration) (Neo4j, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		instance, err := client.GetNeo4j(ctx, name)
		if err != nil {
			return Neo4j{}, err
		}
		if instance.Ready {
			return instance, nil
		}
		if instance.Phase == "Failed" {
			message := instance.Message
			if message == "" {
				message = "the operator reported no condition message"
			}
			return Neo4j{}, clierr.NewUpstreamError("Neo4j %q failed: %s", name, message)
		}
		if err := sleepContext(ctx, waitPollInterval); err != nil {
			return Neo4j{}, waitTimeoutError(ctx, err, name, instance.Phase, timeout)
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
			return err
		}
		if err := sleepContext(ctx, waitPollInterval); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return clierr.NewUpstreamError("timed out after %s waiting for Neo4j %q to be deleted", timeout, name)
			}
			return err
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

func waitTimeoutError(ctx context.Context, err error, name, phase string, timeout time.Duration) error {
	if ctx.Err() == context.DeadlineExceeded {
		return clierr.NewUpstreamError("timed out after %s waiting for Neo4j %q to become ready (last phase: %s)", timeout, name, phase)
	}
	return err
}
