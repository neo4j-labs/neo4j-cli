// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package analytics

//go:generate go tool mockgen -destination=mocks/mock_analytics.go -package=analytics_mocks -typed github.com/neo4j/cli/common/analytics Service,HTTPClient

import (
	"net/http"
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
}
