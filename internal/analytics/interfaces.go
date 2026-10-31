// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package analytics

//go:generate go tool mockgen -destination=mocks/mock_analytics.go -package=analytics_mocks -typed github.com/neo4j/cli/internal/analytics Service,HTTPClient

import (
	"net/http"
	"time"
)

// HTTPClient is the subset of *http.Client used by Analytics, allowing injection of a mock in tests.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Service interface {
	Disable()
	// EmitEvent queues an event for dispatch. The full event name is constructed
	// as "<appName>_<eventSuffix>" so the app-name prefix stays in the analytics
	// package and callers only supply the distinguishing suffix (e.g. "COMMAND_USED").
	EmitEvent(eventSuffix string, event TrackEvent)
	// Flush blocks until all in-flight async EmitEvent goroutines have completed.
	// Call it during shutdown to avoid dropping events.
	Flush()
	// FlushTimeout is Flush bounded by a deadline: it waits at most timeout for
	// queued events to drain, then returns, dropping whatever is still in
	// flight. The exit path of a short-lived process should use this with
	// DefaultShutdownFlushTimeout so a slow analytics endpoint cannot add its
	// HTTP timeout per queued event to every invocation. A timeout <= 0
	// behaves like Flush.
	FlushTimeout(timeout time.Duration)
}
